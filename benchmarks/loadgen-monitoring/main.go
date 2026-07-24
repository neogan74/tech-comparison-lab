package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/tech-comparison-lab/loadgen-monitoring/internal/bench"
	"github.com/tech-comparison-lab/loadgen-monitoring/internal/prom"
	"github.com/tech-comparison-lab/loadgen-monitoring/internal/report"
	"github.com/tech-comparison-lab/loadgen-monitoring/internal/zabbix"
)

func main() {
	db := flag.String("db", "", "system: prometheus | zabbix (required)")
	op := flag.String("op", "all", "operation: write|latest|filtered|history|all")
	count := flag.Int("count", 2_000_000, "total samples to write")
	series := flag.Int("series", 2_000, "unique series / trapper items (cardinality)")
	interval := flag.Int("interval", 15, "seconds between samples of the same series")
	batchSize := flag.Int("batch", 2_000, "samples per ingest request")
	workers := flag.Int("workers", 4, "concurrent ingest workers")
	queryIter := flag.Int("query-iter", 5, "iterations per query benchmark")
	addr := flag.String("addr", "", "prometheus base URL or zabbix api_jsonrpc.php URL (or MON_ADDR env)")
	zbxSender := flag.String("zbx-sender", "localhost:10051", "zabbix trapper host:port")
	zbxUser := flag.String("zbx-user", "Admin", "zabbix API user")
	zbxPass := flag.String("zbx-pass", "zabbix", "zabbix API password")
	out := flag.String("out", "", "write JSON results to file (optional)")
	dryRun := flag.Bool("dry-run", false, "test connectivity only")
	flag.Parse()

	if *db != "prometheus" && *db != "zabbix" {
		fmt.Fprintln(os.Stderr, "error: --db must be prometheus or zabbix")
		flag.Usage()
		os.Exit(1)
	}
	if err := validateConfig(*op, *count, *series, *interval, *batchSize, *workers, *queryIter); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if *addr == "" {
		*addr = os.Getenv("MON_ADDR")
	}
	if *addr == "" {
		defaults := map[string]string{
			"prometheus": "http://localhost:9090",
			"zabbix":     "http://localhost:8090/api_jsonrpc.php",
		}
		*addr = defaults[*db]
	}

	var backend bench.Backend
	switch *db {
	case "prometheus":
		backend = prom.New(*addr)
	case "zabbix":
		backend = zabbix.New(*addr, *zbxSender, *zbxUser, *zbxPass)
	}

	ctx := context.Background()
	if err := backend.Ping(ctx); err != nil {
		log.Fatalf("%s: connect failed: %v", *db, err)
	}
	fmt.Printf("%s: connected OK (%s)\n", *db, *addr)
	if *dryRun {
		return
	}

	fmt.Printf("%s: provisioning %d series...\n", *db, *series)
	if err := backend.Provision(ctx, *series); err != nil {
		log.Fatalf("%s: provision: %v", *db, err)
	}

	var results []report.Result

	if *op == "write" || *op == "all" {
		fmt.Printf("%s: writing %d samples across %d series (batch=%d workers=%d)...\n",
			*db, *count, *series, *batchSize, *workers)
		durs, total, err := backend.Ingest(ctx, *count, *series, *batchSize, *workers, *interval)
		if err != nil {
			log.Fatalf("%s: write: %v", *db, err)
		}
		r := report.Compute(*db, "write", *count, 0, durs, total)
		r.StorageBytes, _ = backend.StorageBytes(ctx)
		results = append(results, r)
		fmt.Printf("%s: write done in %s\n", *db, total.Round(time.Millisecond))
	}

	for _, qop := range bench.Ops {
		if *op == qop || *op == "all" {
			fmt.Printf("%s: %s (%d iters)...\n", *db, qop, *queryIter)
			durs, total, err := backend.Query(ctx, qop, *queryIter)
			if err != nil {
				log.Fatalf("%s: %s: %v", *db, qop, err)
			}
			results = append(results, report.Compute(*db, qop, *series, *queryIter, durs, total))
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
		"write": true, "latest": true, "filtered": true,
		"history": true, "all": true,
	}
	if !validOps[op] {
		return fmt.Errorf("unknown --op %q, want write|latest|filtered|history|all", op)
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
