package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	mbench "github.com/tech-comparison-lab/loadgen-db/internal/mongo"
	pgbench "github.com/tech-comparison-lab/loadgen-db/internal/postgres"
	"github.com/tech-comparison-lab/loadgen-db/internal/report"
)

func main() {
	db := flag.String("db", "", "database: postgres | mongo (required)")
	op := flag.String("op", "all", "operation: insert | query | agg | update | all")
	count := flag.Int("count", 100000, "number of documents to insert")
	queryIter := flag.Int("query-iter", 0, "query iterations (default: min(count,1000))")
	aggIter := flag.Int("agg-iter", 0, "agg iterations (default: min(count/1000,10), min 1)")
	updateIter := flag.Int("update-iter", 0, "update iterations (default: min(count/100,100), min 1)")
	batch := flag.Int("batch", 1000, "batch size for inserts")
	workers := flag.Int("workers", 8, "number of concurrent workers")
	dsn := flag.String("dsn", "", "connection string (or use PG_DSN / MONGO_DSN env var)")
	out := flag.String("out", "", "write JSON results to this file (optional)")
	truncate := flag.Bool("truncate", false, "truncate/drop collection before running")
	dryRun := flag.Bool("dry-run", false, "test connectivity only, no benchmark")
	flag.Parse()

	if *db == "" {
		fmt.Fprintln(os.Stderr, "error: --db is required (postgres | mongo)")
		flag.Usage()
		os.Exit(1)
	}
	if *db != "postgres" && *db != "mongo" {
		fmt.Fprintf(os.Stderr, "error: unknown --db %q, want postgres or mongo\n", *db)
		os.Exit(1)
	}

	// Apply iteration defaults
	qi, ai, ui := resolveIters(*count, *queryIter, *aggIter, *updateIter)
	cfg := runConfig{
		op: *op, count: *count, queryIter: qi, aggIter: ai, updateIter: ui,
		batch: *batch, workers: *workers, truncate: *truncate, dryRun: *dryRun,
	}
	if err := validateConfig(*db, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// DSN fallback to env vars
	if *dsn == "" {
		switch *db {
		case "postgres":
			*dsn = os.Getenv("PG_DSN")
		case "mongo":
			*dsn = os.Getenv("MONGO_DSN")
		}
	}
	if *dsn == "" {
		fmt.Fprintf(os.Stderr, "error: --dsn not set and %s env var is empty\n",
			map[string]string{"postgres": "PG_DSN", "mongo": "MONGO_DSN"}[*db])
		os.Exit(1)
	}

	ctx := context.Background()
	var results []report.Result

	switch *db {
	case "postgres":
		results = runPostgres(ctx, *dsn, cfg)
	case "mongo":
		results = runMongo(ctx, *dsn, cfg)
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
				log.Printf("warning: could not write JSON to %s: %v", *out, err)
			} else {
				fmt.Printf("\nResults saved to %s\n", *out)
			}
		}
	}
}

type runConfig struct {
	op         string
	count      int
	queryIter  int
	aggIter    int
	updateIter int
	batch      int
	workers    int
	truncate   bool
	dryRun     bool
}

func resolveIters(count, qi, ai, ui int) (int, int, int) {
	if qi == 0 {
		qi = min(count, 1000)
	}
	if ai == 0 {
		ai = min(count/1000, 10)
		if ai < 1 {
			ai = 1
		}
	}
	if ui == 0 {
		ui = min(count/100, 100)
		if ui < 1 {
			ui = 1
		}
	}
	return qi, ai, ui
}

func validateConfig(db string, cfg runConfig) error {
	validOps := map[string]map[string]bool{
		"postgres": {
			"insert": true,
			"query":  true,
			"agg":    true,
			"update": true,
			"all":    true,
		},
		"mongo": {
			"insert": true,
			"query":  true,
			"agg":    true,
			"update": true,
			"all":    true,
		},
	}
	if !validOps[db][cfg.op] {
		return fmt.Errorf("unknown --op %q, want insert|query|agg|update|all", cfg.op)
	}
	if cfg.count < 1 {
		return fmt.Errorf("--count must be >= 1")
	}
	if cfg.batch < 1 {
		return fmt.Errorf("--batch must be >= 1")
	}
	if cfg.workers < 1 {
		return fmt.Errorf("--workers must be >= 1")
	}
	if cfg.queryIter < 1 {
		return fmt.Errorf("--query-iter must be >= 1")
	}
	if cfg.aggIter < 1 {
		return fmt.Errorf("--agg-iter must be >= 1")
	}
	if cfg.updateIter < 1 {
		return fmt.Errorf("--update-iter must be >= 1")
	}
	return nil
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
		fmt.Println("postgres: truncated orders table")
	}

	var results []report.Result

	if cfg.op == "insert" || cfg.op == "all" {
		fmt.Printf("postgres: insert %d docs (batch=%d workers=%d)...\n", cfg.count, cfg.batch, cfg.workers)
		durs, total, err := bench.Insert(ctx, cfg.count, cfg.batch, cfg.workers)
		if err != nil {
			log.Fatalf("postgres insert: %v", err)
		}
		r := report.Compute("postgres", "insert", cfg.count, cfg.workers, durs, total)
		r.StorageBytes, _ = bench.StorageSize(ctx)
		results = append(results, r)
		fmt.Printf("postgres: insert done in %s\n", total.Round(time.Millisecond))
	}

	if cfg.op == "query" || cfg.op == "all" {
		fmt.Printf("postgres: query %d iterations...\n", cfg.queryIter)
		durs, total, err := bench.Query(ctx, cfg.queryIter)
		if err != nil {
			log.Fatalf("postgres query: %v", err)
		}
		results = append(results, report.Compute("postgres", "query", cfg.queryIter, 1, durs, total))
	}

	if cfg.op == "agg" || cfg.op == "all" {
		fmt.Printf("postgres: agg %d iterations...\n", cfg.aggIter)
		durs, total, err := bench.Agg(ctx, cfg.aggIter)
		if err != nil {
			log.Fatalf("postgres agg: %v", err)
		}
		results = append(results, report.Compute("postgres", "agg", cfg.aggIter, 1, durs, total))
	}

	if cfg.op == "update" || cfg.op == "all" {
		fmt.Printf("postgres: update %d iterations...\n", cfg.updateIter)
		durs, total, err := bench.Update(ctx, cfg.updateIter)
		if err != nil {
			log.Fatalf("postgres update: %v", err)
		}
		results = append(results, report.Compute("postgres", "update", cfg.updateIter, 1, durs, total))
	}

	return results
}

func runMongo(ctx context.Context, dsn string, cfg runConfig) []report.Result {
	bench, err := mbench.New(ctx, dsn)
	if err != nil {
		log.Fatalf("mongo connect: %v", err)
	}
	defer bench.Close(ctx)
	fmt.Println("mongo: connected OK")

	if cfg.dryRun {
		return nil
	}
	if cfg.truncate {
		if err := bench.Drop(ctx); err != nil {
			log.Fatalf("mongo drop: %v", err)
		}
		fmt.Println("mongo: dropped orders collection")
	}

	idxDur, err := bench.EnsureIndexes(ctx)
	if err != nil {
		log.Fatalf("mongo ensure indexes: %v", err)
	}
	fmt.Printf("mongo: indexes ready (%s)\n", idxDur.Round(time.Millisecond))

	var results []report.Result

	if cfg.op == "insert" || cfg.op == "all" {
		fmt.Printf("mongo: insert %d docs (batch=%d workers=%d)...\n", cfg.count, cfg.batch, cfg.workers)
		durs, total, err := bench.Insert(ctx, cfg.count, cfg.batch, cfg.workers)
		if err != nil {
			log.Fatalf("mongo insert: %v", err)
		}
		r := report.Compute("mongo", "insert", cfg.count, cfg.workers, durs, total)
		r.IndexBuildMs = idxDur.Milliseconds()
		r.StorageBytes, _ = bench.StorageSize(ctx)
		results = append(results, r)
		fmt.Printf("mongo: insert done in %s\n", total.Round(time.Millisecond))
	}

	if cfg.op == "query" || cfg.op == "all" {
		fmt.Printf("mongo: query %d iterations...\n", cfg.queryIter)
		durs, total, err := bench.Query(ctx, cfg.queryIter)
		if err != nil {
			log.Fatalf("mongo query: %v", err)
		}
		results = append(results, report.Compute("mongo", "query", cfg.queryIter, 1, durs, total))
	}

	if cfg.op == "agg" || cfg.op == "all" {
		fmt.Printf("mongo: agg %d iterations...\n", cfg.aggIter)
		durs, total, err := bench.Agg(ctx, cfg.aggIter)
		if err != nil {
			log.Fatalf("mongo agg: %v", err)
		}
		results = append(results, report.Compute("mongo", "agg", cfg.aggIter, 1, durs, total))
	}

	if cfg.op == "update" || cfg.op == "all" {
		fmt.Printf("mongo: update %d iterations...\n", cfg.updateIter)
		durs, total, err := bench.Update(ctx, cfg.updateIter)
		if err != nil {
			log.Fatalf("mongo update: %v", err)
		}
		results = append(results, report.Compute("mongo", "update", cfg.updateIter, 1, durs, total))
	}

	return results
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
