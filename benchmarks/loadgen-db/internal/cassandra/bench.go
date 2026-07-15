package cassandra

import (
	"context"
	"fmt"
	"hash/fnv"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gocql/gocql"

	"github.com/tech-comparison-lab/loadgen-db/internal/gen"
)

const bucketCount = 32

var countries = []string{"US", "GB", "DE", "FR", "CA", "AU", "JP"}

// Bench drives Cassandra operations against a query-oriented, bucketed table.
type Bench struct {
	session *gocql.Session
}

// New connects to the comma-separated Cassandra contact points in dsn.
func New(dsn string) (*Bench, error) {
	hosts := strings.Split(dsn, ",")
	for i := range hosts {
		hosts[i] = strings.TrimSpace(hosts[i])
	}
	cluster := gocql.NewCluster(hosts...)
	cluster.Keyspace = "bench"
	cluster.Consistency = gocql.LocalQuorum
	cluster.ConnectTimeout = 15 * time.Second
	cluster.Timeout = 30 * time.Second
	cluster.NumConns = 2
	session, err := cluster.CreateSession()
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	return &Bench{session: session}, nil
}

func (b *Bench) Close() { b.session.Close() }

func (b *Bench) Truncate(ctx context.Context) error {
	return b.session.Query("TRUNCATE bench.orders_by_country").WithContext(ctx).Exec()
}

// Insert writes independent rows concurrently. batchSize controls work chunks,
// not CQL BATCH statements (cross-partition batches are an anti-pattern).
func (b *Bench) Insert(ctx context.Context, count, batchSize, workers int) ([]time.Duration, time.Duration, error) {
	type job struct{ docs []gen.Order }
	jobs := make(chan job, workers*2)
	durs := make([]time.Duration, 0, (count+batchSize-1)/batchSize)
	var mu sync.Mutex
	var wg sync.WaitGroup
	var firstErr error

	stmt := `INSERT INTO bench.orders_by_country
		(country, bucket, id, user_id, user_tier, product_id, product_category,
		 product_price, quantity, status, tags, metadata_source, metadata_session,
		 created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				t := time.Now()
				for _, order := range j.docs {
					if err := b.session.Query(stmt,
						order.User.Country, bucket(order.ID), order.ID,
						order.User.ID, order.User.Tier, order.Product.ID, order.Product.Category,
						order.Product.Price, order.Quantity, order.Status, order.Tags,
						order.Metadata.Source, order.Metadata.Session, order.CreatedAt, order.UpdatedAt,
					).WithContext(ctx).Exec(); err != nil {
						mu.Lock()
						if firstErr == nil {
							firstErr = err
						}
						mu.Unlock()
						break
					}
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
		jobs <- job{docs: gen.Batch(size)}
		remaining -= size
	}
	close(jobs)
	wg.Wait()
	return durs, time.Since(start), firstErr
}

// Query returns up to 100 US order IDs, fanning out over bucket partitions.
func (b *Bench) Query(ctx context.Context, iterations int) ([]time.Duration, time.Duration, error) {
	durs := make([]time.Duration, 0, iterations)
	start := time.Now()
	for i := 0; i < iterations; i++ {
		t := time.Now()
		remaining := 100
		for bucketID := 0; bucketID < bucketCount && remaining > 0; bucketID++ {
			iter := b.session.Query(
				"SELECT id FROM bench.orders_by_country WHERE country = ? AND bucket = ? LIMIT ?",
				"US", bucketID, remaining,
			).WithContext(ctx).Iter()
			var id string
			for iter.Scan(&id) {
				remaining--
			}
			if err := iter.Close(); err != nil {
				return durs, time.Since(start), err
			}
		}
		durs = append(durs, time.Since(t))
	}
	return durs, time.Since(start), nil
}

// Agg computes the global top users by quantity. Cassandra cannot efficiently
// perform this cross-partition aggregation, so rows are streamed and reduced in
// the client; that cost is intentionally included in the comparison.
func (b *Bench) Agg(ctx context.Context, iterations int) ([]time.Duration, time.Duration, error) {
	durs := make([]time.Duration, 0, iterations)
	start := time.Now()
	for i := 0; i < iterations; i++ {
		t := time.Now()
		totals := make(map[string]int)
		for _, country := range countries {
			for bucketID := 0; bucketID < bucketCount; bucketID++ {
				iter := b.session.Query(
					"SELECT user_id, quantity FROM bench.orders_by_country WHERE country = ? AND bucket = ?",
					country, bucketID,
				).WithContext(ctx).PageSize(5000).Iter()
				var userID string
				var quantity int
				for iter.Scan(&userID, &quantity) {
					totals[userID] += quantity
				}
				if err := iter.Close(); err != nil {
					return durs, time.Since(start), err
				}
			}
		}
		values := make([]int, 0, len(totals))
		for _, total := range totals {
			values = append(values, total)
		}
		sort.Sort(sort.Reverse(sort.IntSlice(values)))
		if len(values) > 100 {
			values = values[:100]
		}
		durs = append(durs, time.Since(t))
	}
	return durs, time.Since(start), nil
}

// Update changes metadata_session for up to 1000 US rows per iteration.
func (b *Bench) Update(ctx context.Context, iterations int) ([]time.Duration, time.Duration, error) {
	type key struct {
		bucket int
		id     string
	}
	keys := make([]key, 0, 1000)
	for bucketID := 0; bucketID < bucketCount && len(keys) < 1000; bucketID++ {
		iter := b.session.Query(
			"SELECT id FROM bench.orders_by_country WHERE country = ? AND bucket = ? LIMIT ?",
			"US", bucketID, 1000-len(keys),
		).WithContext(ctx).Iter()
		var id string
		for iter.Scan(&id) {
			keys = append(keys, key{bucket: bucketID, id: id})
		}
		if err := iter.Close(); err != nil {
			return nil, 0, err
		}
	}

	durs := make([]time.Duration, 0, iterations)
	start := time.Now()
	for i := 0; i < iterations; i++ {
		t := time.Now()
		for _, k := range keys {
			if err := b.session.Query(
				"UPDATE bench.orders_by_country SET metadata_session = ?, updated_at = ? WHERE country = ? AND bucket = ? AND id = ?",
				gocql.TimeUUID().String(), time.Now().UTC(), "US", k.bucket, k.id,
			).WithContext(ctx).Exec(); err != nil {
				return durs, time.Since(start), err
			}
		}
		durs = append(durs, time.Since(t))
	}
	return durs, time.Since(start), nil
}

func bucket(id string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(id))
	return int(h.Sum32() % bucketCount)
}
