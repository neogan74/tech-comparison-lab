package report

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"text/tabwriter"
	"time"
)

// Result holds benchmark metrics for a single db+operation combination.
type Result struct {
	DB           string  `json:"db"`
	Operation    string  `json:"op"`
	Count        int     `json:"count"`
	Workers      int     `json:"workers"`
	P50Ms        float64 `json:"p50_ms"`
	P95Ms        float64 `json:"p95_ms"`
	P99Ms        float64 `json:"p99_ms"`
	MinMs        float64 `json:"min_ms"`
	MaxMs        float64 `json:"max_ms"`
	MeanMs       float64 `json:"mean_ms"`
	ThroughputOps float64 `json:"throughput_ops_sec"`
	StorageBytes int64   `json:"storage_bytes,omitempty"`
	IndexBuildMs int64   `json:"index_build_ms,omitempty"`
	TotalMs      int64   `json:"total_ms"`
}

// Summary is the top-level JSON output structure.
type Summary struct {
	RunID     string   `json:"run_id"`
	Timestamp string   `json:"timestamp"`
	Results   []Result `json:"results"`
}

// Compute derives p50/p95/p99/min/max/mean/throughput from raw durations.
// count is the number of logical operations (documents for insert, iterations
// for query/agg/update); durations contains one entry per batch or iteration.
func Compute(db, op string, count, workers int, durations []time.Duration, total time.Duration) Result {
	r := Result{
		DB:        db,
		Operation: op,
		Count:     count,
		Workers:   workers,
		TotalMs:   total.Milliseconds(),
	}
	if len(durations) == 0 {
		return r
	}

	sorted := make([]time.Duration, len(durations))
	copy(sorted, durations)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	n := len(sorted)
	pct := func(p float64) float64 {
		idx := int(math.Ceil(p/100.0*float64(n))) - 1
		if idx < 0 {
			idx = 0
		}
		if idx >= n {
			idx = n - 1
		}
		return ms(sorted[idx])
	}

	var sum int64
	for _, d := range sorted {
		sum += d.Microseconds()
	}

	r.P50Ms = pct(50)
	r.P95Ms = pct(95)
	r.P99Ms = pct(99)
	r.MinMs = ms(sorted[0])
	r.MaxMs = ms(sorted[n-1])
	r.MeanMs = float64(sum) / float64(n) / 1000.0
	if total > 0 {
		r.ThroughputOps = float64(count) / total.Seconds()
	}
	return r
}

func ms(d time.Duration) float64 {
	return float64(d.Microseconds()) / 1000.0
}

// PrintTable writes a formatted comparison table to stdout.
func PrintTable(results []Result) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "DB\tOperation\tCount\tp50(ms)\tp95(ms)\tp99(ms)\tops/s\tStorage")
	fmt.Fprintln(w, "--\t---------\t-----\t-------\t-------\t-------\t----\t-------")
	for _, r := range results {
		storage := "-"
		if r.StorageBytes > 0 {
			storage = formatBytes(r.StorageBytes)
		}
		fmt.Fprintf(w, "%s\t%s\t%d\t%.2f\t%.2f\t%.2f\t%.0f\t%s\n",
			r.DB, r.Operation, r.Count,
			r.P50Ms, r.P95Ms, r.P99Ms,
			r.ThroughputOps, storage)
	}
	w.Flush()
}

// WriteJSON marshals the summary to a JSON file at path.
func WriteJSON(summary Summary, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(summary)
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%dB", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%cB", float64(b)/float64(div), "KMGTPE"[exp])
}
