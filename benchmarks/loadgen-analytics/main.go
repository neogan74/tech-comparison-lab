package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	chbench "github.com/tech-comparison-lab/loadgen-analytics/internal/clickhouse"
	pgbench "github.com/tech-comparison-lab/loadgen-analytics/internal/postgres"
	"github.com/tech-comparison-lab/loadgen-analytics/internal/report"
)

func main() {
	db := flag.String("db", "", "database: clickhouse | postgres (required)")
	op := flag.String("op", "all", "operation: insert|count-group|time-range|agg-revenue|distinct-users|all")
	count := flag.Int("count", 10_000_000, "rows to insert")
	batchSize := flag.Int("batch", 100_000, "rows per insert batch")
	workers := flag.Int("workers", 4, "concurrent insert workers")
	queryIter := flag.Int("query-iter", 5, "iterations per query benchmark")
	addr := flag.String("addr", "", "connection address (or CH_ADDR / PG_DSN env)")
	out := flag.String("out", "", "write JSON results to file (optional)")
	truncate := flag.Bool("truncate", false, "truncate table before running")
	dryRun := flag.Bool("dry-run", false, "test connectivity only")
	flag.Parse()

	if *db == "" {
		fmt.Fprintln(os.Stderr, "error: --db is required (clickhouse | postgres)")
		flag.Usage()
		os.Exit(1)
	}
	if *db != "clickhouse" && *db != "postgres" {
		fmt.Fprintf(os.Stderr, "error: unknown --db %q\n", *db)
		os.Exit(1)
	}
	if *addr == "" {
		envKey := map[string]string{"clickhouse": "CH_ADDR", "postgres": "PG_DSN"}[*db]
		*addr = os.Getenv(envKey)
	}
	if *addr == "" {
		defaults := map[string]string{
			"clickhouse": "localhost:9000",
			"postgres":   "postgres://bench:benchpass@localhost:5433/bench?sslmode=disable",
		}
		*addr = defaults[*db]
	}

	ctx := context.Background()
	var results []report.Result

	cfg := runConfig{
		op: *op, count: *count, batchSize: *batchSize,
		workers: *workers, queryIter: *queryIter,
		truncate: *truncate, dryRun: *dryRun,
	}

	switch *db {
	case "clickhouse":
		results = runClickhouse(ctx, *addr, cfg)
	case "postgres":
		results = runPostgres(ctx, *addr, cfg)
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

type runConfig struct {
	op        string
	count     int
	batchSize int
	workers   int
	queryIter int
	truncate  bool
	dryRun    bool
}

func runClickhouse(ctx context.Context, addr string, cfg runConfig) []report.Result {
	bench, err := chbench.New(ctx, addr)
	if err != nil {
		log.Fatalf("clickhouse connect: %v", err)
	}
	defer bench.Close()
	fmt.Println("clickhouse: connected OK")

	if cfg.dryRun {
		return nil
	}
	if err := bench.Setup(ctx); err != nil {
		log.Fatalf("clickhouse setup: %v", err)
	}
	if cfg.truncate {
		if err := bench.Truncate(ctx); err != nil {
			log.Fatalf("clickhouse truncate: %v", err)
		}
		fmt.Println("clickhouse: truncated events table")
	}

	var results []report.Result

	if cfg.op == "insert" || cfg.op == "all" {
		fmt.Printf("clickhouse: insert %d rows (batch=%d workers=%d)...\n",
			cfg.count, cfg.batchSize, cfg.workers)
		durs, total, err := bench.Insert(ctx, cfg.count, cfg.batchSize, cfg.workers)
		if err != nil {
			log.Fatalf("clickhouse insert: %v", err)
		}
		r := report.Compute("clickhouse", "insert", cfg.count, 0, durs, total)
		r.StorageBytes, _ = bench.StorageSize(ctx)
		results = append(results, r)
		fmt.Printf("clickhouse: insert done in %s\n", total.Round(time.Millisecond))
	}

	queries := []struct {
		name string
		fn   func(context.Context, int) ([]time.Duration, time.Duration, error)
	}{
		{"count-group", bench.CountGroup},
		{"time-range", bench.TimeRange},
		{"agg-revenue", bench.AggRevenue},
		{"distinct-users", bench.DistinctUsers},
	}
	for _, q := range queries {
		if cfg.op == q.name || cfg.op == "all" {
			fmt.Printf("clickhouse: %s (%d iters)...\n", q.name, cfg.queryIter)
			durs, total, err := q.fn(ctx, cfg.queryIter)
			if err != nil {
				log.Fatalf("clickhouse %s: %v", q.name, err)
			}
			results = append(results, report.Compute("clickhouse", q.name, cfg.count, cfg.queryIter, durs, total))
		}
	}
	return results
}

func runPostgres(ctx context.Context, dsn string, cfg runConfig) []report.Result {
	bench, err := pgbench.New(ctx, dsn)
	if err != nil {
		log.Fatalf("postgres connect: %v", err)
	}
	defer bench.Close()
	fmt.Println("postgres: connected OK")

	if cfg.dryRun {
		return nil
	}
	if cfg.truncate {
		if err := bench.Truncate(ctx); err != nil {
			log.Fatalf("postgres truncate: %v", err)
		}
		fmt.Println("postgres: truncated events table")
	}

	var results []report.Result

	if cfg.op == "insert" || cfg.op == "all" {
		fmt.Printf("postgres: insert %d rows (batch=%d workers=%d)...\n",
			cfg.count, cfg.batchSize, cfg.workers)
		durs, total, err := bench.Insert(ctx, cfg.count, cfg.batchSize, cfg.workers)
		if err != nil {
			log.Fatalf("postgres insert: %v", err)
		}
		r := report.Compute("postgres", "insert", cfg.count, 0, durs, total)
		r.StorageBytes, _ = bench.StorageSize(ctx)
		results = append(results, r)
		fmt.Printf("postgres: insert done in %s\n", total.Round(time.Millisecond))
	}

	queries := []struct {
		name string
		fn   func(context.Context, int) ([]time.Duration, time.Duration, error)
	}{
		{"count-group", bench.CountGroup},
		{"time-range", bench.TimeRange},
		{"agg-revenue", bench.AggRevenue},
		{"distinct-users", bench.DistinctUsers},
	}
	for _, q := range queries {
		if cfg.op == q.name || cfg.op == "all" {
			fmt.Printf("postgres: %s (%d iters)...\n", q.name, cfg.queryIter)
			durs, total, err := q.fn(ctx, cfg.queryIter)
			if err != nil {
				log.Fatalf("postgres %s: %v", q.name, err)
			}
			results = append(results, report.Compute("postgres", q.name, cfg.count, cfg.queryIter, durs, total))
		}
	}
	return results
}
