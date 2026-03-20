package postgres

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tech-comparison-lab/loadgen-analytics/internal/gen"
)

// Bench drives PostgreSQL benchmark operations via pgxpool.
type Bench struct {
	pool *pgxpool.Pool
}

// New opens a pgxpool and pings the server.
func New(ctx context.Context, dsn string) (*Bench, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("pgxpool.New: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return &Bench{pool: pool}, nil
}

// Close releases all pool connections.
func (b *Bench) Close() { b.pool.Close() }

// Truncate removes all rows from the events table.
func (b *Bench) Truncate(ctx context.Context) error {
	_, err := b.pool.Exec(ctx, "TRUNCATE events")
	return err
}

// StorageSize returns pg_total_relation_size('events') in bytes.
func (b *Bench) StorageSize(ctx context.Context) (int64, error) {
	var size int64
	err := b.pool.QueryRow(ctx, "SELECT pg_total_relation_size('events')").Scan(&size)
	return size, err
}

// Insert inserts count events using workers goroutines via CopyFrom.
func (b *Bench) Insert(ctx context.Context, count, batchSize, workers int) ([]time.Duration, time.Duration, error) {
	type job struct{ events []gen.Event }
	jobs := make(chan job, workers*2)

	var mu sync.Mutex
	durs := make([]time.Duration, 0, count/batchSize+1)
	var wg sync.WaitGroup
	var firstErr error

	cols := []string{"id", "user_id", "event", "ts", "country", "value", "session"}

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				rows := make([][]any, len(j.events))
				for k, e := range j.events {
					rows[k] = []any{e.ID, int32(e.UserID), e.Event, e.TS, e.Country, e.Value, e.Session}
				}
				t := time.Now()
				_, err := b.pool.CopyFrom(ctx, pgx.Identifier{"events"}, cols, pgx.CopyFromRows(rows))
				if err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					mu.Unlock()
					continue
				}
				mu.Lock()
				durs = append(durs, time.Since(t))
				mu.Unlock()
			}
		}()
	}

	start := time.Now()
	remaining := count
	for remaining > 0 {
		size := batchSize
		if remaining < size {
			size = remaining
		}
		jobs <- job{events: gen.Batch(size)}
		remaining -= size
	}
	close(jobs)
	wg.Wait()
	return durs, time.Since(start), firstErr
}

// runQuery executes q for iters iterations and collects per-iteration durations.
func (b *Bench) runQuery(ctx context.Context, q string, iters int) ([]time.Duration, time.Duration, error) {
	durs := make([]time.Duration, 0, iters)
	start := time.Now()
	for i := 0; i < iters; i++ {
		t := time.Now()
		rows, err := b.pool.Query(ctx, q)
		if err != nil {
			return durs, time.Since(start), err
		}
		for rows.Next() {
		} // drain
		rows.Close()
		if err := rows.Err(); err != nil {
			return durs, time.Since(start), err
		}
		durs = append(durs, time.Since(t))
	}
	return durs, time.Since(start), nil
}

// CountGroup: SELECT user_id, count(*) GROUP BY user_id ORDER BY count DESC LIMIT 100
func (b *Bench) CountGroup(ctx context.Context, iters int) ([]time.Duration, time.Duration, error) {
	return b.runQuery(ctx,
		`SELECT user_id, count(*) AS cnt FROM events GROUP BY user_id ORDER BY cnt DESC LIMIT 100`,
		iters)
}

// TimeRange: count events by type within H1 2024
func (b *Bench) TimeRange(ctx context.Context, iters int) ([]time.Duration, time.Duration, error) {
	return b.runQuery(ctx,
		`SELECT event, count(*) FROM events
		 WHERE ts >= '2024-01-01' AND ts < '2024-07-01'
		 GROUP BY event ORDER BY count(*) DESC`,
		iters)
}

// AggRevenue: total purchase revenue by country
func (b *Bench) AggRevenue(ctx context.Context, iters int) ([]time.Duration, time.Duration, error) {
	return b.runQuery(ctx,
		`SELECT country, sum(value) AS revenue FROM events
		 WHERE event = 'purchase'
		 GROUP BY country ORDER BY revenue DESC`,
		iters)
}

// DistinctUsers: count distinct buyers in H1 2024
func (b *Bench) DistinctUsers(ctx context.Context, iters int) ([]time.Duration, time.Duration, error) {
	return b.runQuery(ctx,
		`SELECT count(DISTINCT user_id) FROM events
		 WHERE event = 'purchase' AND ts >= '2024-01-01'`,
		iters)
}
