package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/tech-comparison-lab/loadgen-gitops/internal/bench"
	"github.com/tech-comparison-lab/loadgen-gitops/internal/gitea"
	"github.com/tech-comparison-lab/loadgen-gitops/internal/k8s"
	"github.com/tech-comparison-lab/loadgen-gitops/internal/report"
)

func main() {
	tool        := flag.String("tool", "", "gitops tool: argocd|flux (required)")
	op          := flag.String("op", "all", "operation: sync-latency|reconcile|bulk|all")
	workload    := flag.String("workload", "configmap", "resource type: configmap|deployment|mixed")
	count       := flag.Int("count", 20, "iterations for sync-latency and reconcile")
	bulkSize    := flag.Int("bulk-size", 10, "resources pushed in a single bulk commit")
	kubeconfig  := flag.String("kubeconfig", "", "path to kubeconfig (default: ~/.kube/config)")
	kctx        := flag.String("context", "", "kubeconfig context (default: current)")
	namespace   := flag.String("namespace", "", "k8s namespace for benchmark resources")
	giteaURL    := flag.String("gitea-url", "http://localhost:3000", "Gitea base URL")
	giteaUser   := flag.String("gitea-user", "benchadmin", "Gitea username")
	giteaToken  := flag.String("gitea-token", "", "Gitea API token (preferred)")
	giteaPass   := flag.String("gitea-pass", "", "Gitea password (fallback when token is empty)")
	giteaRepo   := flag.String("gitea-repo", "", "Gitea repository name (required)")
	syncTimeout := flag.Duration("sync-timeout", 120*time.Second, "per-iteration sync timeout")
	out         := flag.String("out", "", "write JSON results to this file")
	dryRun      := flag.Bool("dry-run", false, "test connectivity only, no benchmarks")
	flag.Parse()

	if *tool == "" || (*tool != "argocd" && *tool != "flux") {
		fmt.Fprintln(os.Stderr, "error: --tool is required (argocd|flux)")
		flag.Usage()
		os.Exit(1)
	}
	if *giteaRepo == "" {
		fmt.Fprintln(os.Stderr, "error: --gitea-repo is required")
		flag.Usage()
		os.Exit(1)
	}
	if *giteaToken == "" && *giteaPass == "" {
		fmt.Fprintln(os.Stderr, "error: --gitea-token or --gitea-pass is required")
		flag.Usage()
		os.Exit(1)
	}
	if *namespace == "" {
		*namespace = "bench-" + *tool
	}

	gc := gitea.NewClient(*giteaURL, *giteaUser, *giteaToken, *giteaPass)
	ctx := context.Background()

	if *dryRun {
		fmt.Println("gitea: testing connectivity...")
		if err := gc.Ping(ctx); err != nil {
			log.Fatalf("gitea ping: %v", err)
		}
		fmt.Printf("gitea: ok (%s)\n", *giteaURL)
		fmt.Println("k8s: testing connectivity...")
		kc, err := k8s.NewClient(*kubeconfig, *kctx)
		if err != nil {
			log.Fatalf("k8s: %v", err)
		}
		if err := kc.Ping(ctx); err != nil {
			log.Fatalf("k8s ping: %v", err)
		}
		fmt.Println("k8s: ok")
		return
	}

	kc, err := k8s.NewClient(*kubeconfig, *kctx)
	if err != nil {
		log.Fatalf("k8s: %v", err)
	}

	cfg := bench.Config{
		Tool:        *tool,
		Workload:    *workload,
		Count:       *count,
		BulkSize:    *bulkSize,
		Namespace:   *namespace,
		GiteaRepo:   *giteaRepo,
		SyncTimeout: *syncTimeout,
	}

	results, err := bench.Run(ctx, gc, kc, *op, cfg)
	if err != nil {
		log.Fatalf("bench: %v", err)
	}

	report.PrintTable(results)

	if *out != "" {
		s := report.Summary{
			Tool:      *tool,
			RunID:     fmt.Sprintf("%s-%d", *tool, time.Now().Unix()),
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Results:   results,
		}
		if err := report.WriteJSON(s, *out); err != nil {
			log.Printf("warning: could not write JSON: %v", err)
		} else {
			fmt.Printf("\nResults saved to %s\n", *out)
		}
	}
}
