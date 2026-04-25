package client

import (
	"bufio"
	"context"
	"fmt"
	mrand "math/rand"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bradfitz/gomemcache/memcache"
)

// MemcachedBench wraps a gomemcache client for benchmark operations.
type MemcachedBench struct {
	mc   *memcache.Client
	addr string
}

// newMemcachedBench connects to addr and pings the server.
func newMemcachedBench(addr string) (*MemcachedBench, error) {
	mc := memcache.New(addr)
	mc.Timeout = 5 * time.Second
	mc.MaxIdleConns = 64
	if err := mc.Ping(); err != nil {
		return nil, fmt.Errorf("memcached ping %s: %w", addr, err)
	}
	return &MemcachedBench{mc: mc, addr: addr}, nil
}

// Close is a no-op for gomemcache (connections are pooled internally).
func (b *MemcachedBench) Close() error { return nil }

// FlushAll removes all keys from the server.
func (b *MemcachedBench) FlushAll(_ context.Context) error {
	return b.mc.FlushAll()
}

// rawStats fetches Memcached stats via a raw TCP connection.
func (b *MemcachedBench) rawStats() (map[string]string, error) {
	conn, err := net.DialTimeout("tcp", b.addr, 3*time.Second)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(3 * time.Second))

	fmt.Fprintf(conn, "stats\r\n")
	result := make(map[string]string)
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "END" {
			break
		}
		parts := strings.SplitN(line, " ", 3)
		if len(parts) == 3 && parts[0] == "STAT" {
			result[parts[1]] = parts[2]
		}
	}
	return result, nil
}

// MemoryUsed returns bytes used from Memcached stats.
func (b *MemcachedBench) MemoryUsed(_ context.Context) (string, error) {
	stats, err := b.rawStats()
	if err != nil {
		return "", err
	}
	if v, ok := stats["bytes"]; ok {
		n, _ := strconv.ParseInt(v, 10, 64)
		return formatBytes(n), nil
	}
	return "", nil
}

// KeyCount returns curr_items from Memcached stats.
func (b *MemcachedBench) KeyCount(_ context.Context) (int64, error) {
	stats, err := b.rawStats()
	if err != nil {
		return 0, err
	}
	if v, ok := stats["curr_items"]; ok {
		n, _ := strconv.ParseInt(v, 10, 64)
		return n, nil
	}
	return 0, nil
}

// Set runs count individual SET commands across workers goroutines.
func (b *MemcachedBench) Set(_ context.Context, count, workers int) ([]time.Duration, time.Duration, error) {
	return b.runWorkers(workers, count, func(_, start, end int, durs chan<- time.Duration) error {
		for i := start; i < end; i++ {
			t := time.Now()
			if err := b.mc.Set(&memcache.Item{Key: benchKey(i), Value: []byte(benchValue(i))}); err != nil {
				return err
			}
			durs <- time.Since(t)
		}
		return nil
	})
}

// Get runs iterations random GET commands across workers goroutines.
func (b *MemcachedBench) Get(_ context.Context, iterations, workers, totalKeys int) ([]time.Duration, time.Duration, error) {
	iterPerWorker := iterations / workers
	return b.runWorkers(workers, iterations, func(_, _, _ int, durs chan<- time.Duration) error {
		for i := 0; i < iterPerWorker; i++ {
			key := benchKey(mrand.Intn(totalKeys))
			t := time.Now()
			_, _ = b.mc.Get(key) // cache miss on unknown key is ok
			durs <- time.Since(t)
		}
		return nil
	})
}

// PipelineSet inserts count keys in parallel across workers.
// Memcached has no write pipeline; this is equivalent to Set with workers.
func (b *MemcachedBench) PipelineSet(_ context.Context, count, _, workers int) ([]time.Duration, time.Duration, error) {
	return b.runWorkers(workers, count, func(_, start, end int, durs chan<- time.Duration) error {
		for i := start; i < end; i++ {
			t := time.Now()
			if err := b.mc.Set(&memcache.Item{Key: benchKey(i), Value: []byte(benchValue(i))}); err != nil {
				return err
			}
			durs <- time.Since(t)
		}
		return nil
	})
}

// PipelineGet runs iterations multi-GET batches of pipeSize keys across workers.
// Uses Memcached's native GetMulti for efficient batched retrieval.
func (b *MemcachedBench) PipelineGet(_ context.Context, iterations, pipeSize, workers, totalKeys int) ([]time.Duration, time.Duration, error) {
	iterPerWorker := iterations / workers
	totalCmds := iterations * pipeSize
	return b.runWorkers(workers, totalCmds, func(_, _, _ int, durs chan<- time.Duration) error {
		for i := 0; i < iterPerWorker; i++ {
			keys := make([]string, pipeSize)
			for j := range keys {
				keys[j] = benchKey(mrand.Intn(totalKeys))
			}
			t := time.Now()
			_, _ = b.mc.GetMulti(keys)
			durs <- time.Since(t)
		}
		return nil
	})
}

// Mixed runs iterations batches with 80% GET and 20% SET across workers.
func (b *MemcachedBench) Mixed(_ context.Context, iterations, pipeSize, workers, totalKeys int) ([]time.Duration, time.Duration, error) {
	iterPerWorker := iterations / workers
	totalCmds := iterations * pipeSize
	setThreshold := pipeSize * 20 / 100 // 20% SETs per batch

	return b.runWorkers(workers, totalCmds, func(_, _, _ int, durs chan<- time.Duration) error {
		for i := 0; i < iterPerWorker; i++ {
			getKeys := make([]string, 0, pipeSize-setThreshold)
			for j := 0; j < pipeSize; j++ {
				if j < setThreshold {
					k := mrand.Intn(totalKeys)
					t := time.Now()
					_ = b.mc.Set(&memcache.Item{Key: benchKey(k), Value: []byte(benchValue(k))})
					durs <- time.Since(t)
				} else {
					getKeys = append(getKeys, benchKey(mrand.Intn(totalKeys)))
				}
			}
			if len(getKeys) > 0 {
				t := time.Now()
				_, _ = b.mc.GetMulti(getKeys)
				// attribute the multiget duration as one measurement per key
				elapsed := time.Since(t)
				perKey := elapsed / time.Duration(len(getKeys))
				for range getKeys {
					durs <- perKey
				}
			}
		}
		return nil
	})
}

// runWorkers runs fn across workers goroutines and collects durations.
func (b *MemcachedBench) runWorkers(
	workers, totalUnits int,
	fn func(wid, start, end int, durs chan<- time.Duration) error,
) ([]time.Duration, time.Duration, error) {
	chunk := totalUnits / workers
	if chunk < 1 {
		chunk = 1
	}
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

// formatBytes converts bytes to human-readable string.
func formatBytes(n int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
	)
	switch {
	case n >= mb:
		return fmt.Sprintf("%.2fM", float64(n)/mb)
	case n >= kb:
		return fmt.Sprintf("%.2fK", float64(n)/kb)
	default:
		return fmt.Sprintf("%dB", n)
	}
}
