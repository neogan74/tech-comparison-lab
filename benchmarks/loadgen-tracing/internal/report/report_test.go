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
	r := Compute("jaeger", "write", 100000, 5, durs, 5*time.Second)
	if r.P50Ms < 49 || r.P50Ms > 51 {
		t.Errorf("p50 = %.2f, want ~50ms", r.P50Ms)
	}
	if !(r.MinMs <= r.P50Ms && r.P50Ms <= r.P95Ms && r.P95Ms <= r.P99Ms && r.P99Ms <= r.MaxMs) {
		t.Errorf("ordering violated")
	}
	if r.ThroughputOps < 19990 || r.ThroughputOps > 20010 {
		t.Errorf("throughput = %.2f, want ~20000", r.ThroughputOps)
	}
}

func TestComputeEmpty(t *testing.T) {
	r := Compute("zipkin", "find-traces", 0, 0, nil, 0)
	if r.P50Ms != 0 || r.ThroughputOps != 0 {
		t.Errorf("expected zero result, got %+v", r)
	}
}
