package client

import (
	"context"
	"fmt"
	mrand "math/rand"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// Bench wraps a go-redis client for benchmark operations.
// Works with both Redis and Valkey (same RESP protocol).
type Bench struct {
	rdb  *redis.Client
	name string // "redis" or "valkey"
}

// New connects to addr and pings the server.
func New(name, addr string) (*Bench, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:         addr,
		PoolSize:     64,
		MinIdleConns: 16,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("%s ping %s: %w", name, addr, err)
	}
	return &Bench{rdb: rdb, name: name}, nil
}

// Close releases the connection pool.
func (b *Bench) Close() error { return b.rdb.Close() }

// FlushAll removes all keys from the database.
func (b *Bench) FlushAll(ctx context.Context) error {
	return b.rdb.FlushAll(ctx).Err()
}

// MemoryUsed returns used_memory_human from INFO memory.
func (b *Bench) MemoryUsed(ctx context.Context) (string, error) {
	info, err := b.rdb.Info(ctx, "memory").Result()
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(info, "\n") {
		if strings.HasPrefix(line, "used_memory_human:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "used_memory_human:")), nil
		}
	}
	return "", nil
}

// KeyCount returns the number of keys in db0 from INFO keyspace.
func (b *Bench) KeyCount(ctx context.Context) (int64, error) {
	info, err := b.rdb.Info(ctx, "keyspace").Result()
	if err != nil {
		return 0, err
	}
	// Format: "db0:keys=10000000,expires=0,avg_ttl=0"
	for _, line := range strings.Split(info, "\n") {
		if strings.HasPrefix(line, "db0:") {
			var keys int64
			fmt.Sscanf(strings.TrimPrefix(line, "db0:keys="), "%d", &keys)
			return keys, nil
		}
	}
	return 0, nil
}

// benchKey returns the canonical key for index i.
func benchKey(i int) string { return fmt.Sprintf("bench:%010d", i) }

// benchValue returns a ~200-byte JSON value for key i.
func benchValue(i int) string {
	return fmt.Sprintf(
		`{"id":"bench:%010d","user_id":"u%09d","score":%d,"tier":"%s","tags":["sale","new"],"ts":"2024-01-01T00:00:00Z","meta":"padding-padding-padding-padding-00"}`,
		i, mrand.Intn(1000000), mrand.Intn(1000),
		[3]string{"free", "basic", "premium"}[mrand.Intn(3)],
	)
}

// Set runs count individual SET commands across workers goroutines.
// Worker i owns keys [i*chunk .. (i+1)*chunk).
func (b *Bench) Set(ctx context.Context, count, workers int) ([]time.Duration, time.Duration, error) {
	return b.runWorkers(workers, count, func(wid, start, end int, durs chan<- time.Duration) error {
		for i := start; i < end; i++ {
			t := time.Now()
			if err := b.rdb.Set(ctx, benchKey(i), benchValue(i), 0).Err(); err != nil {
				return err
			}
			durs <- time.Since(t)
		}
		return nil
	})
}

// Get runs iterations random GET commands across workers goroutines.
// Assumes count keys exist (set before calling).
func (b *Bench) Get(ctx context.Context, iterations, workers, totalKeys int) ([]time.Duration, time.Duration, error) {
	iterPerWorker := iterations / workers
	return b.runWorkers(workers, iterations, func(wid, _, _ int, durs chan<- time.Duration) error {
		for i := 0; i < iterPerWorker; i++ {
			key := benchKey(mrand.Intn(totalKeys))
			t := time.Now()
			if err := b.rdb.Get(ctx, key).Err(); err != nil && err != redis.Nil {
				return err
			}
			durs <- time.Since(t)
		}
		return nil
	})
}

// PipelineSet inserts count keys using pipeSize-command pipelines across workers.
// Each worker owns a distinct key range to avoid duplication.
func (b *Bench) PipelineSet(ctx context.Context, count, pipeSize, workers int) ([]time.Duration, time.Duration, error) {
	return b.runWorkers(workers, count/pipeSize, func(wid, start, end int, durs chan<- time.Duration) error {
		keyBase := wid * (count / workers)
		idx := 0
		for batch := start; batch < end; batch++ {
			pipe := b.rdb.Pipeline()
			for j := 0; j < pipeSize; j++ {
				k := keyBase + idx
				pipe.Set(ctx, benchKey(k), benchValue(k), 0)
				idx++
			}
			t := time.Now()
			if _, err := pipe.Exec(ctx); err != nil {
				return err
			}
			durs <- time.Since(t)
		}
		return nil
	})
}

// PipelineGet runs iterations pipelines of pipeSize random GETs across workers.
func (b *Bench) PipelineGet(ctx context.Context, iterations, pipeSize, workers, totalKeys int) ([]time.Duration, time.Duration, error) {
	iterPerWorker := iterations / workers
	return b.runWorkers(workers, iterations, func(wid, _, _ int, durs chan<- time.Duration) error {
		for i := 0; i < iterPerWorker; i++ {
			pipe := b.rdb.Pipeline()
			for j := 0; j < pipeSize; j++ {
				pipe.Get(ctx, benchKey(mrand.Intn(totalKeys)))
			}
			t := time.Now()
			if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
				return err
			}
			durs <- time.Since(t)
		}
		return nil
	})
}

// Mixed runs iterations pipelines of pipeSize commands (80% GET, 20% SET) across workers.
func (b *Bench) Mixed(ctx context.Context, iterations, pipeSize, workers, totalKeys int) ([]time.Duration, time.Duration, error) {
	iterPerWorker := iterations / workers
	setThreshold := pipeSize * 20 / 100 // 20% SETs
	setCounter := 0
	var mu sync.Mutex

	return b.runWorkers(workers, iterations, func(wid, _, _ int, durs chan<- time.Duration) error {
		for i := 0; i < iterPerWorker; i++ {
			pipe := b.rdb.Pipeline()
			for j := 0; j < pipeSize; j++ {
				mu.Lock()
				isSet := setCounter < setThreshold
				setCounter = (setCounter + 1) % pipeSize
				mu.Unlock()
				if isSet {
					k := mrand.Intn(totalKeys)
					pipe.Set(ctx, benchKey(k), benchValue(k), 0)
				} else {
					pipe.Get(ctx, benchKey(mrand.Intn(totalKeys)))
				}
			}
			t := time.Now()
			if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
				return err
			}
			durs <- time.Since(t)
		}
		return nil
	})
}

// runWorkers runs fn across workers goroutines and collects durations.
// totalUnits is divided evenly across workers for key range calculation.
func (b *Bench) runWorkers(
	workers, totalUnits int,
	fn func(wid, start, end int, durs chan<- time.Duration) error,
) ([]time.Duration, time.Duration, error) {
	chunk := totalUnits / workers
	durs := make(chan time.Duration, totalUnits)
	var wg sync.WaitGroup
	errs := make(chan error, workers)

	start := time.Now()
	for w := 0; w < workers; w++ {
		wg.Add(1)
		wStart := w * chunk
		wEnd := wStart + chunk
		if w == workers-1 {
			wEnd = totalUnits
		}
		go func(wid, s, e int) {
			defer wg.Done()
			if err := fn(wid, s, e, durs); err != nil {
				errs <- err
			}
		}(w, wStart, wEnd)
	}
	wg.Wait()
	total := time.Since(start)
	close(durs)
	close(errs)

	var firstErr error
	for err := range errs {
		if firstErr == nil {
			firstErr = err
		}
	}

	result := make([]time.Duration, 0, len(durs))
	for d := range durs {
		result = append(result, d)
	}
	return result, total, firstErr
}
