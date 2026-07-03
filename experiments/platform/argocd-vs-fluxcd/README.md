# Argo CD vs Flux CD — GitOps Sync Latency Benchmark

Compares **Argo CD** and **Flux CD** as GitOps continuous delivery tools. Both follow the pull-based reconciliation model but differ in architecture, UI approach, and reconciliation internals.

## Test Scenarios

| Operation | What is measured |
|-----------|-----------------|
| `sync-latency` | Time from git push to ConfigMap appearing in k8s (p50/p95/p99) |
| `reconcile` | Time from manual resource deletion to controller re-applying it |
| `bulk` | Total time to push N manifests and wait for all to be applied |

## Architecture

- **kind** cluster (1 control-plane + 1 worker) — isolated k8s environment
- **Gitea** (in Docker) — local Git server, connected to the kind network
- **loadgen-gitops** (Go binary) — pushes manifests via Gitea REST API, watches k8s via client-go
- Tools are benchmarked **sequentially** (ArgoCD then Flux) against their own separate git repos

Both tools are configured with a **10-second reconciliation interval** to give consistent timing. Sync latency therefore measures poll-delay + apply overhead.

## How to Run

```bash
# Smoke test only (fast, ~10 min)
./run.sh --smoke-only

# Full benchmark (~20 min)
./run.sh

# Clean start (remove kind cluster + Gitea volumes)
./run.sh --clean

# Keep kind cluster running after the run
./run.sh --keep-cluster
```

## Dependencies

| Tool | Install |
|------|---------|
| `kind` | `brew install kind` / [kind.sigs.k8s.io](https://kind.sigs.k8s.io) |
| `kubectl` | `brew install kubectl` |
| `flux` | `brew install fluxcd/tap/flux` |
| Docker | running Docker daemon |
| Go 1.22+ | for building the benchmark binary |

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `COUNT` | `20` | Sync-latency and reconcile iterations per tool |
| `BULK_SIZE` | `10` | Number of resources in the bulk test |
| `SMOKE_COUNT` | `3` | Iterations in smoke mode |
| `SMOKE_BULK_SIZE` | `3` | Bulk size in smoke mode |
| `SYNC_TIMEOUT` | `120s` | Per-iteration timeout |
| `GITEA_PASS` | `benchpass123` | Gitea admin password |
| `ARGOCD_VERSION` | `v2.13.3` | Argo CD release to install |
| `FLUX_VERSION` | `v2.4.0` | Flux CD release to install |
| `SKIP_BUILD` | `false` | Skip Go build if binary exists |
| `KEEP_CLUSTER` | `0` | Set to `1` to leave the kind cluster running |

## Results

Results are saved to `results/`:
- `argocd.json` — Argo CD benchmark results
- `flux.json` — Flux CD benchmark results
- `summary.json` — merged comparison (schema: `results-summary/v1`)

## Key Differences

| Aspect | Argo CD | Flux CD |
|--------|---------|---------|
| Architecture | Monolithic server + UI | Modular controllers (source, kustomize, helm, …) |
| Reconciliation | Application CRD | GitRepository + Kustomization CRDs |
| UI | Built-in web UI | CLI-first; optional Weave GitOps UI |
| Multi-tenancy | Projects + RBAC | Namespace-scoped sources |
| Default sync interval | 3 min (configurable) | 1 min (configurable) |
