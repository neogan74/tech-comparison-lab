// Package bench defines the Backend interface shared by the Prometheus and
// Zabbix benchmark targets, plus the set of query operations both must serve.
package bench

import (
	"context"
	"time"
)

// Op names the read workloads exercised against both backends. Each maps to a
// PromQL expression on the Prometheus side and to a JSON-RPC API call on the
// Zabbix side (see the per-backend implementations for the exact mapping).
const (
	OpLatest   = "latest"   // most recent value of every series
	OpFiltered = "filtered" // most recent value of the us-east-1 subset (~1/5)
	OpHistory  = "history"  // raw samples over a 30m window for a bounded set
)

// Ops is the ordered list of read operations run by --op all.
var Ops = []string{OpLatest, OpFiltered, OpHistory}

// Backend drives ingest and query operations against one monitoring system.
// Implementations are Prometheus (remote_write + PromQL) and Zabbix (sender
// protocol + history/item API).
type Backend interface {
	// Name returns the backend identifier used in result rows ("prometheus"
	// or "zabbix").
	Name() string

	// Ping verifies connectivity without mutating state.
	Ping(ctx context.Context) error

	// Provision prepares the backend to accept seriesN unique series. It is a
	// no-op for Prometheus (schemaless) and creates a host plus seriesN trapper
	// items for Zabbix.
	Provision(ctx context.Context, seriesN int) error

	// Ingest writes count samples spread across seriesN series, spaced
	// intervalSec apart, using workers concurrent senders and batchSize
	// samples per request. It returns per-request latencies and the wall-clock
	// total.
	Ingest(ctx context.Context, count, seriesN, batchSize, workers, intervalSec int) ([]time.Duration, time.Duration, error)

	// Query runs read operation op (one of the Op* constants) iters times and
	// returns per-iteration latencies and the wall-clock total.
	Query(ctx context.Context, op string, iters int) ([]time.Duration, time.Duration, error)

	// StorageBytes returns the backend's self-reported on-disk size in bytes,
	// or 0 when the backend does not expose a comparable figure (Zabbix stores
	// history in an external RDBMS).
	StorageBytes(ctx context.Context) (int64, error)
}
