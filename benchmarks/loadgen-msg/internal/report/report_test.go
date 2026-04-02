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
	r := Compute("kafka", "produce", 100000, 1000, durs, 5*time.Second)

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
	// throughput: 100000 / 5s = 20000
	if r.ThroughputOps < 19990 || r.ThroughputOps > 20010 {
		t.Errorf("throughput = %.2f, want ~20000", r.ThroughputOps)
	}
}

func TestComputeEmpty(t *testing.T) {
	r := Compute("rabbitmq", "consume", 0, 0, nil, 0)
	if r.ThroughputOps != 0 || r.P50Ms != 0 {
		t.Errorf("expected zeroed result, got %+v", r)
	}
}

func TestComputeSingle(t *testing.T) {
	r := Compute("kafka", "produce", 1000, 1000, []time.Duration{10 * time.Millisecond}, time.Second)
	if r.P50Ms != 10 || r.P99Ms != 10 {
		t.Errorf("single: p50=%.2f p99=%.2f, want 10ms", r.P50Ms, r.P99Ms)
	}
}
