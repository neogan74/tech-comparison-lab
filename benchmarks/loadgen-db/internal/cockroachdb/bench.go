package cockroachdb

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tech-comparison-lab/loadgen-db/internal/gen"
)

// Bench drives CockroachDB benchmark operations via pgxpool.
// CockroachDB is PostgreSQL wire-protocol compatible, so pgx/v5 is used directly.
type Bench struct {
	pool *pgxpool.Pool
}

// New opens a pgxpool connection and pings the server.
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

// Truncate removes all rows from the orders table.
func (b *Bench) Truncate(ctx context.Context) error {
	_, err := b.pool.Exec(ctx, "TRUNCATE orders")
	return err
}

// Insert inserts count documents in batches of batchSize using workers goroutines.
// Returns per-batch durations and total elapsed time.
func (b *Bench) Insert(ctx context.Context, count, batchSize, workers int) ([]time.Duration, time.Duration, error) {
	totalBatches := count / batchSize
	if count%batchSize != 0 {
		totalBatches++
	}

	type job struct {
		docs []gen.Order
	}
	jobs := make(chan job, workers*2)
	durs := make([]time.Duration, 0, totalBatches)
	var mu sync.Mutex
	var wg sync.WaitGroup
	var firstErr error

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				start := time.Now()
				if err := b.insertBatch(ctx, j.docs); err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					mu.Unlock()
					continue
				}
				mu.Lock()
				durs = append(durs, time.Since(start))
				mu.Unlock()
			}
		}()
	}

	start := time.Now()
	remaining := count
	for remaining > 0 {
		size := batchSize
		if remaining < batchSize {
			size = remaining
		}
		jobs <- job{docs: gen.Batch(size)}
		remaining -= size
	}
	close(jobs)
	wg.Wait()
	total := time.Since(start)

	return durs, total, firstErr
}

func (b *Bench) insertBatch(ctx context.Context, docs []gen.Order) error {
	batch := &pgx.Batch{}
	const q = "INSERT INTO orders(id, doc, created_at) VALUES($1, $2, $3) ON CONFLICT DO NOTHING"
	for _, d := range docs {
		docJSON, err := json.Marshal(d)
		if err != nil {
			return err
		}
		batch.Queue(q, d.ID, string(docJSON), d.CreatedAt)
	}
	br := b.pool.SendBatch(ctx, batch)
	for range docs {
		if _, err := br.Exec(); err != nil {
			br.Close()
			return err
		}
	}
	return br.Close()
}

// Query runs iterations of: SELECT id, doc FROM orders WHERE user.country = 'US' LIMIT 100
func (b *Bench) Query(ctx context.Context, iterations int) ([]time.Duration, time.Duration, error) {
	const q = `SELECT id, doc FROM orders WHERE doc->'user'->>'country' = 'US' LIMIT 100`
	durs := make([]time.Duration, 0, iterations)
	start := time.Now()
	for i := 0; i < iterations; i++ {
		t := time.Now()
		rows, err := b.pool.Query(ctx, q)
		if err != nil {
			return durs, time.Since(start), err
		}
		for rows.Next() {
			var id, doc string
			if err := rows.Scan(&id, &doc); err != nil {
				rows.Close()
				return durs, time.Since(start), err
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return durs, time.Since(start), err
		}
		durs = append(durs, time.Since(t))
	}
	return durs, time.Since(start), nil
}

// Agg runs iterations of: GROUP BY user.id, SUM(quantity) ORDER BY total DESC LIMIT 100
func (b *Bench) Agg(ctx context.Context, iterations int) ([]time.Duration, time.Duration, error) {
	const q = `
		SELECT doc->'user'->>'id' AS user_id,
		       SUM((doc->>'quantity')::int) AS total_qty
		FROM orders
		GROUP BY 1
		ORDER BY 2 DESC
		LIMIT 100`
	durs := make([]time.Duration, 0, iterations)
	start := time.Now()
	for i := 0; i < iterations; i++ {
		t := time.Now()
		rows, err := b.pool.Query(ctx, q)
		if err != nil {
			return durs, time.Since(start), err
		}
		for rows.Next() {
			var userID string
			var totalQty int64
			if err := rows.Scan(&userID, &totalQty); err != nil {
				rows.Close()
				return durs, time.Since(start), err
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return durs, time.Since(start), err
		}
		durs = append(durs, time.Since(t))
	}
	return durs, time.Since(start), nil
}

// Update runs iterations of: set metadata.session = md5(random()) WHERE country='US' LIMIT 1000
// Uses CTE since CockroachDB (like PostgreSQL) has no UPDATE ... LIMIT natively.
func (b *Bench) Update(ctx context.Context, iterations int) ([]time.Duration, time.Duration, error) {
	const q = `
		WITH target AS (
			SELECT id FROM orders
			WHERE doc->'user'->>'country' = 'US'
			LIMIT 1000
		)
		UPDATE orders
		SET doc = jsonb_set(doc, '{metadata,session}', to_jsonb(md5(random()::text)))
		WHERE id IN (SELECT id FROM target)`
	durs := make([]time.Duration, 0, iterations)
	start := time.Now()
	for i := 0; i < iterations; i++ {
		t := time.Now()
		if _, err := b.pool.Exec(ctx, q); err != nil {
			return durs, time.Since(start), err
		}
		durs = append(durs, time.Since(t))
	}
	return durs, time.Since(start), nil
}

// StorageSize returns pg_total_relation_size('orders') in bytes.
// CockroachDB v22.2+ supports this pg_catalog function.
func (b *Bench) StorageSize(ctx context.Context) (int64, error) {
	var size int64
	err := b.pool.QueryRow(ctx, "SELECT pg_total_relation_size('orders')").Scan(&size)
	if err != nil {
		return 0, nil // gracefully degrade if not supported
	}
	return size, nil
}
