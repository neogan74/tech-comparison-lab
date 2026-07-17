package report

import (
	"testing"
	"time"
)

func TestFromDurations(t *testing.T) {
	result := FromDurations("deploy", []time.Duration{
		5 * time.Millisecond,
		1 * time.Millisecond,
		3 * time.Millisecond,
		2 * time.Millisecond,
	}, 0)
	if result.Count != 4 {
		t.Fatalf("count = %d, want 4", result.Count)
	}
	if result.P50MS != 2 || result.P95MS != 5 {
		t.Fatalf("unexpected percentiles: p50=%v p95=%v", result.P50MS, result.P95MS)
	}
	if result.MinMS != 1 || result.MaxMS != 5 {
		t.Fatalf("unexpected range: min=%v max=%v", result.MinMS, result.MaxMS)
	}
}

func TestFromDurationsEmpty(t *testing.T) {
	result := FromDurations("recover", nil, 2)
	if result.Op != "recover" || result.Errors != 2 || result.Count != 0 {
		t.Fatalf("unexpected empty result: %#v", result)
	}
}
