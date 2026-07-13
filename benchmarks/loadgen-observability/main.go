package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/tech-comparison-lab/loadgen-observability/internal/bench"
	"github.com/tech-comparison-lab/loadgen-observability/internal/report"
)

func main() {
	db := flag.String("db", "", "database: prometheus | victoriametrics (required)")
	op := flag.String("op", "all", "operation: write|instant-sum|instant-filtered|topk|range-avg|all")
	count := flag.Int("count", 10_000_000, "total samples to write")
	series := flag.Int("series", 10_000, "unique time series (cardinality)")
	interval := flag.Int("interval", 15, "seconds between samples of the same series")
	batchSize := flag.Int("batch", 5_000, "samples per remote_write request")
	workers := flag.Int("workers", 4, "concurrent remote_write workers")
	queryIter := flag.Int("query-iter", 5, "iterations per query benchmark")
	addr := flag.String("addr", "", "base URL, e.g. http://localhost:9090 (or OBS_ADDR env)")
	out := flag.String("out", "", "write JSON results to file (optional)")
	dryRun := flag.Bool("dry-run", false, "test connectivity only")
	flag.Parse()

	if *db != "prometheus" && *db != "victoriametrics" {
		fmt.Fprintln(os.Stderr, "error: --db must be prometheus or victoriametrics")
		flag.Usage()
		os.Exit(1)
	}
	if err := validateConfig(*op, *count, *series, *interval, *batchSize, *workers, *queryIter); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if *addr == "" {
		*addr = os.Getenv("OBS_ADDR")
	}
	if *addr == "" {
		defaults := map[string]string{
			"prometheus":      "http://localhost:9090",
			"victoriametrics": "http://localhost:8428",
		}
		*addr = defaults[*db]
	}

	ctx := context.Background()
	b := bench.New(*db, *addr)

	if err := b.Ping(ctx); err != nil {
		log.Fatalf("%s: connect failed: %v", *db, err)
	}
	fmt.Printf("%s: connected OK (%s)\n", *db, *addr)
	if *dryRun {
		return
	}

	var results []report.Result

	if *op == "write" || *op == "all" {
		fmt.Printf("%s: writing %d samples across %d series (batch=%d workers=%d)...\n",
			*db, *count, *series, *batchSize, *workers)
		durs, total, err := b.Ingest(ctx, *count, *series, *batchSize, *workers, *interval)
		if err != nil {
			log.Fatalf("%s: write: %v", *db, err)
		}
		r := report.Compute(*db, "write", *count, 0, durs, total)
		r.StorageBytes, _ = b.StorageSize(ctx)
		results = append(results, r)
		fmt.Printf("%s: write done in %s\n", *db, total.Round(time.Millisecond))
	}

	queries := []struct {
		name string
		fn   func(context.Context, int) ([]time.Duration, time.Duration, error)
	}{
		{"instant-sum", b.InstantSum},
		{"instant-filtered", b.InstantFiltered},
		{"topk", b.TopK},
		{"range-avg", b.RangeAvg},
	}
	for _, q := range queries {
		if *op == q.name || *op == "all" {
			fmt.Printf("%s: %s (%d iters)...\n", *db, q.name, *queryIter)
			durs, total, err := q.fn(ctx, *queryIter)
			if err != nil {
				log.Fatalf("%s: %s: %v", *db, q.name, err)
			}
			results = append(results, report.Compute(*db, q.name, *series, *queryIter, durs, total))
		}
	}

	if len(results) > 0 {
		report.PrintTable(results)
		if *out != "" {
			summary := report.Summary{
				RunID:     fmt.Sprintf("%s-%d", *db, time.Now().Unix()),
				Timestamp: time.Now().UTC().Format(time.RFC3339),
				Results:   results,
			}
			if err := report.WriteJSON(summary, *out); err != nil {
				log.Printf("warning: %v", err)
			} else {
				fmt.Printf("\nResults saved to %s\n", *out)
			}
		}
	}
}

func validateConfig(op string, count, series, interval, batchSize, workers, queryIter int) error {
	validOps := map[string]bool{
		"write": true, "instant-sum": true, "instant-filtered": true,
		"topk": true, "range-avg": true, "all": true,
	}
	if !validOps[op] {
		return fmt.Errorf("unknown --op %q, want write|instant-sum|instant-filtered|topk|range-avg|all", op)
	}
	if count < 1 {
		return fmt.Errorf("--count must be >= 1")
	}
	if series < 1 {
		return fmt.Errorf("--series must be >= 1")
	}
	if interval < 1 {
		return fmt.Errorf("--interval must be >= 1")
	}
	if batchSize < 1 {
		return fmt.Errorf("--batch must be >= 1")
	}
	if workers < 1 {
		return fmt.Errorf("--workers must be >= 1")
	}
	if queryIter < 1 {
		return fmt.Errorf("--query-iter must be >= 1")
	}
	return nil
}
