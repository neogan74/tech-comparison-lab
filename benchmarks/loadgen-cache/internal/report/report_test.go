package report

import (
	"testing"
	"time"
)

func TestComputePercentiles(t *testing.T) {
	durs := make([]time.Duration, 100)
	for i := range durs {
		durs[i] = time.Duration(i+1) * time.Millisecond
	}
	r := Compute("redis", "pipeline-set", 10000, 8, durs, 5*time.Second)

	if r.P50Ms < 49 || r.P50Ms > 51 {
		t.Errorf("p50 = %.2f, want ~50ms", r.P50Ms)
	}
	if r.P95Ms < 94 || r.P95Ms > 96 {
		t.Errorf("p95 = %.2f, want ~95ms", r.P95Ms)
	}
	if !(r.MinMs <= r.P50Ms && r.P50Ms <= r.P95Ms && r.P95Ms <= r.P99Ms && r.P99Ms <= r.MaxMs) {
		t.Errorf("ordering violated: min=%.2f p50=%.2f p95=%.2f p99=%.2f max=%.2f",
			r.MinMs, r.P50Ms, r.P95Ms, r.P99Ms, r.MaxMs)
	}
	// throughput: 10000 ops / 5s = 2000
	if r.ThroughputOps < 1990 || r.ThroughputOps > 2010 {
		t.Errorf("throughput = %.2f, want ~2000", r.ThroughputOps)
	}
}

func TestComputeEmpty(t *testing.T) {
	r := Compute("valkey", "get", 0, 1, nil, 0)
	if r.ThroughputOps != 0 || r.P50Ms != 0 {
		t.Errorf("expected zeroed result, got %+v", r)
	}
}

func TestComputeSingle(t *testing.T) {
	r := Compute("redis", "set", 1, 1, []time.Duration{5 * time.Millisecond}, time.Second)
	if r.P50Ms != 5 || r.P99Ms != 5 {
		t.Errorf("single-element: p50=%.2f p99=%.2f, want 5ms", r.P50Ms, r.P99Ms)
	}
}
