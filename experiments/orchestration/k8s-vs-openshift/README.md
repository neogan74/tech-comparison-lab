# Experiment: Kubernetes vs OpenShift

Side-by-side benchmark of **vanilla Kubernetes** (kind) vs **OpenShift** (CRC / OCP) across four categories:

| Category | What we measure |
|---|---|
| **API latency** | list-namespaces, list-pods, list-nodes — p50/p95/p99 |
| **Control plane overhead** | System namespace count, system pod count, node capacity |
| **Deploy speed** | Create Deployment → all pods Ready (N rounds, percentile stats) |
| **Scale speed** | 1→5→maxReplicas→1 replicas (time per step) |

## Why these metrics?

- **API latency** reveals whether the extra OpenShift API machinery (OAuth proxy, admission webhooks, OLM) adds observable overhead to routine operations.
- **Overhead** is the most striking difference: a vanilla kind cluster has ~3 system namespaces and ~20 system pods; OpenShift Local ships with 50+ `openshift-*` namespaces and 100+ system pods.
- **Deploy/scale speed** shows whether the SCC admission controller and additional validating webhooks add measurable latency to workload operations.

## Requirements

| Tool | Notes |
|---|---|
| `kind` | Creates the vanilla k8s cluster |
| `kubectl` | Required for kubeconfig access |
| `go ≥ 1.22` | Builds the benchmark binary |
| `jq` | Side-by-side table (optional) |
| OpenShift Local (CRC) | Optional — for the OCP side |

## How to run

```bash
# Kubernetes only (kind, smoke test — ~2 min)
SMOKE_ONLY=1 ./run.sh

# Kubernetes only (full — ~15 min with 3 rounds, 20 replicas)
./run.sh

# With OpenShift (CRC must be running: crc start)
OCP_CONTEXT=crc-admin ./run.sh

# Keep kind cluster alive after the run
KEEP_CLUSTER=1 ./run.sh

# Custom scale
COUNT=500 ROUNDS=5 REPLICAS=50 ./run.sh
```

Results are saved to `results/`.

## Expected results

**Overhead** — the most dramatic difference:

| Metric | Kubernetes (kind) | OpenShift (CRC) |
|---|---|---|
| System namespaces | 3–4 | 50+ |
| System pods | 20–30 | 150–200 |

**API latency** — OpenShift typically adds 1–3 ms to list operations due to additional admission and aggregation layers.

**Deploy/scale speed** — comparable on lightly-loaded clusters; OpenShift SCC validation adds a few ms per pod admission but is rarely the bottleneck.

## Stack

| Component | Details |
|---|---|
| k8s cluster | `kind` (1 control-plane + 1 worker) |
| OCP cluster | OpenShift Local (CRC) or any OCP 4.x |
| Benchmark tool | `benchmarks/loadgen-k8s` — Go CLI using `k8s.io/client-go` |
| Workload image | `registry.k8s.io/pause:3.10` (tiny, non-root, already cached) |
