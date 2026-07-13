package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/tech-comparison-lab/loadgen-cache/internal/client"
	"github.com/tech-comparison-lab/loadgen-cache/internal/report"
)

func main() {
	db := flag.String("db", "", "database: redis | valkey | dragonfly | memcached (required)")
	op := flag.String("op", "all", "operation: set|get|pipeline-set|pipeline-get|mixed|all")
	count := flag.Int("count", 1000000, "total keys for set/pipeline-set; ignored for get/mixed (use --iterations)")
	iterations := flag.Int("iterations", 100000, "iterations for get/pipeline-get/mixed")
	pipeSize := flag.Int("pipe-size", 100, "commands per pipeline batch (or multi-get batch for memcached)")
	workers := flag.Int("workers", 16, "concurrent goroutines")
	addr := flag.String("addr", "", "server address host:port (or REDIS_ADDR / VALKEY_ADDR / MEMCACHED_ADDR env)")
	out := flag.String("out", "", "write JSON results to this file (optional)")
	flush := flag.Bool("flush", false, "flush all data before running (WARNING: deletes all data)")
	dryRun := flag.Bool("dry-run", false, "test connectivity only")
	flag.Parse()

	if *db == "" {
		fmt.Fprintln(os.Stderr, "error: --db is required (redis | valkey | dragonfly | memcached)")
		flag.Usage()
		os.Exit(1)
	}

	cfg := runConfig{
		op: *op, count: *count, iterations: *iterations,
		pipeSize: *pipeSize, workers: *workers,
	}
	if err := validateConfig(*db, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if *addr == "" {
		envKey := map[string]string{
			"redis":     "REDIS_ADDR",
			"valkey":    "VALKEY_ADDR",
			"dragonfly": "DRAGONFLY_ADDR",
			"memcached": "MEMCACHED_ADDR",
		}[*db]
		*addr = os.Getenv(envKey)
	}
	if *addr == "" {
		defaults := map[string]string{
			"redis":     "localhost:6379",
			"valkey":    "localhost:6380",
			"dragonfly": "localhost:6384",
			"memcached": "localhost:11211",
		}
		*addr = defaults[*db]
	}

	ctx := context.Background()
	bench, err := client.New(*db, *addr)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer bench.Close()
	fmt.Printf("%s: connected to %s\n", *db, *addr)

	if *dryRun {
		return
	}
	if *flush {
		if err := bench.FlushAll(ctx); err != nil {
			log.Fatalf("flushall: %v", err)
		}
		fmt.Printf("%s: flushed\n", *db)
	}

	results := run(ctx, bench, *db, cfg)

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
	iterations int
	pipeSize   int
	workers    int
}

func validateConfig(db string, cfg runConfig) error {
	validOps := map[string]bool{
		"set":          true,
		"get":          true,
		"pipeline-set": true,
		"pipeline-get": true,
		"mixed":        true,
		"all":          true,
	}
	if !validOps[cfg.op] {
		return fmt.Errorf("unknown --op %q, want set|get|pipeline-set|pipeline-get|mixed|all", cfg.op)
	}
	validDBs := map[string]bool{"redis": true, "valkey": true, "dragonfly": true, "memcached": true}
	if !validDBs[db] {
		return fmt.Errorf("unknown --db %q", db)
	}
	if cfg.count < 1 {
		return fmt.Errorf("--count must be >= 1")
	}
	if cfg.iterations < 1 {
		return fmt.Errorf("--iterations must be >= 1")
	}
	if cfg.pipeSize < 1 {
		return fmt.Errorf("--pipe-size must be >= 1")
	}
	if cfg.workers < 1 {
		return fmt.Errorf("--workers must be >= 1")
	}
	return nil
}

func run(ctx context.Context, bench client.Client, db string, cfg runConfig) []report.Result {
	var results []report.Result

	// For "all": pipeline-set first (fills keys), then query ops
	runSet := cfg.op == "set"
	runPipeSet := cfg.op == "pipeline-set" || cfg.op == "all"
	runGet := cfg.op == "get" || cfg.op == "all"
	runPipeGet := cfg.op == "pipeline-get" || cfg.op == "all"
	runMixed := cfg.op == "mixed" || cfg.op == "all"

	if runSet {
		fmt.Printf("%s: set %d keys (workers=%d)...\n", db, cfg.count, cfg.workers)
		durs, total, err := bench.Set(ctx, cfg.count, cfg.workers)
		if err != nil {
			log.Fatalf("%s set: %v", db, err)
		}
		r := report.Compute(db, "set", cfg.count, cfg.workers, durs, total)
		r.MemoryUsed, _ = bench.MemoryUsed(ctx)
		r.KeyCount, _ = bench.KeyCount(ctx)
		results = append(results, r)
		fmt.Printf("%s: set done in %s\n", db, total.Round(time.Millisecond))
	}

	if runPipeSet {
		totalCmds := cfg.count
		fmt.Printf("%s: pipeline-set %d keys (pipe=%d workers=%d)...\n", db, totalCmds, cfg.pipeSize, cfg.workers)
		durs, total, err := bench.PipelineSet(ctx, totalCmds, cfg.pipeSize, cfg.workers)
		if err != nil {
			log.Fatalf("%s pipeline-set: %v", db, err)
		}
		r := report.Compute(db, "pipeline-set", totalCmds, cfg.workers, durs, total)
		r.MemoryUsed, _ = bench.MemoryUsed(ctx)
		r.KeyCount, _ = bench.KeyCount(ctx)
		results = append(results, r)
		fmt.Printf("%s: pipeline-set done in %s\n", db, total.Round(time.Millisecond))
	}

	if runGet {
		fmt.Printf("%s: get %d iterations (workers=%d)...\n", db, cfg.iterations, cfg.workers)
		durs, total, err := bench.Get(ctx, cfg.iterations, cfg.workers, cfg.count)
		if err != nil {
			log.Fatalf("%s get: %v", db, err)
		}
		results = append(results, report.Compute(db, "get", cfg.iterations, cfg.workers, durs, total))
		fmt.Printf("%s: get done in %s\n", db, total.Round(time.Millisecond))
	}

	if runPipeGet {
		totalCmds := cfg.iterations * cfg.pipeSize
		fmt.Printf("%s: pipeline-get %d iterations x %d cmds (workers=%d)...\n",
			db, cfg.iterations, cfg.pipeSize, cfg.workers)
		durs, total, err := bench.PipelineGet(ctx, cfg.iterations, cfg.pipeSize, cfg.workers, cfg.count)
		if err != nil {
			log.Fatalf("%s pipeline-get: %v", db, err)
		}
		r := report.Compute(db, "pipeline-get", totalCmds, cfg.workers, durs, total)
		results = append(results, r)
		fmt.Printf("%s: pipeline-get done in %s\n", db, total.Round(time.Millisecond))
	}

	if runMixed {
		totalCmds := cfg.iterations * cfg.pipeSize
		fmt.Printf("%s: mixed %d iterations x %d cmds 80%%GET/20%%SET (workers=%d)...\n",
			db, cfg.iterations, cfg.pipeSize, cfg.workers)
		durs, total, err := bench.Mixed(ctx, cfg.iterations, cfg.pipeSize, cfg.workers, cfg.count)
		if err != nil {
			log.Fatalf("%s mixed: %v", db, err)
		}
		results = append(results, report.Compute(db, "mixed", totalCmds, cfg.workers, durs, total))
		fmt.Printf("%s: mixed done in %s\n", db, total.Round(time.Millisecond))
	}

	return results
}
