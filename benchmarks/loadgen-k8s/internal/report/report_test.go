package report_test

import (
	"testing"
	"time"

	"github.com/tech-comparison-lab/loadgen-k8s/internal/report"
)

func TestFromDurations(t *testing.T) {
	durs := []time.Duration{
		1 * time.Millisecond,
		2 * time.Millisecond,
		5 * time.Millisecond,
		100 * time.Millisecond,
	}
	r := report.FromDurations("test", durs, 0)
	if r.Count != 4 {
		t.Errorf("expected count=4, got %d", r.Count)
	}
	if r.P50MS <= 0 {
		t.Errorf("expected P50MS > 0, got %f", r.P50MS)
	}
	if r.P99MS < r.P50MS {
		t.Errorf("P99 < P50: P50=%f P99=%f", r.P50MS, r.P99MS)
	}
	if r.MinMS > r.MaxMS {
		t.Errorf("min > max: min=%f max=%f", r.MinMS, r.MaxMS)
	}
}

func TestFromDurationsEmpty(t *testing.T) {
	r := report.FromDurations("empty", nil, 3)
	if r.Op != "empty" {
		t.Errorf("expected op=empty, got %s", r.Op)
	}
	if r.Errors != 3 {
		t.Errorf("expected errors=3, got %d", r.Errors)
	}
}

func TestScalar(t *testing.T) {
	r := report.Scalar("node-count", 3, "count")
	if r.Value != 3 {
		t.Errorf("expected value=3, got %f", r.Value)
	}
	if r.Unit != "count" {
		t.Errorf("expected unit=count, got %s", r.Unit)
	}
}
