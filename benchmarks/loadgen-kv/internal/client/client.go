package client

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// KVClient is the common interface for etcd and consul benchmark clients.
type KVClient interface {
	Write(ctx context.Context, count, workers int) ([]time.Duration, time.Duration, error)
	Read(ctx context.Context, count, workers int) ([]time.Duration, time.Duration, error)
	Mixed(ctx context.Context, count, workers int) ([]time.Duration, time.Duration, error)
	Watch(ctx context.Context, count, workers int) ([]time.Duration, time.Duration, error)
	Election(ctx context.Context, count int) ([]time.Duration, time.Duration, error)
	Cleanup(ctx context.Context) error
	Close()
}

// New creates a KVClient for the given db name and address.
func New(db, addr string) (KVClient, error) {
	switch db {
	case "etcd":
		return newEtcdClient(addr)
	case "consul":
		return newConsulClient(addr)
	default:
		return nil, fmt.Errorf("unknown db %q: want etcd|consul", db)
	}
}

// runParallel distributes count operations across workers goroutines and
// collects per-operation durations. Returns (durations, total wall time, error).
func runParallel(ctx context.Context, count, workers int, fn func(ctx context.Context, i int) (time.Duration, error)) ([]time.Duration, time.Duration, error) {
	durs := make([]time.Duration, count)
	errCh := make(chan error, workers)
	jobs := make(chan int, count)

	for i := 0; i < count; i++ {
		jobs <- i
	}
	close(jobs)

	var wg sync.WaitGroup
	start := time.Now()
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				d, err := fn(ctx, i)
				if err != nil {
					select {
					case errCh <- err:
					default:
					}
					return
				}
				durs[i] = d
			}
		}()
	}
	wg.Wait()
	total := time.Since(start)

	select {
	case err := <-errCh:
		return nil, 0, err
	default:
	}
	return durs, total, nil
}
