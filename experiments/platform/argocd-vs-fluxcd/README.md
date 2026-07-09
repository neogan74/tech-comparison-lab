# Argo CD vs Flux CD — GitOps Sync Latency Benchmark

Compares **Argo CD** and **Flux CD** as GitOps continuous delivery tools. Both follow the pull-based reconciliation model but differ in architecture, UI approach, and reconciliation internals.

## Test Scenarios

| Operation | What is measured |
|-----------|-----------------|
| `sync-latency` | Time from git push to resource ready in k8s (p50/p95/p99) |
| `reconcile` | Time from manual resource deletion to controller re-applying it (self-heal speed) |
| `bulk` | Total time to push N manifests and wait for all to be applied |

## Workloads

Control what gets pushed to git with `--workload`:

| Workload | Resources pushed | Wait condition | Notes |
|----------|-----------------|----------------|-------|
| `configmap` (default) | ConfigMap | CM exists | Fastest; no image pull |
| `deployment` | Deployment (pause image) | 1 available replica | Measures full scheduling pipeline |
| `mixed` | Deployment + Service + ConfigMap | Deployment available | Realistic microservice bundle |

The `deployment` and `mixed` workloads use `registry.k8s.io/pause:3.10` which is already
cached in kind nodes — no image pull delay.

```bash
# Run with Deployment workload
./run.sh --workload=deployment

# Run mixed workload, keep everything running to inspect after
./run.sh --workload=mixed --keep-tools --expose-ui
```

## How to Run

```bash
# Smoke test (fast, ~10 min)
./run.sh --smoke-only

# Full benchmark (~20 min)
./run.sh

# Clean start
./run.sh --clean

# Keep ArgoCD + Flux running after benchmark to inspect state
./run.sh --keep-tools

# Everything at once: deployment workload, live UI, keep running
./run.sh --workload=deployment --expose-ui --keep-tools
```

## Watching the Benchmark Live

Run with `--expose-ui` for visual feedback:

```bash
./run.sh --smoke-only --expose-ui
```

| Tool | UI | URL |
|------|----|-----|
| **Argo CD** | Built-in web UI (port-forward auto) | https://localhost:8080 |
| **Flux CD** | [Capacitor](https://github.com/gimlet-io/capacitor) if installed | http://localhost:3333 |

Install Capacitor (optional, for Flux UI):
```bash
brew install gimlet-io/capacitor/capacitor
```

During the Flux benchmark, `kustomize-controller` and `source-controller` logs are always
streamed to the terminal (prefixed `[flux-log]`) when `--expose-ui` is set.

Watch live status in a separate terminal:
```bash
# ArgoCD
kubectl --context kind-bench-gitops get app -n argocd -w

# Flux
watch -n2 flux get all --context kind-bench-gitops
kubectl --context kind-bench-gitops get deploy,cm -n bench-flux -w
```

## Inspecting State After the Benchmark

With `--keep-tools`, the cluster, Argo CD, and Flux are left running. The script prints
full connection instructions at the end, including the ArgoCD admin password.

Push a manifest manually to trigger a sync:
```bash
# Encode your YAML
CONTENT=$(base64 -i my-app.yaml)

# Push to Gitea
curl -u benchadmin:benchpass123 \
  -X POST http://localhost:3000/api/v1/repos/benchadmin/bench-flux/contents/manifests/my-app.yaml \
  -H 'Content-Type: application/json' \
  -d "{\"message\":\"manual: my-app\",\"content\":\"$CONTENT\",\"branch\":\"main\"}"

# Watch Flux pick it up
flux --context kind-bench-gitops reconcile kustomization bench-bench-flux --with-source
kubectl --context kind-bench-gitops get all -n bench-flux
```

Tear down when done:
```bash
kind delete cluster --name bench-gitops
docker compose -f deployments/docker-compose/gitops/docker-compose.yml down -v
```

## Dependencies

| Tool | Install |
|------|---------|
| `kind` | `brew install kind` |
| `kubectl` | `brew install kubectl` |
| `flux` | `brew install fluxcd/tap/flux` |
| `capacitor` | `brew install gimlet-io/capacitor/capacitor` (optional) |
| Docker | running Docker daemon |
| Go 1.22+ | for building the benchmark binary |

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `COUNT` | `20` | Sync-latency and reconcile iterations per tool |
| `BULK_SIZE` | `10` | Resources in the bulk test |
| `SMOKE_COUNT` | `3` | Iterations in smoke mode |
| `SMOKE_BULK_SIZE` | `3` | Bulk size in smoke mode |
| `SYNC_TIMEOUT` | `120s` | Per-iteration timeout |
| `WORKLOAD` | `configmap` | Resource type: `configmap\|deployment\|mixed` |
| `GITEA_PASS` | `benchpass123` | Gitea admin password |
| `ARGOCD_VERSION` | `v2.13.3` | Argo CD release to install |
| `FLUX_VERSION` | `v2.4.0` | Flux CD release to install |
| `SKIP_BUILD` | `false` | Skip Go build if binary exists |
| `KEEP_CLUSTER` | `0` | Set to `1` to leave the kind cluster running |

## Results

Saved to `results/`:
- `argocd.json` — Argo CD benchmark results
- `flux.json` — Flux CD benchmark results
- `summary.json` — merged comparison (`results-summary/v1` schema)

## Key Differences

| Aspect | Argo CD | Flux CD |
|--------|---------|---------|
| Architecture | Monolithic server + UI | Modular controllers (source, kustomize, helm…) |
| Reconciliation model | Application CRD | GitRepository + Kustomization CRDs |
| UI | Built-in web UI | CLI-first; Capacitor (community) |
| Multi-tenancy | Projects + RBAC | Namespace-scoped sources |
| Default sync interval | 3 min (configurable) | 1 min (configurable) |
| Self-heal | `selfHeal: true` on Application | Always on (Kustomization reconciles every interval) |
