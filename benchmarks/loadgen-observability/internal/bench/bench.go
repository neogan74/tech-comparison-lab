// Package bench drives the remote_write ingest + PromQL query workload
// shared by the Prometheus and VictoriaMetrics benchmark targets.
package bench

import (
	"context"
	"fmt"
	mrand "math/rand"
	"sync"
	"time"

	"github.com/tech-comparison-lab/loadgen-observability/internal/gen"
	"github.com/tech-comparison-lab/loadgen-observability/internal/metrics"
	"github.com/tech-comparison-lab/loadgen-observability/internal/query"
	"github.com/tech-comparison-lab/loadgen-observability/internal/remote"
)

// MetricName is the synthetic gauge series name used by every run.
const MetricName = "bench_metric"

// Bench drives ingest and query operations against one target (prometheus or
// victoriametrics), both of which speak the Prometheus remote_write and
// PromQL HTTP APIs.
type Bench struct {
	db     string
	remote *remote.Client
	query  *query.Client
	addr   string
}

// New creates a Bench for db ("prometheus" | "victoriametrics") against the
// base URL addr (e.g. http://localhost:9090).
func New(db, addr string) *Bench {
	return &Bench{
		db:     db,
		remote: remote.New(addr),
		query:  query.New(addr),
		addr:   addr,
	}
}

// Ping verifies connectivity with a cheap literal query.
func (b *Bench) Ping(ctx context.Context) error {
	_, err := b.query.Instant(ctx, "vector(1)")
	return err
}

// Ingest pushes count synthetic samples spread across seriesN unique time
// series via remote_write, using workers concurrent goroutines. Series
// payloads are generated per-batch (not pre-materialized) to keep memory
// bounded for large runs.
func (b *Bench) Ingest(ctx context.Context, count, seriesN, batchSize, workers, intervalSec int) ([]time.Duration, time.Duration, error) {
	samplesPerSeries := max(1, count/seriesN)
	seriesPerBatch := max(1, batchSize/samplesPerSeries)
	intervalMs := int64(intervalSec) * 1000
	nowMs := time.Now().UnixMilli()
	startMs := nowMs - int64(samplesPerSeries)*intervalMs

	seriesDefs := gen.Build(MetricName, seriesN)

	type job struct{ start, end int }
	jobs := make(chan job, workers*2)

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
		jobs <- job{start: i, end: end}
	}
	close(jobs)
	wg.Wait()
	return durs, time.Since(start), firstErr
}

func (b *Bench) runQuery(ctx context.Context, expr string, iters int) ([]time.Duration, time.Duration, error) {
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

// InstantSum: sum(bench_metric) — full aggregation across every series.
func (b *Bench) InstantSum(ctx context.Context, iters int) ([]time.Duration, time.Duration, error) {
	return b.runQuery(ctx, fmt.Sprintf("sum(%s)", MetricName), iters)
}

// InstantFiltered: sum(bench_metric{region="us-east-1"}) — label-narrowed aggregation.
func (b *Bench) InstantFiltered(ctx context.Context, iters int) ([]time.Duration, time.Duration, error) {
	return b.runQuery(ctx, fmt.Sprintf(`sum(%s{region="us-east-1"})`, MetricName), iters)
}

// TopK: topk(10, bench_metric) — top-k selection across all series.
func (b *Bench) TopK(ctx context.Context, iters int) ([]time.Duration, time.Duration, error) {
	return b.runQuery(ctx, fmt.Sprintf("topk(10, %s)", MetricName), iters)
}

// RangeAvg: avg_over_time(bench_metric[30m]) — range-vector function per series.
func (b *Bench) RangeAvg(ctx context.Context, iters int) ([]time.Duration, time.Duration, error) {
	return b.runQuery(ctx, fmt.Sprintf("avg_over_time(%s[30m])", MetricName), iters)
}

// StorageSize returns the target's self-reported on-disk size in bytes,
// read from its own /metrics endpoint. Returns 0 if the underlying gauge
// isn't present yet (e.g. Prometheus hasn't cut a block).
func (b *Bench) StorageSize(ctx context.Context) (int64, error) {
	switch b.db {
	case "prometheus":
		blocks, blocksErr := metrics.SumMetric(ctx, b.addr, "prometheus_tsdb_storage_blocks_bytes")
		wal, walErr := metrics.SumMetric(ctx, b.addr, "prometheus_tsdb_wal_storage_size_bytes")
		if blocksErr != nil && walErr != nil {
			return 0, fmt.Errorf("prometheus storage size: %v / %v", blocksErr, walErr)
		}
		return int64(blocks + wal), nil
	case "victoriametrics":
		v, err := metrics.SumMetric(ctx, b.addr, "vm_data_size_bytes")
		if err != nil {
			return 0, err
		}
		return int64(v), nil
	default:
		return 0, fmt.Errorf("unknown db %q", b.db)
	}
}
