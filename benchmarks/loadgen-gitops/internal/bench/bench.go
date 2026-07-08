package bench

import (
	"context"
	"fmt"
	"time"

	"github.com/tech-comparison-lab/loadgen-gitops/internal/gitea"
	"github.com/tech-comparison-lab/loadgen-gitops/internal/k8s"
	"github.com/tech-comparison-lab/loadgen-gitops/internal/report"
)

// Config holds all benchmark parameters.
type Config struct {
	Tool        string
	Workload    string // configmap | deployment | mixed
	Count       int
	BulkSize    int
	Namespace   string
	GiteaRepo   string
	SyncTimeout time.Duration
}

// Run executes the requested operations and returns results.
func Run(ctx context.Context, gc *gitea.Client, kc *k8s.Client, op string, cfg Config) ([]report.Result, error) {
	if cfg.Workload == "" {
		cfg.Workload = k8s.WorkloadConfigMap
	}

	if err := setup(ctx, gc, kc, cfg); err != nil {
		return nil, fmt.Errorf("setup: %w", err)
	}

	runSync      := op == "sync-latency" || op == "all"
	runReconcile := op == "reconcile" || op == "all"
	runBulk      := op == "bulk" || op == "all"

	var results []report.Result

	if runSync {
		fmt.Printf("[%s/%s] sync-latency: %d iterations (timeout=%s)\n",
			cfg.Tool, cfg.Workload, cfg.Count, cfg.SyncTimeout)
		durs, err := benchSyncLatency(ctx, gc, kc, cfg)
		if err != nil {
			return results, fmt.Errorf("sync-latency: %w", err)
		}
		results = append(results, report.Compute(cfg.Tool, "sync-latency", cfg.Count, durs, sumDurations(durs)))
	}

	if runReconcile {
		fmt.Printf("[%s/%s] reconcile: %d iterations\n", cfg.Tool, cfg.Workload, cfg.Count)
		durs, err := benchReconcile(ctx, kc, cfg)
		if err != nil {
			return results, fmt.Errorf("reconcile: %w", err)
		}
		results = append(results, report.Compute(cfg.Tool, "reconcile", cfg.Count, durs, sumDurations(durs)))
	}

	if runBulk {
		fmt.Printf("[%s/%s] bulk: %d resources\n", cfg.Tool, cfg.Workload, cfg.BulkSize)
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

// setup ensures the k8s namespace exists and pushes the anchor manifest used
// by the reconcile benchmark.
func setup(ctx context.Context, gc *gitea.Client, kc *k8s.Client, cfg Config) error {
	if err := kc.EnsureNamespace(ctx, cfg.Namespace); err != nil {
		return fmt.Errorf("namespace: %w", err)
	}
	if err := gc.EnsureRepo(ctx, cfg.GiteaRepo); err != nil {
		return fmt.Errorf("repo: %w", err)
	}

	// Push the anchor resource used by the reconcile benchmark.
	const initName = "bench-init"
	files := manifestFiles(initName, cfg.Namespace, cfg.Workload)
	for path, content := range files {
		_, sha, err := gc.GetFile(ctx, cfg.GiteaRepo, path)
		if err != nil {
			return fmt.Errorf("get %s: %w", path, err)
		}
		if _, err := gc.PutFile(ctx, cfg.GiteaRepo, path, "init: "+path, []byte(content), sha); err != nil {
			return fmt.Errorf("push %s: %w", path, err)
		}
	}
	return nil
}

// benchSyncLatency pushes per-iteration manifests and measures time-to-apply.
func benchSyncLatency(ctx context.Context, gc *gitea.Client, kc *k8s.Client, cfg Config) ([]time.Duration, error) {
	var durs []time.Duration
	for i := 0; i < cfg.Count; i++ {
		name := fmt.Sprintf("bench-sync-%d", i)
		t := time.Now()
		if err := pushManifests(ctx, gc, cfg.GiteaRepo, name, cfg.Namespace, cfg.Workload, i); err != nil {
			return durs, fmt.Errorf("iter %d push: %w", i, err)
		}
		if err := waitForReady(ctx, kc, cfg.Namespace, name, cfg.Workload, cfg.SyncTimeout); err != nil {
			return durs, fmt.Errorf("iter %d wait: %w", i, err)
		}
		elapsed := time.Since(t)
		durs = append(durs, elapsed)
		fmt.Printf("  [%d/%d] %s ready in %s\n", i+1, cfg.Count, name, elapsed.Round(time.Millisecond))
	}
	return durs, nil
}

// benchReconcile deletes the anchor resource and measures re-apply time.
func benchReconcile(ctx context.Context, kc *k8s.Client, cfg Config) ([]time.Duration, error) {
	const name = "bench-init"
	var durs []time.Duration
	for i := 0; i < cfg.Count; i++ {
		if err := waitForReady(ctx, kc, cfg.Namespace, name, cfg.Workload, cfg.SyncTimeout); err != nil {
			return durs, fmt.Errorf("iter %d pre-check: %w", i, err)
		}
		if err := deleteResource(ctx, kc, cfg.Namespace, name, cfg.Workload); err != nil {
			return durs, fmt.Errorf("iter %d delete: %w", i, err)
		}
		t := time.Now()
		if err := waitForReady(ctx, kc, cfg.Namespace, name, cfg.Workload, cfg.SyncTimeout); err != nil {
			return durs, fmt.Errorf("iter %d reconcile wait: %w", i, err)
		}
		elapsed := time.Since(t)
		durs = append(durs, elapsed)
		fmt.Printf("  [%d/%d] reconcile in %s\n", i+1, cfg.Count, elapsed.Round(time.Millisecond))
	}
	return durs, nil
}

// benchBulk pushes BulkSize manifests and measures total time until all applied.
func benchBulk(ctx context.Context, gc *gitea.Client, kc *k8s.Client, cfg Config) (time.Duration, error) {
	t := time.Now()
	names := make([]string, cfg.BulkSize)
	for i := 0; i < cfg.BulkSize; i++ {
		names[i] = fmt.Sprintf("bench-bulk-%d", i)
		if err := pushManifests(ctx, gc, cfg.GiteaRepo, names[i], cfg.Namespace, cfg.Workload, i); err != nil {
			return 0, fmt.Errorf("bulk push %d: %w", i, err)
		}
	}
	for _, name := range names {
		if err := waitForReady(ctx, kc, cfg.Namespace, name, cfg.Workload, cfg.SyncTimeout); err != nil {
			return 0, fmt.Errorf("bulk wait %s: %w", name, err)
		}
	}
	return time.Since(t), nil
}

// ---------------------------------------------------------------------------
// Workload helpers
// ---------------------------------------------------------------------------

// manifestFiles returns a map of gitea-path → yaml-content for the given workload.
func manifestFiles(name, namespace, workload string) map[string]string {
	files := make(map[string]string)
	switch workload {
	case k8s.WorkloadDeployment:
		files[fmt.Sprintf("manifests/deploy-%s.yaml", name)] = deploymentYAML(name, namespace)
	case k8s.WorkloadMixed:
		files[fmt.Sprintf("manifests/deploy-%s.yaml", name)] = deploymentYAML(name, namespace)
		files[fmt.Sprintf("manifests/svc-%s.yaml", name)]    = serviceYAML(name, namespace)
		files[fmt.Sprintf("manifests/cm-%s.yaml", name)]     = configMapYAML(name, namespace)
	default: // configmap
		files[fmt.Sprintf("manifests/cm-%s.yaml", name)] = configMapYAML(name, namespace)
	}
	return files
}

// pushManifests creates all files for one benchmark resource in Gitea.
func pushManifests(ctx context.Context, gc *gitea.Client, repo, name, namespace, workload string, idx int) error {
	for path, content := range manifestFiles(name, namespace, workload) {
		if _, err := gc.PutFile(ctx, repo, path, fmt.Sprintf("bench[%d]: %s", idx, name), []byte(content), ""); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	}
	return nil
}

// waitForReady waits for the primary resource of a workload to become ready.
func waitForReady(ctx context.Context, kc *k8s.Client, namespace, name, workload string, timeout time.Duration) error {
	switch workload {
	case k8s.WorkloadDeployment, k8s.WorkloadMixed:
		return kc.WaitForDeploymentReady(ctx, namespace, name, timeout)
	default:
		return kc.WaitForCM(ctx, namespace, name, timeout)
	}
}

// deleteResource removes the primary resource of a workload.
func deleteResource(ctx context.Context, kc *k8s.Client, namespace, name, workload string) error {
	switch workload {
	case k8s.WorkloadDeployment, k8s.WorkloadMixed:
		return kc.DeleteDeployment(ctx, namespace, name)
	default:
		return kc.DeleteCM(ctx, namespace, name)
	}
}

// ---------------------------------------------------------------------------
// YAML generators
// ---------------------------------------------------------------------------

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

// deploymentYAML uses the pause container (already cached in kind nodes, no pull delay).
// creationTimestamp: null is required — without it ArgoCD sees a persistent diff because
// k8s sets this field on spec.template.metadata after apply.
func deploymentYAML(name, namespace string) string {
	return fmt.Sprintf(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: %s
  namespace: %s
spec:
  replicas: 1
  selector:
    matchLabels:
      app: %s
  template:
    metadata:
      creationTimestamp: null
      labels:
        app: %s
    spec:
      containers:
        - name: app
          image: registry.k8s.io/pause:3.10
          imagePullPolicy: IfNotPresent
`, name, namespace, name, name)
}

func serviceYAML(name, namespace string) string {
	return fmt.Sprintf(`apiVersion: v1
kind: Service
metadata:
  name: %s
  namespace: %s
spec:
  selector:
    app: %s
  ports:
    - port: 80
      targetPort: 80
`, name, namespace, name)
}

func sumDurations(durs []time.Duration) time.Duration {
	var total time.Duration
	for _, d := range durs {
		total += d
	}
	return total
}
