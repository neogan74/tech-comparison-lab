package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/tech-comparison-lab/loadgen-kv/internal/client"
	"github.com/tech-comparison-lab/loadgen-kv/internal/report"
)

func main() {
	db := flag.String("db", "", "database: etcd | consul (required)")
	op := flag.String("op", "all", "operation: write|read|mixed|watch|election|all")
	count := flag.Int("count", 10000, "operations per benchmark run")
	workers := flag.Int("workers", 8, "concurrent goroutines (write/read/mixed only)")
	watchCount := flag.Int("watch-count", 100, "watch event measurements (--op watch)")
	electionCount := flag.Int("election-count", 20, "election measurements (--op election)")
	addr := flag.String("addr", "", "host:port (etcd default: localhost:2379, consul default: localhost:8500)")
	out := flag.String("out", "", "write JSON results to this file")
	dryRun := flag.Bool("dry-run", false, "test connectivity only")
	flag.Parse()

	if *db == "" {
		fmt.Fprintln(os.Stderr, "error: --db is required (etcd | consul)")
		flag.Usage()
		os.Exit(1)
	}
	if err := validateArgs(*db, *op); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if *addr == "" {
		envKey := map[string]string{"etcd": "ETCD_ADDR", "consul": "CONSUL_ADDR"}[*db]
		*addr = os.Getenv(envKey)
	}
	if *addr == "" {
		*addr = map[string]string{"etcd": "localhost:2379", "consul": "localhost:8500"}[*db]
	}

	cli, err := client.New(*db, *addr)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer cli.Close()
	fmt.Printf("%s: connected to %s\n", *db, *addr)

	if *dryRun {
		return
	}

	ctx := context.Background()
	results := run(ctx, cli, *db, *op, *count, *workers, *watchCount, *electionCount)

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

func validateArgs(db, op string) error {
	validDBs := map[string]bool{"etcd": true, "consul": true}
	if !validDBs[db] {
		return fmt.Errorf("unknown --db %q, want etcd|consul", db)
	}
	validOps := map[string]bool{
		"write": true, "read": true, "mixed": true,
		"watch": true, "election": true, "all": true,
	}
	if !validOps[op] {
		return fmt.Errorf("unknown --op %q, want write|read|mixed|watch|election|all", op)
	}
	return nil
}

func run(ctx context.Context, cli client.KVClient, db, op string, count, workers, watchCount, electionCount int) []report.Result {
	var results []report.Result

	runWrite := op == "write" || op == "all"
	runRead := op == "read" || op == "all"
	runMixed := op == "mixed" || op == "all"
	runWatch := op == "watch" || op == "all"
	runElection := op == "election" || op == "all"

	// Seed data for read/mixed ops.
	if runRead || runMixed {
		fmt.Printf("%s: seeding %d keys...\n", db, count)
		if _, _, err := cli.Write(ctx, count, workers); err != nil {
			log.Fatalf("%s seed write: %v", db, err)
		}
	}

	if runWrite {
		fmt.Printf("%s: write %d ops (workers=%d)...\n", db, count, workers)
		durs, total, err := cli.Write(ctx, count, workers)
		if err != nil {
			log.Fatalf("%s write: %v", db, err)
		}
		results = append(results, report.Compute(db, "write", count, workers, durs, total))
		fmt.Printf("%s: write done in %s\n", db, total.Round(time.Millisecond))
	}

	if runRead {
		fmt.Printf("%s: read %d ops (workers=%d)...\n", db, count, workers)
		durs, total, err := cli.Read(ctx, count, workers)
		if err != nil {
			log.Fatalf("%s read: %v", db, err)
		}
		results = append(results, report.Compute(db, "read", count, workers, durs, total))
		fmt.Printf("%s: read done in %s\n", db, total.Round(time.Millisecond))
	}

	if runMixed {
		fmt.Printf("%s: mixed %d ops 80%%read/20%%write (workers=%d)...\n", db, count, workers)
		durs, total, err := cli.Mixed(ctx, count, workers)
		if err != nil {
			log.Fatalf("%s mixed: %v", db, err)
		}
		results = append(results, report.Compute(db, "mixed", count, workers, durs, total))
		fmt.Printf("%s: mixed done in %s\n", db, total.Round(time.Millisecond))
	}

	if runWatch {
		fmt.Printf("%s: watch %d measurements...\n", db, watchCount)
		durs, total, err := cli.Watch(ctx, watchCount, workers)
		if err != nil {
			log.Fatalf("%s watch: %v", db, err)
		}
		results = append(results, report.Compute(db, "watch", watchCount, 1, durs, total))
		fmt.Printf("%s: watch done in %s\n", db, total.Round(time.Millisecond))
	}

	if runElection {
		fmt.Printf("%s: election %d measurements...\n", db, electionCount)
		durs, total, err := cli.Election(ctx, electionCount)
		if err != nil {
			log.Fatalf("%s election: %v", db, err)
		}
		results = append(results, report.Compute(db, "election", electionCount, 1, durs, total))
		fmt.Printf("%s: election done in %s\n", db, total.Round(time.Millisecond))
	}

	// Cleanup benchmark keys.
	if err := cli.Cleanup(ctx); err != nil {
		log.Printf("%s cleanup warning: %v", db, err)
	}

	return results
}
