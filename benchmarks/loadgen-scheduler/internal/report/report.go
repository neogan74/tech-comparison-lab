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

type Result struct {
	Op     string  `json:"op"`
	Count  int     `json:"count"`
	P50MS  float64 `json:"p50_ms"`
	P95MS  float64 `json:"p95_ms"`
	P99MS  float64 `json:"p99_ms"`
	MeanMS float64 `json:"mean_ms"`
	MinMS  float64 `json:"min_ms"`
	MaxMS  float64 `json:"max_ms"`
	Errors int     `json:"errors"`
}

type Report struct {
	Platform  string    `json:"platform"`
	Timestamp time.Time `json:"timestamp"`
	Results   []Result  `json:"results"`
}

func FromDurations(op string, durations []time.Duration, errors int) Result {
	if len(durations) == 0 {
		return Result{Op: op, Errors: errors}
	}
	sorted := append([]time.Duration(nil), durations...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	var sum time.Duration
	for _, duration := range sorted {
		sum += duration
	}
	toMS := func(duration time.Duration) float64 {
		return float64(duration.Microseconds()) / 1000
	}
	return Result{
		Op: op, Count: len(sorted), Errors: errors,
		P50MS:  toMS(percentile(sorted, 50)),
		P95MS:  toMS(percentile(sorted, 95)),
		P99MS:  toMS(percentile(sorted, 99)),
		MeanMS: toMS(sum / time.Duration(len(sorted))),
		MinMS:  toMS(sorted[0]), MaxMS: toMS(sorted[len(sorted)-1]),
	}
}

func percentile(sorted []time.Duration, value float64) time.Duration {
	index := int(math.Ceil(value/100*float64(len(sorted)))) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

func PrintTable(writer io.Writer, value Report) {
	fmt.Fprintf(writer, "\nPlatform: %s\n", value.Platform)
	fmt.Fprintf(writer, "%-26s %9s %9s %9s %9s %8s\n", "Operation", "P50(ms)", "P95(ms)", "P99(ms)", "Mean(ms)", "Errors")
	for _, result := range value.Results {
		fmt.Fprintf(writer, "%-26s %9.2f %9.2f %9.2f %9.2f %8d\n",
			result.Op, result.P50MS, result.P95MS, result.P99MS, result.MeanMS, result.Errors)
	}
}

func WriteJSON(path string, value Report) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
