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

// Result holds benchmark metrics for one tool+operation.
type Result struct {
	Tool    string  `json:"tool"`
	Op      string  `json:"op"`
	Count   int     `json:"count"`
	P50Ms   float64 `json:"p50_ms,omitempty"`
	P95Ms   float64 `json:"p95_ms,omitempty"`
	P99Ms   float64 `json:"p99_ms,omitempty"`
	MinMs   float64 `json:"min_ms,omitempty"`
	MaxMs   float64 `json:"max_ms,omitempty"`
	MeanMs  float64 `json:"mean_ms,omitempty"`
	TotalMs int64   `json:"total_ms"`
}

// Summary is the top-level JSON output written per-tool.
type Summary struct {
	Tool      string   `json:"tool"`
	RunID     string   `json:"run_id"`
	Timestamp string   `json:"timestamp"`
	Results   []Result `json:"results"`
}

func Compute(tool, op string, count int, durations []time.Duration, total time.Duration) Result {
	r := Result{
		Tool:    tool,
		Op:      op,
		Count:   count,
		TotalMs: total.Milliseconds(),
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
	return r
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000.0 }

func PrintTable(results []Result) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "Tool\tOperation\tCount\tp50(ms)\tp95(ms)\tp99(ms)\ttotal(ms)")
	fmt.Fprintln(w, "----\t---------\t-----\t-------\t-------\t-------\t---------")
	for _, r := range results {
		if r.P50Ms > 0 {
			fmt.Fprintf(w, "%s\t%s\t%d\t%.0f\t%.0f\t%.0f\t%d\n",
				r.Tool, r.Op, r.Count, r.P50Ms, r.P95Ms, r.P99Ms, r.TotalMs)
		} else {
			fmt.Fprintf(w, "%s\t%s\t%d\t-\t-\t-\t%d\n",
				r.Tool, r.Op, r.Count, r.TotalMs)
		}
	}
	w.Flush()
}

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
