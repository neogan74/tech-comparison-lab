// Package gen produces synthetic distributed-tracing spans in the Zipkin v2
// JSON model, which both Zipkin and Jaeger (via its Zipkin-compatible
// collector endpoint) accept unchanged.
package gen

import (
	"fmt"
	mrand "math/rand"

	"github.com/tech-comparison-lab/loadgen-tracing/internal/tracing"
)

// SpansPerTrace is the fixed shape of every generated trace: one root server
// span plus (SpansPerTrace-1) downstream client/server child spans.
const SpansPerTrace = 5

var operations = []string{
	"GET /api/v2/resource",
	"POST /api/v2/resource",
	"query.select",
	"cache.get",
	"publish.event",
}

// Services returns n deterministic service names, e.g. "svc-000".
func Services(n int) []string {
	if n < 1 {
		n = 1
	}
	names := make([]string, n)
	for i := range names {
		names[i] = fmt.Sprintf("svc-%03d", i)
	}
	return names
}

// TraceID returns the deterministic 128-bit (32 hex chars) trace id for trace i.
func TraceID(i int) string { return fmt.Sprintf("%032x", i+1) }

func spanID(traceIdx, spanIdx int) string {
	return fmt.Sprintf("%016x", int64(traceIdx+1)*1000+int64(spanIdx))
}

// Trace builds the spans for trace index i. services assigns the root service
// (round-robin), nowMicros anchors the trace to a recent wall-clock time so it
// falls inside the backends' default query lookback window.
func Trace(i int, services []string, nowMicros int64, rnd *mrand.Rand) []tracing.Span {
	root := services[i%len(services)]
	spans := make([]tracing.Span, 0, SpansPerTrace)
	traceID := TraceID(i)
	base := nowMicros - int64(rnd.Intn(60_000_000)) // within the last 60s

	rootDur := int64(2000 + rnd.Intn(50_000))
	spans = append(spans, tracing.Span{
		TraceID:       traceID,
		ID:            spanID(i, 0),
		Name:          operations[i%len(operations)],
		Timestamp:     base,
		Duration:      rootDur,
		Kind:          "SERVER",
		LocalEndpoint: tracing.Endpoint{ServiceName: root},
		Tags:          map[string]string{"http.status_code": "200", "region": "us-east-1"},
	})

	for s := 1; s < SpansPerTrace; s++ {
		svc := services[(i+s)%len(services)]
		off := int64(rnd.Intn(int(rootDur/2) + 1))
		spans = append(spans, tracing.Span{
			TraceID:       traceID,
			ID:            spanID(i, s),
			ParentID:      spanID(i, 0),
			Name:          operations[(i+s)%len(operations)],
			Timestamp:     base + off,
			Duration:      int64(500 + rnd.Intn(10_000)),
			Kind:          "CLIENT",
			LocalEndpoint: tracing.Endpoint{ServiceName: svc},
			Tags:          map[string]string{"component": "grpc"},
		})
	}
	return spans
}
