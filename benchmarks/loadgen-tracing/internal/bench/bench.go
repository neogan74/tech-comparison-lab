// Package bench drives the span-ingest + trace-query workload shared by the
// Jaeger and Zipkin benchmark targets.
package bench

import (
	"context"
	mrand "math/rand"
	"sync"
	"time"

	"github.com/tech-comparison-lab/loadgen-tracing/internal/gen"
	"github.com/tech-comparison-lab/loadgen-tracing/internal/tracing"
)

// Bench orchestrates ingest and query operations against one backend
// ("jaeger" | "zipkin").
type Bench struct {
	db       string
	client   *tracing.Client
	services []string
}

// New creates a Bench. ingestAddr is the Zipkin-format collector base URL,
// queryAddr the query API base URL (may equal ingestAddr for Zipkin),
// servicesN the number of distinct synthetic services.
func New(db, ingestAddr, queryAddr string, servicesN int) *Bench {
	return &Bench{
		db:       db,
		client:   tracing.New(db, ingestAddr, queryAddr),
		services: gen.Services(servicesN),
	}
}

// Service returns the primary service used for query benchmarks.
func (b *Bench) Service() string { return b.services[0] }

// Ping verifies query connectivity via the services endpoint.
func (b *Bench) Ping(ctx context.Context) error {
	return b.client.Services(ctx)
}

// Ingest generates count spans, grouped into fixed-size traces, and pushes
// them via workers concurrent goroutines batching batchSize spans per request.
// Spans are generated per batch to keep memory bounded for large runs.
func (b *Bench) Ingest(ctx context.Context, count, batchSize, workers int) ([]time.Duration, time.Duration, error) {
	traces := max(1, count/gen.SpansPerTrace)
	tracesPerBatch := max(1, batchSize/gen.SpansPerTrace)
	nowMicros := time.Now().UnixMicro()

	type job struct{ start, end int }
	jobs := make(chan job, workers*2)

	var mu sync.Mutex
	durs := make([]time.Duration, 0, traces/tracesPerBatch+1)
	var firstErr error
	var wg sync.WaitGroup

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(seed int64) {
			defer wg.Done()
			rnd := mrand.New(mrand.NewSource(seed))
			for j := range jobs {
				batch := make([]tracing.Span, 0, (j.end-j.start)*gen.SpansPerTrace)
				for i := j.start; i < j.end; i++ {
					batch = append(batch, gen.Trace(i, b.services, nowMicros, rnd)...)
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
	for i := 0; i < traces; i += tracesPerBatch {
		end := i + tracesPerBatch
		if end > traces {
			end = traces
		}
		jobs <- job{start: i, end: end}
	}
	close(jobs)
	wg.Wait()
	return durs, time.Since(start), firstErr
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

// ListServices benchmarks the service-discovery endpoint.
func (b *Bench) ListServices(ctx context.Context, iters int) ([]time.Duration, time.Duration, error) {
	return repeat(ctx, iters, func(ctx context.Context) error {
		return b.client.Services(ctx)
	})
}

// FindTraces benchmarks recent-traces-by-service lookups.
func (b *Bench) FindTraces(ctx context.Context, iters int) ([]time.Duration, time.Duration, error) {
	return repeat(ctx, iters, func(ctx context.Context) error {
		return b.client.Traces(ctx, b.Service(), 20)
	})
}

// FindOperations benchmarks the per-service operation-name listing.
func (b *Bench) FindOperations(ctx context.Context, iters int) ([]time.Duration, time.Duration, error) {
	return repeat(ctx, iters, func(ctx context.Context) error {
		return b.client.Operations(ctx, b.Service())
	})
}

// FindTrace benchmarks a point lookup of a single trace by id. The id is
// sampled from the backend so its exact stored form is used; if nothing is
// indexed yet the trace endpoint is still exercised with a generated id.
func (b *Bench) FindTrace(ctx context.Context, iters int) ([]time.Duration, time.Duration, error) {
	id, err := b.client.SampleTraceID(ctx, b.Service())
	if err != nil {
		return nil, 0, err
	}
	if id == "" {
		id = gen.TraceID(0)
	}
	return repeat(ctx, iters, func(ctx context.Context) error {
		return b.client.Trace(ctx, id)
	})
}
