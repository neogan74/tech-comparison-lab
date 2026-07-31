package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/tech-comparison-lab/loadgen-mesh/internal/client"
	"github.com/tech-comparison-lab/loadgen-mesh/internal/dataplane"
	"github.com/tech-comparison-lab/loadgen-mesh/internal/mesh"
	"github.com/tech-comparison-lab/loadgen-mesh/internal/report"
)

func main() {
	var (
		kubeconfig = flag.String("kubeconfig", client.DefaultKubeconfig(), "path to kubeconfig")
		kctx       = flag.String("context", "", "kubeconfig context (default: current)")
		meshName   = flag.String("mesh", "", "service mesh: istio | linkerd (required)")
		namespace  = flag.String("namespace", "", "workload namespace (default: bench-<mesh>)")
		op         = flag.String("op", "all", "benchmark: all|inject|footprint|data-plane")
		replicas   = flag.Int("replicas", 3, "echo replicas for the inject benchmark")
		rounds     = flag.Int("rounds", 3, "rounds for the inject benchmark")
		count      = flag.Int("count", 5000, "requests for the data-plane benchmark")
		workers    = flag.Int("workers", 25, "concurrent workers for the data-plane benchmark")
		addr       = flag.String("addr", "", "data-plane target URL (e.g. http://localhost:8080)")
		label      = flag.String("label", "meshed", "label for the data-plane result (meshed|baseline)")
		outFile    = flag.String("out", "", "output JSON file")
	)
	flag.Parse()

	spec, ok := mesh.Specs[*meshName]
	if !ok {
		fmt.Fprintln(os.Stderr, "error: --mesh must be istio or linkerd")
		flag.Usage()
		os.Exit(1)
	}
	if *op != "all" && *op != "inject" && *op != "footprint" && *op != "data-plane" {
		fmt.Fprintf(os.Stderr, "error: unknown --op %q (all|inject|footprint|data-plane)\n", *op)
		os.Exit(1)
	}
	if *replicas < 1 || *rounds < 1 || *count < 1 || *workers < 1 {
		fmt.Fprintln(os.Stderr, "error: --replicas, --rounds, --count, --workers must be >= 1")
		os.Exit(1)
	}
	ns := *namespace
	if ns == "" {
		ns = "bench-" + spec.Name
	}

	cfg, err := client.BuildConfig(*kubeconfig, *kctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "kubeconfig: %v\n", err)
		os.Exit(1)
	}
	cs, err := client.NewClientset(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "clientset: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	fmt.Printf("mesh: %s  namespace: %s  op: %s\n", spec.Name, ns, *op)

	var results []report.Result

	if *op == "inject" || *op == "all" {
		fmt.Println("→ inject...")
		r, err := mesh.RunInject(ctx, cs, spec, ns, *replicas, *rounds)
		if err != nil {
			fmt.Fprintf(os.Stderr, "inject: %v\n", err)
			os.Exit(1)
		}
		results = append(results, r...)
	}

	if *op == "footprint" || *op == "all" {
		fmt.Println("→ footprint...")
		// footprint reads an injected echo pod; ensure one exists.
		if err := mesh.EnsureNamespace(ctx, cs, spec, ns, true); err != nil {
			fmt.Fprintf(os.Stderr, "footprint namespace: %v\n", err)
			os.Exit(1)
		}
		if err := mesh.EnsureEcho(ctx, cs, ns, 1); err != nil {
			fmt.Fprintf(os.Stderr, "footprint echo: %v\n", err)
			os.Exit(1)
		}
		r, err := mesh.RunFootprint(ctx, cs, spec, ns)
		if err != nil {
			fmt.Fprintf(os.Stderr, "footprint: %v\n", err)
			os.Exit(1)
		}
		results = append(results, r...)
	}

	if *op == "data-plane" || *op == "all" {
		fmt.Printf("→ data-plane (%s)...\n", *label)
		target := *addr
		if target == "" {
			fmt.Fprintln(os.Stderr, "data-plane: --addr is required")
			os.Exit(1)
		}
		r, err := dataplane.Run(ctx, target, *label, *count, *workers)
		if err != nil {
			fmt.Fprintf(os.Stderr, "data-plane: %v\n", err)
			os.Exit(1)
		}
		results = append(results, r...)
	}

	report.StampMesh(spec.Name, results)
	rep := report.Report{Mesh: spec.Name, Timestamp: time.Now().UTC(), Results: results}
	report.PrintTable(os.Stdout, rep)

	if *outFile != "" {
		if err := report.WriteJSON(*outFile, rep); err != nil {
			fmt.Fprintf(os.Stderr, "write json: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("\nResults saved to %s\n", *outFile)
	}
}
