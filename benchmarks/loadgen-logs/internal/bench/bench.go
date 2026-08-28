// Package bench drives the log-ingest + log-query workload shared by the Loki
// and Elasticsearch benchmark targets.
package bench

import (
	"context"
	mrand "math/rand"
	"sync"
	"time"

	"github.com/tech-comparison-lab/loadgen-logs/internal/gen"
	"github.com/tech-comparison-lab/loadgen-logs/internal/logs"
)

// Bench orchestrates ingest and query operations against one backend
// ("loki" | "elasticsearch").
type Bench struct {
	db       string
	client   *logs.Client
	services []string
	window   time.Duration
	limit    int
}

// New creates a Bench. addr is the backend base URL, servicesN the number of
// distinct synthetic services, window the time span entries are spread over
// and queries look back across, limit the page size for line-returning
// queries.
func New(db, addr string, servicesN int, window time.Duration, limit int) *Bench {
	return &Bench{
		db:       db,
		client:   logs.New(db, addr),
		services: gen.Services(servicesN),
		window:   window,
		limit:    limit,
	}
}

// Service returns the primary service used for query benchmarks.
func (b *Bench) Service() string { return b.services[0] }

// Ping verifies the backend is reachable.
func (b *Bench) Ping(ctx context.Context) error { return b.client.Ping(ctx) }

// Setup prepares backend-side schema (Elasticsearch index + mapping).
func (b *Bench) Setup(ctx context.Context) error { return b.client.EnsureIndex(ctx) }

// Ingest generates count log entries and pushes them via workers concurrent
// goroutines, batchSize entries per request. Entries are generated per batch to
// keep memory bounded for large runs. It returns per-request durations, the
// wall-clock total, and the first error encountered.
func (b *Bench) Ingest(ctx context.Context, count, batchSize, workers int) ([]time.Duration, time.Duration, error) {
	nowNs := time.Now().UnixNano()
	windowNs := b.window.Nanoseconds()

	type job struct{ start, end int }
	jobs := make(chan job, workers*2)

	var mu sync.Mutex
	durs := make([]time.Duration, 0, count/batchSize+1)
	var firstErr error
	var wg sync.WaitGroup

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(seed int64) {
			defer wg.Done()
			rnd := mrand.New(mrand.NewSource(seed))
			for j := range jobs {
				batch := make([]logs.Entry, 0, j.end-j.start)
				for i := j.start; i < j.end; i++ {
					batch = append(batch, gen.Entry(i, b.services, nowNs, windowNs, rnd))
				}
				t0 := time.Now()
				if err := b.client.Push(ctx, batch); err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					mu.Unlock()
					continue
				}
				d := time.Since(t0)
				mu.Lock()
				durs = append(durs, d)
				mu.Unlock()
			}
		}(int64(w + 1))
	}

	start := time.Now()
	for i := 0; i < count; i += batchSize {
		end := i + batchSize
		if end > count {
			end = count
		}
		jobs <- job{start: i, end: end}
	}
	close(jobs)
	wg.Wait()
	return durs, time.Since(start), firstErr
}

// Flush makes ingested entries queryable (Elasticsearch index refresh).
func (b *Bench) Flush(ctx context.Context) error { return b.client.Flush(ctx) }

// Indexed reports how many entries are queryable for the primary service,
// letting the caller fail loudly when ingest silently dropped data.
func (b *Bench) Indexed(ctx context.Context) (int64, error) {
	return b.client.CountIngested(ctx, b.Service(), b.window)
}

func repeat(ctx context.Context, iters int, fn func(context.Context) error) ([]time.Duration, time.Duration, error) {
	durs := make([]time.Duration, 0, iters)
	start := time.Now()
	for i := 0; i < iters; i++ {
		t0 := time.Now()
		if err := fn(ctx); err != nil {
			return durs, time.Since(start), err
		}
		durs = append(durs, time.Since(t0))
	}
	return durs, time.Since(start), nil
}

// LabelValues benchmarks enumerating distinct service values: a Loki label
// index lookup against an Elasticsearch terms aggregation.
func (b *Bench) LabelValues(ctx context.Context, iters int) ([]time.Duration, time.Duration, error) {
	return repeat(ctx, iters, func(ctx context.Context) error {
		return b.client.LabelValues(ctx, b.window)
	})
}

// QueryRange benchmarks tailing the most recent lines for one service.
func (b *Bench) QueryRange(ctx context.Context, iters int) ([]time.Duration, time.Duration, error) {
	return repeat(ctx, iters, func(ctx context.Context) error {
		return b.client.QueryRange(ctx, b.Service(), b.limit, b.window)
	})
}

// FilterMatch benchmarks searching one service's lines for a token: Loki's
// brute-force line filter against Elasticsearch's inverted index.
func (b *Bench) FilterMatch(ctx context.Context, iters int) ([]time.Duration, time.Duration, error) {
	return repeat(ctx, iters, func(ctx context.Context) error {
		return b.client.FilterMatch(ctx, b.Service(), gen.FilterToken, b.limit, b.window)
	})
}

// CountOverTime benchmarks counting one service's lines over the window.
func (b *Bench) CountOverTime(ctx context.Context, iters int) ([]time.Duration, time.Duration, error) {
	return repeat(ctx, iters, func(ctx context.Context) error {
		return b.client.CountOverTime(ctx, b.Service(), b.window)
	})
}
