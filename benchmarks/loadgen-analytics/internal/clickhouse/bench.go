package clickhouse

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"

	"github.com/tech-comparison-lab/loadgen-analytics/internal/gen"
)

// Bench drives ClickHouse benchmark operations.
type Bench struct {
	conn clickhouse.Conn
}

// New connects to ClickHouse and creates the bench database/table.
func New(ctx context.Context, addr string) (*Bench, error) {
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{addr},
		Auth: clickhouse.Auth{
			Database: "bench",
			Username: "bench",
			Password: "benchpass",
		},
		Compression: &clickhouse.Compression{Method: clickhouse.CompressionLZ4},
	})
	if err != nil {
		return nil, fmt.Errorf("clickhouse.Open: %w", err)
	}
	if err := conn.Ping(ctx); err != nil {
		return nil, fmt.Errorf("ping: %w", err)
	}
	return &Bench{conn: conn}, nil
}

// Close releases the connection.
func (b *Bench) Close() { b.conn.Close() }

// Setup creates the bench database and events table.
func (b *Bench) Setup(ctx context.Context) error {
	if err := b.conn.Exec(ctx, "CREATE DATABASE IF NOT EXISTS bench"); err != nil {
		return err
	}
	return b.conn.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS bench.events (
			id       String,
			user_id  UInt32,
			event    LowCardinality(String),
			ts       DateTime,
			country  LowCardinality(String),
			value    Float32,
			session  String
		) ENGINE = MergeTree()
		PARTITION BY toYYYYMM(ts)
		ORDER BY (user_id, ts)
	`)
}

// Truncate removes all rows from the table.
func (b *Bench) Truncate(ctx context.Context) error {
	return b.conn.Exec(ctx, "TRUNCATE TABLE IF EXISTS bench.events")
}

// StorageSize returns compressed on-disk size in bytes.
func (b *Bench) StorageSize(ctx context.Context) (int64, error) {
	var size uint64
	err := b.conn.QueryRow(ctx,
		"SELECT sum(bytes_on_disk) FROM system.parts WHERE database='bench' AND table='events' AND active",
	).Scan(&size)
	return int64(size), err
}

// Insert inserts count events using workers goroutines, batchSize per batch.
func (b *Bench) Insert(ctx context.Context, count, batchSize, workers int) ([]time.Duration, time.Duration, error) {
	type job struct{ events []gen.Event }
	jobs := make(chan job, workers*2)

	var mu sync.Mutex
	durs := make([]time.Duration, 0, count/batchSize+1)
	var wg sync.WaitGroup
	var firstErr error

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				t := time.Now()
				batch, err := b.conn.PrepareBatch(ctx, "INSERT INTO bench.events")
				if err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					mu.Unlock()
					continue
				}
				for _, e := range j.events {
					if err := batch.Append(e.ID, e.UserID, e.Event, e.TS, e.Country, e.Value, e.Session); err != nil {
						mu.Lock()
						if firstErr == nil {
							firstErr = err
						}
						mu.Unlock()
						break
					}
				}
				if err := batch.Send(); err != nil {
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
		rows, err := b.conn.Query(ctx, q)
		if err != nil {
			return durs, time.Since(start), err
		}
		for rows.Next() {
		} // drain
		if err := rows.Err(); err != nil {
			return durs, time.Since(start), err
		}
		rows.Close()
		durs = append(durs, time.Since(t))
	}
	return durs, time.Since(start), nil
}

// CountGroup: SELECT user_id, count() GROUP BY user_id ORDER BY count() DESC LIMIT 100
func (b *Bench) CountGroup(ctx context.Context, iters int) ([]time.Duration, time.Duration, error) {
	return b.runQuery(ctx,
		`SELECT user_id, count() AS cnt FROM bench.events GROUP BY user_id ORDER BY cnt DESC LIMIT 100`,
		iters)
}

// TimeRange: count events by type within H1 2024
func (b *Bench) TimeRange(ctx context.Context, iters int) ([]time.Duration, time.Duration, error) {
	return b.runQuery(ctx,
		`SELECT event, count() FROM bench.events
		 WHERE ts >= '2024-01-01 00:00:00' AND ts < '2024-07-01 00:00:00'
		 GROUP BY event ORDER BY count() DESC`,
		iters)
}

// AggRevenue: total purchase revenue by country
func (b *Bench) AggRevenue(ctx context.Context, iters int) ([]time.Duration, time.Duration, error) {
	return b.runQuery(ctx,
		`SELECT country, sum(value) AS revenue FROM bench.events
		 WHERE event = 'purchase'
		 GROUP BY country ORDER BY revenue DESC`,
		iters)
}

// DistinctUsers: count distinct buyers in H1 2024
func (b *Bench) DistinctUsers(ctx context.Context, iters int) ([]time.Duration, time.Duration, error) {
	return b.runQuery(ctx,
		`SELECT count(DISTINCT user_id) FROM bench.events
		 WHERE event = 'purchase' AND ts >= '2024-01-01 00:00:00'`,
		iters)
}
