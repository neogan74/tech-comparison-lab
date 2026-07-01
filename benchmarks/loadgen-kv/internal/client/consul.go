package client

import (
	"context"
	"fmt"
	"time"

	consulapi "github.com/hashicorp/consul/api"
)

// ConsulClient wraps Consul HTTP client for benchmarking.
type ConsulClient struct {
	client *consulapi.Client
}

func newConsulClient(addr string) (*ConsulClient, error) {
	cfg := consulapi.DefaultConfig()
	cfg.Address = addr
	cli, err := consulapi.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("consul connect %s: %w", addr, err)
	}
	// Verify connectivity.
	_, err = cli.Status().Leader()
	if err != nil {
		return nil, fmt.Errorf("consul ping %s: %w", addr, err)
	}
	return &ConsulClient{client: cli}, nil
}

func (c *ConsulClient) Close() {}

// Write benchmarks KV Put operations.
func (c *ConsulClient) Write(ctx context.Context, count, workers int) ([]time.Duration, time.Duration, error) {
	kv := c.client.KV()
	return runParallel(ctx, count, workers, func(ctx context.Context, i int) (time.Duration, error) {
		p := &consulapi.KVPair{
			Key:   fmt.Sprintf("bench/write/%d", i),
			Value: []byte(fmt.Sprintf("value-%d", i)),
		}
		start := time.Now()
		_, err := kv.Put(p, nil)
		return time.Since(start), err
	})
}

// Read benchmarks KV Get operations.
func (c *ConsulClient) Read(ctx context.Context, count, workers int) ([]time.Duration, time.Duration, error) {
	kv := c.client.KV()
	return runParallel(ctx, count, workers, func(ctx context.Context, i int) (time.Duration, error) {
		key := fmt.Sprintf("bench/write/%d", i%count)
		start := time.Now()
		_, _, err := kv.Get(key, nil)
		return time.Since(start), err
	})
}

// Mixed runs 80% read / 20% write.
func (c *ConsulClient) Mixed(ctx context.Context, count, workers int) ([]time.Duration, time.Duration, error) {
	kv := c.client.KV()
	return runParallel(ctx, count, workers, func(ctx context.Context, i int) (time.Duration, error) {
		key := fmt.Sprintf("bench/write/%d", i%count)
		start := time.Now()
		var err error
		if i%5 == 0 {
			_, err = kv.Put(&consulapi.KVPair{Key: key, Value: []byte(fmt.Sprintf("v%d", i))}, nil)
		} else {
			_, _, err = kv.Get(key, nil)
		}
		return time.Since(start), err
	})
}

// Watch measures time from Put to watch event delivery using blocking query.
func (c *ConsulClient) Watch(ctx context.Context, count, workers int) ([]time.Duration, time.Duration, error) {
	kv := c.client.KV()
	durs := make([]time.Duration, 0, count)
	total := time.Now()

	for i := 0; i < count; i++ {
		key := fmt.Sprintf("bench/watch/%d", i)

		// Get current index before the put.
		_, meta, err := kv.Get(key, nil)
		if err != nil {
			return nil, 0, fmt.Errorf("watch pre-get: %w", err)
		}
		waitIndex := uint64(0)
		if meta != nil {
			waitIndex = meta.LastIndex
		}

		putStart := time.Now()
		if _, err := kv.Put(&consulapi.KVPair{Key: key, Value: []byte("v")}, nil); err != nil {
			return nil, 0, fmt.Errorf("watch put: %w", err)
		}

		// Blocking query — returns when index changes (i.e. our put landed).
		q := &consulapi.QueryOptions{
			WaitIndex: waitIndex,
			WaitTime:  5 * time.Second,
		}
		_, _, err = kv.Get(key, q)
		if err != nil {
			return nil, 0, fmt.Errorf("watch block-get: %w", err)
		}
		durs = append(durs, time.Since(putStart))
	}

	return durs, time.Since(total), nil
}

// Election measures lock acquire time using Consul Session + Lock.
func (c *ConsulClient) Election(ctx context.Context, count int) ([]time.Duration, time.Duration, error) {
	durs := make([]time.Duration, 0, count)
	total := time.Now()
	session := c.client.Session()
	kv := c.client.KV()

	for i := 0; i < count; i++ {
		sessID, _, err := session.Create(&consulapi.SessionEntry{
			TTL:      "10s",
			Behavior: consulapi.SessionBehaviorDelete,
		}, nil)
		if err != nil {
			return nil, 0, fmt.Errorf("election session create: %w", err)
		}

		lockKey := fmt.Sprintf("bench/election/%d", i)
		p := &consulapi.KVPair{
			Key:     lockKey,
			Session: sessID,
		}

		start := time.Now()
		acquired, _, err := kv.Acquire(p, nil)
		if err != nil {
			_, _ = session.Destroy(sessID, nil)
			return nil, 0, fmt.Errorf("lock acquire: %w", err)
		}
		elapsed := time.Since(start)

		if !acquired {
			_, _ = session.Destroy(sessID, nil)
			return nil, 0, fmt.Errorf("lock acquire returned false for key %s", lockKey)
		}
		durs = append(durs, elapsed)

		// Release: destroy session (deletes key due to SessionBehaviorDelete).
		_, _ = session.Destroy(sessID, nil)
	}

	return durs, time.Since(total), nil
}

// Cleanup removes benchmark keys.
func (c *ConsulClient) Cleanup(ctx context.Context) error {
	_, err := c.client.KV().DeleteTree("bench/", nil)
	return err
}

