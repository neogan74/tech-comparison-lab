package bench

import (
	"context"
	"fmt"
	"time"

	"github.com/tech-comparison-lab/loadgen-gitops/internal/gitea"
	"github.com/tech-comparison-lab/loadgen-gitops/internal/k8s"
	"github.com/tech-comparison-lab/loadgen-gitops/internal/report"
)

// Config holds benchmark parameters.
type Config struct {
	Tool        string
	Count       int
	BulkSize    int
	Namespace   string
	GiteaRepo   string
	SyncTimeout time.Duration
}

// Run executes the requested operations and returns results.
func Run(ctx context.Context, gc *gitea.Client, kc *k8s.Client, op string, cfg Config) ([]report.Result, error) {
	if err := setup(ctx, gc, kc, cfg); err != nil {
		return nil, fmt.Errorf("setup: %w", err)
	}

	runSync      := op == "sync-latency" || op == "all"
	runReconcile := op == "reconcile" || op == "all"
	runBulk      := op == "bulk" || op == "all"

	var results []report.Result

	if runSync {
		fmt.Printf("[%s] sync-latency: %d iterations (timeout=%s)\n", cfg.Tool, cfg.Count, cfg.SyncTimeout)
		durs, err := benchSyncLatency(ctx, gc, kc, cfg)
		if err != nil {
			return results, fmt.Errorf("sync-latency: %w", err)
		}
		results = append(results, report.Compute(cfg.Tool, "sync-latency", cfg.Count, durs, sumDurations(durs)))
	}

	if runReconcile {
		fmt.Printf("[%s] reconcile: %d iterations\n", cfg.Tool, cfg.Count)
		durs, err := benchReconcile(ctx, kc, cfg)
		if err != nil {
			return results, fmt.Errorf("reconcile: %w", err)
		}
		results = append(results, report.Compute(cfg.Tool, "reconcile", cfg.Count, durs, sumDurations(durs)))
	}

	if runBulk {
		fmt.Printf("[%s] bulk: %d resources\n", cfg.Tool, cfg.BulkSize)
		dur, err := benchBulk(ctx, gc, kc, cfg)
		if err != nil {
			return results, fmt.Errorf("bulk: %w", err)
		}
		results = append(results, report.Result{
			Tool:    cfg.Tool,
			Op:      "bulk",
			Count:   cfg.BulkSize,
			TotalMs: dur.Milliseconds(),
		})
	}

	return results, nil
}

// setup initialises the k8s namespace and pushes the seed manifest to Gitea.
func setup(ctx context.Context, gc *gitea.Client, kc *k8s.Client, cfg Config) error {
	if err := kc.EnsureNamespace(ctx, cfg.Namespace); err != nil {
		return fmt.Errorf("namespace: %w", err)
	}
	if err := gc.EnsureRepo(ctx, cfg.GiteaRepo); err != nil {
		return fmt.Errorf("repo: %w", err)
	}

	// Push the reconcile anchor manifest used by the reconcile benchmark.
	_, sha, err := gc.GetFile(ctx, cfg.GiteaRepo, "manifests/cm-bench-init.yaml")
	if err != nil {
		return err
	}
	if _, err := gc.PutFile(ctx, cfg.GiteaRepo, "manifests/cm-bench-init.yaml",
		"init: bench anchor", []byte(configMapYAML("bench-init", cfg.Namespace)), sha); err != nil {
		return fmt.Errorf("push init manifest: %w", err)
	}
	return nil
}

// benchSyncLatency pushes individual CM manifests and measures time-to-sync for each.
func benchSyncLatency(ctx context.Context, gc *gitea.Client, kc *k8s.Client, cfg Config) ([]time.Duration, error) {
	var durs []time.Duration
	for i := 0; i < cfg.Count; i++ {
		cmName := fmt.Sprintf("bench-sync-%d", i)
		filePath := fmt.Sprintf("manifests/cm-bench-sync-%d.yaml", i)
		t := time.Now()
		if _, err := gc.PutFile(ctx, cfg.GiteaRepo, filePath,
			"bench: add "+cmName, []byte(configMapYAML(cmName, cfg.Namespace)), ""); err != nil {
			return durs, fmt.Errorf("iter %d push: %w", i, err)
		}
		if err := kc.WaitForCM(ctx, cfg.Namespace, cmName, cfg.SyncTimeout); err != nil {
			return durs, fmt.Errorf("iter %d wait: %w", i, err)
		}
		elapsed := time.Since(t)
		durs = append(durs, elapsed)
		fmt.Printf("  [%d/%d] %s appeared in %s\n", i+1, cfg.Count, cmName, elapsed.Round(time.Millisecond))
	}
	return durs, nil
}

// benchReconcile repeatedly deletes the anchor CM and measures re-apply time.
func benchReconcile(ctx context.Context, kc *k8s.Client, cfg Config) ([]time.Duration, error) {
	const cmName = "bench-init"
	var durs []time.Duration
	for i := 0; i < cfg.Count; i++ {
		// Ensure it exists before deleting.
		if err := kc.WaitForCM(ctx, cfg.Namespace, cmName, cfg.SyncTimeout); err != nil {
			return durs, fmt.Errorf("iter %d pre-check: %w", i, err)
		}
		if err := kc.DeleteCM(ctx, cfg.Namespace, cmName); err != nil {
			return durs, fmt.Errorf("iter %d delete: %w", i, err)
		}
		t := time.Now()
		if err := kc.WaitForCM(ctx, cfg.Namespace, cmName, cfg.SyncTimeout); err != nil {
			return durs, fmt.Errorf("iter %d re-apply wait: %w", i, err)
		}
		elapsed := time.Since(t)
		durs = append(durs, elapsed)
		fmt.Printf("  [%d/%d] reconcile completed in %s\n", i+1, cfg.Count, elapsed.Round(time.Millisecond))
	}
	return durs, nil
}

// benchBulk pushes N manifests concurrently and measures total time until all are applied.
func benchBulk(ctx context.Context, gc *gitea.Client, kc *k8s.Client, cfg Config) (time.Duration, error) {
	t := time.Now()
	cmNames := make([]string, cfg.BulkSize)
	for i := 0; i < cfg.BulkSize; i++ {
		cmNames[i] = fmt.Sprintf("bench-bulk-%d", i)
		filePath := fmt.Sprintf("manifests/cm-bench-bulk-%d.yaml", i)
		if _, err := gc.PutFile(ctx, cfg.GiteaRepo, filePath,
			"bench: bulk "+cmNames[i], []byte(configMapYAML(cmNames[i], cfg.Namespace)), ""); err != nil {
			return 0, fmt.Errorf("bulk push %d: %w", i, err)
		}
	}
	for _, name := range cmNames {
		if err := kc.WaitForCM(ctx, cfg.Namespace, name, cfg.SyncTimeout); err != nil {
			return 0, fmt.Errorf("bulk wait %s: %w", name, err)
		}
	}
	return time.Since(t), nil
}

func configMapYAML(name, namespace string) string {
	return fmt.Sprintf(`apiVersion: v1
kind: ConfigMap
metadata:
  name: %s
  namespace: %s
data:
  bench: "1"
`, name, namespace)
}

func sumDurations(durs []time.Duration) time.Duration {
	var total time.Duration
	for _, d := range durs {
		total += d
	}
	return total
}
