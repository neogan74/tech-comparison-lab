package client

import (
	"context"
	"fmt"
	"sync"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
)

// EtcdClient wraps etcd v3 client for benchmarking.
type EtcdClient struct {
	client *clientv3.Client
}

func newEtcdClient(addr string) (*EtcdClient, error) {
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{"http://" + addr},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("etcd connect %s: %w", addr, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = cli.Get(ctx, "ping")
	if err != nil {
		cli.Close()
		return nil, fmt.Errorf("etcd ping %s: %w", addr, err)
	}
	return &EtcdClient{client: cli}, nil
}

func (e *EtcdClient) Close() { e.client.Close() }

// Write benchmarks sequential Put operations across workers.
func (e *EtcdClient) Write(ctx context.Context, count, workers int) ([]time.Duration, time.Duration, error) {
	return runParallel(ctx, count, workers, func(ctx context.Context, i int) (time.Duration, error) {
		key := fmt.Sprintf("bench:write:%d", i)
		val := fmt.Sprintf("value-%d", i)
		start := time.Now()
		_, err := e.client.Put(ctx, key, val)
		return time.Since(start), err
	})
}

// Read benchmarks sequential Get operations across workers.
func (e *EtcdClient) Read(ctx context.Context, count, workers int) ([]time.Duration, time.Duration, error) {
	return runParallel(ctx, count, workers, func(ctx context.Context, i int) (time.Duration, error) {
		key := fmt.Sprintf("bench:write:%d", i%count)
		start := time.Now()
		_, err := e.client.Get(ctx, key)
		return time.Since(start), err
	})
}

// Mixed runs 80% read / 20% write.
func (e *EtcdClient) Mixed(ctx context.Context, count, workers int) ([]time.Duration, time.Duration, error) {
	return runParallel(ctx, count, workers, func(ctx context.Context, i int) (time.Duration, error) {
		key := fmt.Sprintf("bench:write:%d", i%count)
		start := time.Now()
		var err error
		if i%5 == 0 {
			_, err = e.client.Put(ctx, key, fmt.Sprintf("v%d", i))
		} else {
			_, err = e.client.Get(ctx, key)
		}
		return time.Since(start), err
	})
}

// Watch measures time from Put to watch event delivery.
func (e *EtcdClient) Watch(ctx context.Context, count, workers int) ([]time.Duration, time.Duration, error) {
	durs := make([]time.Duration, 0, count)
	var mu sync.Mutex
	start := time.Now()

	for i := 0; i < count; i++ {
		key := fmt.Sprintf("bench:watch:%d", i)
		watchCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		ch := e.client.Watch(watchCtx, key)

		putStart := time.Now()
		if _, err := e.client.Put(ctx, key, "v"); err != nil {
			cancel()
			return nil, 0, fmt.Errorf("watch put: %w", err)
		}

		select {
		case <-ch:
			mu.Lock()
			durs = append(durs, time.Since(putStart))
			mu.Unlock()
		case <-watchCtx.Done():
			cancel()
			return nil, 0, fmt.Errorf("watch timeout on key %s", key)
		}
		cancel()
	}

	return durs, time.Since(start), nil
}

// Election measures leader campaign time using etcd distributed election.
func (e *EtcdClient) Election(ctx context.Context, count int) ([]time.Duration, time.Duration, error) {
	durs := make([]time.Duration, 0, count)
	total := time.Now()

	for i := 0; i < count; i++ {
		electionKey := fmt.Sprintf("/bench/election/%d", i)

		sess, err := concurrency.NewSession(e.client, concurrency.WithTTL(5))
		if err != nil {
			return nil, 0, fmt.Errorf("election session: %w", err)
		}

		el := concurrency.NewElection(sess, electionKey)
		start := time.Now()
		campCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		if err := el.Campaign(campCtx, "leader"); err != nil {
			cancel()
			sess.Close()
			return nil, 0, fmt.Errorf("campaign: %w", err)
		}
		durs = append(durs, time.Since(start))
		cancel()

		// Resign and close so next iteration starts fresh.
		_ = el.Resign(ctx)
		sess.Close()
	}

	return durs, time.Since(total), nil
}

// Cleanup removes benchmark keys.
func (e *EtcdClient) Cleanup(ctx context.Context) error {
	_, err := e.client.Delete(ctx, "bench:", clientv3.WithPrefix())
	return err
}
