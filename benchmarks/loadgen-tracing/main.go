package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/tech-comparison-lab/loadgen-tracing/internal/bench"
	"github.com/tech-comparison-lab/loadgen-tracing/internal/report"
)

func main() {
	db := flag.String("db", "", "backend: jaeger | zipkin (required)")
	op := flag.String("op", "all", "operation: write|list-services|find-traces|find-operations|find-trace|all")
	count := flag.Int("count", 200_000, "total spans to write")
	services := flag.Int("services", 50, "distinct synthetic services")
	batchSize := flag.Int("batch", 500, "spans per ingest request")
	workers := flag.Int("workers", 4, "concurrent ingest workers")
	queryIter := flag.Int("query-iter", 20, "iterations per query benchmark")
	addr := flag.String("addr", "", "ingest base URL, e.g. http://localhost:9411 (or TRACING_ADDR env)")
	queryAddr := flag.String("query-addr", "", "query base URL if it differs from --addr (Jaeger)")
	out := flag.String("out", "", "write JSON results to file (optional)")
	dryRun := flag.Bool("dry-run", false, "test connectivity only")
	flag.Parse()

	if *db != "jaeger" && *db != "zipkin" {
		fmt.Fprintln(os.Stderr, "error: --db must be jaeger or zipkin")
		flag.Usage()
		os.Exit(1)
	}
	if err := validateConfig(*op, *count, *services, *batchSize, *workers, *queryIter); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if *addr == "" {
		*addr = os.Getenv("TRACING_ADDR")
	}
	if *addr == "" {
		defaults := map[string]string{
			"jaeger": "http://localhost:9411",
			"zipkin": "http://localhost:9412",
		}
		*addr = defaults[*db]
	}
	if *queryAddr == "" {
		queryDefaults := map[string]string{
			"jaeger": "http://localhost:16686",
			"zipkin": *addr,
		}
		*queryAddr = queryDefaults[*db]
	}

	ctx := context.Background()
	b := bench.New(*db, *addr, *queryAddr, *services)

	if err := b.Ping(ctx); err != nil {
		log.Fatalf("%s: connect failed: %v", *db, err)
	}
	fmt.Printf("%s: connected OK (ingest=%s query=%s)\n", *db, *addr, *queryAddr)
	if *dryRun {
		return
	}

	var results []report.Result

	if *op == "write" || *op == "all" {
		fmt.Printf("%s: writing %d spans across %d services (batch=%d workers=%d)...\n",
			*db, *count, *services, *batchSize, *workers)
		durs, total, err := b.Ingest(ctx, *count, *batchSize, *workers)
		if err != nil {
			log.Fatalf("%s: write: %v", *db, err)
		}
		results = append(results, report.Compute(*db, "write", *count, 0, durs, total))
		fmt.Printf("%s: write done in %s\n", *db, total.Round(time.Millisecond))
		// Give the backend a moment to index freshly ingested spans before querying.
		time.Sleep(2 * time.Second)
	}

	queries := []struct {
		name string
		fn   func(context.Context, int) ([]time.Duration, time.Duration, error)
	}{
		{"list-services", b.ListServices},
		{"find-traces", b.FindTraces},
		{"find-operations", b.FindOperations},
		{"find-trace", b.FindTrace},
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

func validateConfig(op string, count, services, batchSize, workers, queryIter int) error {
	validOps := map[string]bool{
		"write": true, "list-services": true, "find-traces": true,
		"find-operations": true, "find-trace": true, "all": true,
	}
	if !validOps[op] {
		return fmt.Errorf("unknown --op %q, want write|list-services|find-traces|find-operations|find-trace|all", op)
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
	return nil
}
