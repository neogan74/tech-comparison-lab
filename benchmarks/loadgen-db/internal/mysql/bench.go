package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/tech-comparison-lab/loadgen-db/internal/gen"
)

// Bench drives MySQL benchmark operations via database/sql.
type Bench struct {
	db *sql.DB
}

// New opens a *sql.DB connection pool and pings the server.
func New(ctx context.Context, dsn string) (*Bench, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}
	db.SetMaxOpenConns(32)
	db.SetMaxIdleConns(16)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return &Bench{db: db}, nil
}

// Close releases all pool connections.
func (b *Bench) Close() { b.db.Close() }

// Truncate removes all rows from the orders table.
func (b *Bench) Truncate(ctx context.Context) error {
	_, err := b.db.ExecContext(ctx, "TRUNCATE TABLE orders")
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

// insertBatch performs a multi-row INSERT IGNORE for a batch of orders.
func (b *Bench) insertBatch(ctx context.Context, docs []gen.Order) error {
	if len(docs) == 0 {
		return nil
	}
	// Build: INSERT IGNORE INTO orders(id, doc, created_at) VALUES (?,?,?),(?,?,?),...
	placeholders := make([]string, len(docs))
	args := make([]any, 0, len(docs)*3)
	for i, d := range docs {
		placeholders[i] = "(?,?,?)"
		docJSON, err := json.Marshal(d)
		if err != nil {
			return err
		}
		args = append(args, d.ID, string(docJSON), d.CreatedAt.Format("2006-01-02 15:04:05.999999"))
	}
	q := "INSERT IGNORE INTO orders(id, doc, created_at) VALUES " + strings.Join(placeholders, ",")
	_, err := b.db.ExecContext(ctx, q, args...)
	return err
}

// Query runs iterations of: SELECT id, doc FROM orders WHERE user.country = 'US' LIMIT 100
// Rows are fully iterated to include network + deserialization cost.
func (b *Bench) Query(ctx context.Context, iterations int) ([]time.Duration, time.Duration, error) {
	const q = `SELECT id, doc FROM orders WHERE doc->>'$.user.country' = 'US' LIMIT 100`
	durs := make([]time.Duration, 0, iterations)
	start := time.Now()
	for i := 0; i < iterations; i++ {
		t := time.Now()
		rows, err := b.db.QueryContext(ctx, q)
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
		SELECT doc->>'$.user.id' AS user_id,
		       SUM(CAST(doc->>'$.quantity' AS UNSIGNED)) AS total_qty
		FROM orders
		GROUP BY 1
		ORDER BY 2 DESC
		LIMIT 100`
	durs := make([]time.Duration, 0, iterations)
	start := time.Now()
	for i := 0; i < iterations; i++ {
		t := time.Now()
		rows, err := b.db.QueryContext(ctx, q)
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

// Update runs iterations of: SET metadata.session = MD5(RAND()) WHERE country='US' LIMIT 1000
func (b *Bench) Update(ctx context.Context, iterations int) ([]time.Duration, time.Duration, error) {
	const q = `
		UPDATE orders
		SET doc = JSON_SET(doc, '$.metadata.session', MD5(RAND()))
		WHERE doc->>'$.user.country' = 'US'
		LIMIT 1000`
	durs := make([]time.Duration, 0, iterations)
	start := time.Now()
	for i := 0; i < iterations; i++ {
		t := time.Now()
		if _, err := b.db.ExecContext(ctx, q); err != nil {
			return durs, time.Since(start), err
		}
		durs = append(durs, time.Since(t))
	}
	return durs, time.Since(start), nil
}

// StorageSize returns data_length + index_length for the orders table in bytes.
func (b *Bench) StorageSize(ctx context.Context) (int64, error) {
	const q = `
		SELECT COALESCE(data_length + index_length, 0)
		FROM information_schema.TABLES
		WHERE table_schema = DATABASE() AND table_name = 'orders'`
	var size int64
	err := b.db.QueryRowContext(ctx, q).Scan(&size)
	return size, err
}
