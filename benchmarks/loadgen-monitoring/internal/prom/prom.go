// Package prom implements the bench.Backend for Prometheus, using the same
// remote_write ingest and PromQL read path as the observability benchmark.
package prom

import (
	"context"
	"fmt"
	mrand "math/rand"
	"sync"
	"time"

	"github.com/tech-comparison-lab/loadgen-monitoring/internal/bench"
	"github.com/tech-comparison-lab/loadgen-monitoring/internal/gen"
	"github.com/tech-comparison-lab/loadgen-monitoring/internal/metrics"
	"github.com/tech-comparison-lab/loadgen-monitoring/internal/query"
	"github.com/tech-comparison-lab/loadgen-monitoring/internal/remote"
)

// MetricName is the synthetic gauge series name used by every run.
const MetricName = "bench_metric"

// Backend drives ingest and query operations against a Prometheus instance
// with --web.enable-remote-write-receiver enabled.
type Backend struct {
	remote *remote.Client
	query  *query.Client
	addr   string
}

// New creates a Prometheus Backend against base URL addr
// (e.g. http://localhost:9090).
func New(addr string) *Backend {
	return &Backend{
		remote: remote.New(addr),
		query:  query.New(addr),
		addr:   addr,
	}
}

// Name identifies this backend in result rows.
func (b *Backend) Name() string { return "prometheus" }

// Ping verifies connectivity with a cheap literal query.
func (b *Backend) Ping(ctx context.Context) error {
	_, err := b.query.Instant(ctx, "vector(1)")
	return err
}

// Provision is a no-op: Prometheus creates series on first write.
func (b *Backend) Provision(ctx context.Context, seriesN int) error { return nil }

// Ingest pushes count synthetic samples across seriesN series via remote_write,
// generating payloads per-batch to keep memory bounded for large runs.
func (b *Backend) Ingest(ctx context.Context, count, seriesN, batchSize, workers, intervalSec int) ([]time.Duration, time.Duration, error) {
	samplesPerSeries := max(1, count/seriesN)
	seriesPerBatch := max(1, batchSize/samplesPerSeries)
	intervalMs := int64(intervalSec) * 1000
	nowMs := time.Now().UnixMilli()
	startMs := nowMs - int64(samplesPerSeries)*intervalMs

	seriesDefs := gen.Build(MetricName, seriesN)

	type jobT struct{ start, end int }
	jobs := make(chan jobT, workers*2)

	var mu sync.Mutex
	durs := make([]time.Duration, 0, seriesN/seriesPerBatch+1)
	var firstErr error
	var wg sync.WaitGroup

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(seed int64) {
			defer wg.Done()
			rnd := mrand.New(mrand.NewSource(seed))
			for j := range jobs {
				batch := make([]remote.TimeSeries, 0, j.end-j.start)
				for i := j.start; i < j.end; i++ {
					labels := make([]remote.Label, len(seriesDefs[i].Labels))
					for k, l := range seriesDefs[i].Labels {
						labels[k] = remote.Label{Name: l.Name, Value: l.Value}
					}
					samples := make([]remote.Sample, samplesPerSeries)
					for t := 0; t < samplesPerSeries; t++ {
						samples[t] = remote.Sample{
							Value:       gen.Value(i, t, rnd),
							TimestampMs: startMs + int64(t)*intervalMs,
						}
					}
					batch = append(batch, remote.TimeSeries{Labels: labels, Samples: samples})
				}

				t0 := time.Now()
				err := b.remote.Push(ctx, batch)
				if err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					mu.Unlock()
					continue
				}
				mu.Lock()
				durs = append(durs, time.Since(t0))
				mu.Unlock()
			}
		}(int64(w + 1))
	}

	start := time.Now()
	for i := 0; i < seriesN; i += seriesPerBatch {
		end := i + seriesPerBatch
		if end > seriesN {
			end = seriesN
		}
		jobs <- jobT{start: i, end: end}
	}
	close(jobs)
	wg.Wait()
	return durs, time.Since(start), firstErr
}

// Query maps a bench.Op* to a PromQL expression and times iters evaluations.
func (b *Backend) Query(ctx context.Context, op string, iters int) ([]time.Duration, time.Duration, error) {
	var expr string
	switch op {
	case bench.OpLatest:
		expr = MetricName
	case bench.OpFiltered:
		expr = fmt.Sprintf(`%s{region="us-east-1"}`, MetricName)
	case bench.OpHistory:
		expr = fmt.Sprintf("%s[30m]", MetricName)
	default:
		return nil, 0, fmt.Errorf("unknown op %q", op)
	}

	durs := make([]time.Duration, 0, iters)
	start := time.Now()
	for i := 0; i < iters; i++ {
		t0 := time.Now()
		if _, err := b.query.Instant(ctx, expr); err != nil {
			return durs, time.Since(start), err
		}
		durs = append(durs, time.Since(t0))
	}
	return durs, time.Since(start), nil
}

// StorageBytes reads Prometheus' self-reported TSDB size (blocks + WAL) from
// its own /metrics endpoint. Returns 0 if the gauges aren't present yet.
func (b *Backend) StorageBytes(ctx context.Context) (int64, error) {
	blocks, blocksErr := metrics.SumMetric(ctx, b.addr, "prometheus_tsdb_storage_blocks_bytes")
	wal, walErr := metrics.SumMetric(ctx, b.addr, "prometheus_tsdb_wal_storage_size_bytes")
	if blocksErr != nil && walErr != nil {
		return 0, fmt.Errorf("prometheus storage size: %v / %v", blocksErr, walErr)
	}
	return int64(blocks + wal), nil
}
