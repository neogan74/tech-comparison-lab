package report

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCompute_basic(t *testing.T) {
	durs := []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
		30 * time.Millisecond,
		40 * time.Millisecond,
		100 * time.Millisecond,
	}
	total := 200 * time.Millisecond
	r := Compute("argocd", "sync-latency", len(durs), durs, total)

	if r.Tool != "argocd" {
		t.Errorf("tool: got %q, want %q", r.Tool, "argocd")
	}
	if r.Op != "sync-latency" {
		t.Errorf("op: got %q, want %q", r.Op, "sync-latency")
	}
	if r.Count != 5 {
		t.Errorf("count: got %d, want 5", r.Count)
	}
	if r.MinMs != 10.0 {
		t.Errorf("min_ms: got %.1f, want 10.0", r.MinMs)
	}
	if r.MaxMs != 100.0 {
		t.Errorf("max_ms: got %.1f, want 100.0", r.MaxMs)
	}
	if r.P50Ms != 30.0 {
		t.Errorf("p50_ms: got %.1f, want 30.0", r.P50Ms)
	}
	if r.TotalMs != 200 {
		t.Errorf("total_ms: got %d, want 200", r.TotalMs)
	}
}

func TestCompute_single(t *testing.T) {
	durs := []time.Duration{50 * time.Millisecond}
	r := Compute("flux", "reconcile", 1, durs, 50*time.Millisecond)
	if r.P50Ms != 50.0 {
		t.Errorf("single element p50: got %.1f, want 50.0", r.P50Ms)
	}
	if r.P95Ms != 50.0 {
		t.Errorf("single element p95: got %.1f, want 50.0", r.P95Ms)
	}
}

func TestCompute_empty(t *testing.T) {
	r := Compute("flux", "bulk", 0, nil, 0)
	if r.P50Ms != 0 || r.TotalMs != 0 {
		t.Errorf("empty durations should produce zero metrics, got p50=%.1f total=%d", r.P50Ms, r.TotalMs)
	}
}

func TestWriteJSON_roundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.json")

	s := Summary{
		Tool:      "argocd",
		RunID:     "argocd-123",
		Timestamp: "2025-01-01T00:00:00Z",
		Results: []Result{
			{Tool: "argocd", Op: "sync-latency", Count: 5, P50Ms: 12.5, TotalMs: 250},
		},
	}
	if err := WriteJSON(s, path); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var got Summary
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Tool != s.Tool {
		t.Errorf("tool: got %q, want %q", got.Tool, s.Tool)
	}
	if len(got.Results) != 1 || got.Results[0].P50Ms != 12.5 {
		t.Errorf("results mismatch: %+v", got.Results)
	}
}
