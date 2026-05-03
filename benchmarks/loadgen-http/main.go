package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	graphqlbench "github.com/tech-comparison-lab/loadgen-http/internal/graphql"
	grpcbench "github.com/tech-comparison-lab/loadgen-http/internal/grpc"
	"github.com/tech-comparison-lab/loadgen-http/internal/report"
	restbench "github.com/tech-comparison-lab/loadgen-http/internal/rest"
)

func main() {
	proto := flag.String("proto", "", "protocol: rest | grpc | graphql (required)")
	op := flag.String("op", "all", "operation: echo|get-user|create-order|all")
	count := flag.Int("count", 10000, "total requests per operation")
	workers := flag.Int("workers", 50, "concurrent goroutines")
	addr := flag.String("addr", "", "server address (REST_ADDR, GRPC_ADDR, or GRAPHQL_ADDR env fallback)")
	out := flag.String("out", "", "write JSON results to file (optional)")
	flag.Parse()

	if *proto == "" {
		fmt.Fprintln(os.Stderr, "error: --proto is required (rest | grpc | graphql)")
		flag.Usage()
		os.Exit(1)
	}

	if *addr == "" {
		env := map[string]string{"rest": "REST_ADDR", "grpc": "GRPC_ADDR", "graphql": "GRAPHQL_ADDR"}[*proto]
		*addr = os.Getenv(env)
	}
	if *addr == "" {
		defaults := map[string]string{
			"rest":    "http://localhost:8080",
			"grpc":    "localhost:50051",
			"graphql": "http://localhost:8080",
		}
		*addr = defaults[*proto]
	}

	ctx := context.Background()
	ops := resolveOps(*op)

	var results []report.Result
	switch *proto {
	case "rest":
		results = runREST(ctx, *addr, ops, *count, *workers)
	case "grpc":
		results = runGRPC(ctx, *addr, ops, *count, *workers)
	case "graphql":
		results = runGraphQL(ctx, *addr, ops, *count, *workers)
	default:
		log.Fatalf("unknown --proto %q", *proto)
	}

	if len(results) > 0 {
		report.PrintTable(results)
		if *out != "" {
			summary := report.Summary{
				RunID:     fmt.Sprintf("%s-%d", *proto, time.Now().Unix()),
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

func resolveOps(op string) []string {
	if op == "all" {
		return []string{"echo", "get-user", "create-order"}
	}
	return []string{op}
}

func runREST(ctx context.Context, addr string, ops []string, count, workers int) []report.Result {
	bench := restbench.New(addr, workers)
	fmt.Printf("rest: targeting %s\n", addr)

	var results []report.Result
	for _, op := range ops {
		fmt.Printf("rest: %s × %d (workers=%d)...\n", op, count, workers)
		durs, total, errs, err := bench.RunOp(ctx, op, count, workers)
		if err != nil {
			log.Fatalf("rest %s: %v", op, err)
		}
		r := report.Compute("rest", op, count, workers, errs, durs, total)
		results = append(results, r)
		fmt.Printf("rest: %s done in %s  RPS=%.0f  errors=%d\n",
			op, total.Round(time.Millisecond), r.ThroughputRPS, errs)
	}
	return results
}

func runGRPC(ctx context.Context, addr string, ops []string, count, workers int) []report.Result {
	bench, err := grpcbench.New(addr)
	if err != nil {
		log.Fatalf("grpc connect: %v", err)
	}
	defer bench.Close()
	fmt.Printf("grpc: connected to %s\n", addr)

	var results []report.Result
	for _, op := range ops {
		fmt.Printf("grpc: %s × %d (workers=%d)...\n", op, count, workers)
		durs, total, errs, err := bench.RunOp(ctx, op, count, workers)
		if err != nil {
			log.Fatalf("grpc %s: %v", op, err)
		}
		r := report.Compute("grpc", op, count, workers, errs, durs, total)
		results = append(results, r)
		fmt.Printf("grpc: %s done in %s  RPS=%.0f  errors=%d\n",
			op, total.Round(time.Millisecond), r.ThroughputRPS, errs)
	}
	return results
}

func runGraphQL(ctx context.Context, addr string, ops []string, count, workers int) []report.Result {
	bench := graphqlbench.New(addr, workers)
	fmt.Printf("graphql: targeting %s/graphql\n", addr)

	var results []report.Result
	for _, op := range ops {
		fmt.Printf("graphql: %s × %d (workers=%d)...\n", op, count, workers)
		durs, total, errs, err := bench.RunOp(ctx, op, count, workers)
		if err != nil {
			log.Fatalf("graphql %s: %v", op, err)
		}
		r := report.Compute("graphql", op, count, workers, errs, durs, total)
		results = append(results, r)
		fmt.Printf("graphql: %s done in %s  RPS=%.0f  errors=%d\n",
			op, total.Round(time.Millisecond), r.ThroughputRPS, errs)
	}
	return results
}
