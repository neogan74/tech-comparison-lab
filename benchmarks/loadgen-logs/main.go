package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/tech-comparison-lab/loadgen-logs/internal/bench"
	"github.com/tech-comparison-lab/loadgen-logs/internal/report"
)

func main() {
	db := flag.String("db", "", "backend: loki | elasticsearch (required)")
	op := flag.String("op", "all", "operation: write|label-values|query-range|filter-match|count-over-time|all")
	count := flag.Int("count", 500_000, "total log entries to write")
	services := flag.Int("services", 50, "distinct synthetic services")
	batchSize := flag.Int("batch", 2000, "entries per ingest request")
	workers := flag.Int("workers", 4, "concurrent ingest workers")
	queryIter := flag.Int("query-iter", 20, "iterations per query benchmark")
	windowSec := flag.Int("window", 300, "seconds entries are spread over and queries look back")
	limit := flag.Int("limit", 100, "page size for line-returning queries")
	addr := flag.String("addr", "", "backend base URL, e.g. http://localhost:3100 (or LOGS_ADDR env)")
	out := flag.String("out", "", "write JSON results to file (optional)")
	dryRun := flag.Bool("dry-run", false, "test connectivity only")
	flag.Parse()

	if *db != "loki" && *db != "elasticsearch" {
		fmt.Fprintln(os.Stderr, "error: --db must be loki or elasticsearch")
		flag.Usage()
		os.Exit(1)
	}
	if err := validateConfig(*op, *count, *services, *batchSize, *workers, *queryIter, *windowSec, *limit); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if *addr == "" {
		*addr = os.Getenv("LOGS_ADDR")
	}
	if *addr == "" {
		defaults := map[string]string{
			"loki":          "http://localhost:3100",
			"elasticsearch": "http://localhost:9200",
		}
		*addr = defaults[*db]
	}

	ctx := context.Background()
	window := time.Duration(*windowSec) * time.Second
	b := bench.New(*db, *addr, *services, window, *limit)

	if err := b.Ping(ctx); err != nil {
		log.Fatalf("%s: connect failed: %v", *db, err)
	}
	fmt.Printf("%s: connected OK (addr=%s)\n", *db, *addr)
	if *dryRun {
		return
	}

	if err := b.Setup(ctx); err != nil {
		log.Fatalf("%s: setup: %v", *db, err)
	}

	var results []report.Result

	if *op == "write" || *op == "all" {
		fmt.Printf("%s: writing %d entries across %d services (batch=%d workers=%d)...\n",
			*db, *count, *services, *batchSize, *workers)
		durs, total, err := b.Ingest(ctx, *count, *batchSize, *workers)
		if err != nil {
			log.Fatalf("%s: write: %v", *db, err)
		}
		results = append(results, report.Compute(*db, "write", *count, 0, durs, total))
		fmt.Printf("%s: write done in %s\n", *db, total.Round(time.Millisecond))

		// Make the fresh data queryable before timing any query.
		if err := b.Flush(ctx); err != nil {
			log.Fatalf("%s: flush: %v", *db, err)
		}
		time.Sleep(2 * time.Second)
		indexed, err := b.Indexed(ctx)
		if err != nil {
			log.Fatalf("%s: verify ingest: %v", *db, err)
		}
		if indexed == 0 {
			log.Fatalf("%s: ingest reported success but no entries are queryable for %s",
				*db, b.Service())
		}
		fmt.Printf("%s: %d entries queryable for %s\n", *db, indexed, b.Service())
	}

	queries := []struct {
		name string
		fn   func(context.Context, int) ([]time.Duration, time.Duration, error)
	}{
		{"label-values", b.LabelValues},
		{"query-range", b.QueryRange},
		{"filter-match", b.FilterMatch},
		{"count-over-time", b.CountOverTime},
	}
	for _, q := range queries {
		if *op == q.name || *op == "all" {
			fmt.Printf("%s: %s (%d iters)...\n", *db, q.name, *queryIter)
			durs, total, err := q.fn(ctx, *queryIter)
			if err != nil {
				log.Fatalf("%s: %s: %v", *db, q.name, err)
			}
			results = append(results, report.Compute(*db, q.name, *queryIter, *queryIter, durs, total))
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

func validateConfig(op string, count, services, batchSize, workers, queryIter, windowSec, limit int) error {
	validOps := map[string]bool{
		"write": true, "label-values": true, "query-range": true,
		"filter-match": true, "count-over-time": true, "all": true,
	}
	if !validOps[op] {
		return fmt.Errorf("unknown --op %q, want write|label-values|query-range|filter-match|count-over-time|all", op)
	}
	if count < 1 {
		return fmt.Errorf("--count must be >= 1")
	}
	if services < 1 {
		return fmt.Errorf("--services must be >= 1")
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
	if windowSec < 1 {
		return fmt.Errorf("--window must be >= 1")
	}
	if limit < 1 {
		return fmt.Errorf("--limit must be >= 1")
	}
	return nil
}
