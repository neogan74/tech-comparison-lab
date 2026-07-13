package client

import (
	"context"
	"time"
)

// Client is the common interface for cache benchmark clients.
type Client interface {
	Close() error
	FlushAll(ctx context.Context) error
	MemoryUsed(ctx context.Context) (string, error)
	KeyCount(ctx context.Context) (int64, error)
	Set(ctx context.Context, count, workers int) ([]time.Duration, time.Duration, error)
	Get(ctx context.Context, iterations, workers, totalKeys int) ([]time.Duration, time.Duration, error)
	PipelineSet(ctx context.Context, count, pipeSize, workers int) ([]time.Duration, time.Duration, error)
	PipelineGet(ctx context.Context, iterations, pipeSize, workers, totalKeys int) ([]time.Duration, time.Duration, error)
	Mixed(ctx context.Context, iterations, pipeSize, workers, totalKeys int) ([]time.Duration, time.Duration, error)
}

// New returns a Client for the given db name and address.
// Supported db values: "redis", "valkey", "dragonfly", "memcached".
func New(name, addr string) (Client, error) {
	switch name {
	case "redis", "valkey", "dragonfly":
		return newRedisBench(name, addr)
	case "memcached":
		return newMemcachedBench(addr)
	default:
		return nil, nil
	}
}
