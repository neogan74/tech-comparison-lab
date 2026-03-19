package report

import (
	"testing"
	"time"
)

func TestComputePercentiles(t *testing.T) {
	// 100 durations: 1ms, 2ms, ..., 100ms
	durations := make([]time.Duration, 100)
	for i := range durations {
		durations[i] = time.Duration(i+1) * time.Millisecond
	}
	total := 5 * time.Second

	r := Compute("postgres", "query", 100, 8, durations, total)

	if r.DB != "postgres" || r.Operation != "query" {
		t.Fatalf("unexpected db/op: %s/%s", r.DB, r.Operation)
	}
	// p50 of 1..100ms should be 50ms
	if r.P50Ms < 49 || r.P50Ms > 51 {
		t.Errorf("p50 = %.2f, want ~50ms", r.P50Ms)
	}
	// p95 should be ~95ms
	if r.P95Ms < 94 || r.P95Ms > 96 {
		t.Errorf("p95 = %.2f, want ~95ms", r.P95Ms)
	}
	// p99 should be ~99ms
	if r.P99Ms < 98 || r.P99Ms > 100 {
		t.Errorf("p99 = %.2f, want ~99ms", r.P99Ms)
	}
	// ordering invariant
	if !(r.MinMs <= r.P50Ms && r.P50Ms <= r.P95Ms && r.P95Ms <= r.P99Ms && r.P99Ms <= r.MaxMs) {
		t.Errorf("percentile ordering violated: min=%.2f p50=%.2f p95=%.2f p99=%.2f max=%.2f",
			r.MinMs, r.P50Ms, r.P95Ms, r.P99Ms, r.MaxMs)
	}
	// throughput: 100 ops / 5s = 20 ops/s
	if r.ThroughputOps < 19 || r.ThroughputOps > 21 {
		t.Errorf("throughput = %.2f, want ~20 ops/s", r.ThroughputOps)
	}
}

func TestComputeEmpty(t *testing.T) {
	r := Compute("mongo", "insert", 0, 1, nil, 0)
	if r.ThroughputOps != 0 || r.P50Ms != 0 {
		t.Errorf("expected zeroed result for empty durations, got %+v", r)
	}
}

func TestComputeSingle(t *testing.T) {
	r := Compute("postgres", "agg", 1, 1, []time.Duration{42 * time.Millisecond}, time.Second)
	if r.P50Ms != 42 || r.P95Ms != 42 || r.P99Ms != 42 {
		t.Errorf("single-element: p50=%.2f p95=%.2f p99=%.2f, all want 42ms", r.P50Ms, r.P95Ms, r.P99Ms)
	}
}
