package report

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"time"
)

// Result holds metrics for a single benchmark operation.
type Result struct {
	Mesh          string  `json:"mesh,omitempty"`
	Op            string  `json:"op"`
	Count         int     `json:"count,omitempty"`
	P50MS         float64 `json:"p50_ms,omitempty"`
	P95MS         float64 `json:"p95_ms,omitempty"`
	P99MS         float64 `json:"p99_ms,omitempty"`
	MeanMS        float64 `json:"mean_ms,omitempty"`
	MinMS         float64 `json:"min_ms,omitempty"`
	MaxMS         float64 `json:"max_ms,omitempty"`
	ThroughputRPS float64 `json:"throughput_rps,omitempty"`
	Value         float64 `json:"value,omitempty"`
	Unit          string  `json:"unit,omitempty"`
	Errors        int     `json:"errors,omitempty"`
}

// StampMesh sets the mesh tag on every result in rs.
func StampMesh(mesh string, rs []Result) {
	for i := range rs {
		rs[i].Mesh = mesh
	}
}

// Report holds all results for a benchmark run.
type Report struct {
	Mesh      string    `json:"mesh"`
	Timestamp time.Time `json:"timestamp"`
	Results   []Result  `json:"results"`
}

// FromDurations computes a Result with latency percentiles from a slice of durations.
func FromDurations(op string, durs []time.Duration, errors int) Result {
	if len(durs) == 0 {
		return Result{Op: op, Errors: errors}
	}
	sorted := make([]time.Duration, len(durs))
	copy(sorted, durs)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	var sum time.Duration
	for _, d := range sorted {
		sum += d
	}
	toMS := func(d time.Duration) float64 { return float64(d.Microseconds()) / 1000.0 }
	return Result{
		Op:     op,
		Count:  len(durs),
		P50MS:  toMS(pct(sorted, 50)),
		P95MS:  toMS(pct(sorted, 95)),
		P99MS:  toMS(pct(sorted, 99)),
		MeanMS: toMS(sum / time.Duration(len(sorted))),
		MinMS:  toMS(sorted[0]),
		MaxMS:  toMS(sorted[len(sorted)-1]),
		Errors: errors,
	}
}

// Scalar creates a Result with a single scalar value.
func Scalar(op string, value float64, unit string) Result {
	return Result{Op: op, Value: value, Unit: unit}
}

func pct(sorted []time.Duration, p float64) time.Duration {
	idx := int(math.Ceil(p/100.0*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// PrintTable prints a human-readable results table to w.
func PrintTable(w io.Writer, r Report) {
	fmt.Fprintf(w, "\nMesh : %s\n", r.Mesh)
	fmt.Fprintf(w, "%-35s  %9s %9s %9s %9s  %10s  %-8s\n",
		"Operation", "P50(ms)", "P95(ms)", "P99(ms)", "Mean(ms)", "Value", "Unit")
	fmt.Fprintf(w, "%-35s  %9s %9s %9s %9s  %10s  %-8s\n",
		"───────────────────────────────────",
		"─────────", "─────────", "─────────", "─────────", "──────────", "────────")
	for _, res := range r.Results {
		if res.P50MS > 0 {
			fmt.Fprintf(w, "%-35s  %9.2f %9.2f %9.2f %9.2f  %10s  %-8s\n",
				res.Op, res.P50MS, res.P95MS, res.P99MS, res.MeanMS, "-", "-")
		} else {
			fmt.Fprintf(w, "%-35s  %9s %9s %9s %9s  %10.1f  %-8s\n",
				res.Op, "-", "-", "-", "-", res.Value, res.Unit)
		}
	}
}

// WriteJSON writes the report as indented JSON to outFile.
func WriteJSON(outFile string, r Report) error {
	f, err := os.Create(outFile)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}
