# Experiment: Istio vs Linkerd — Service Mesh

Benchmarks **Istio** (minimal profile, istiod + Envoy sidecars) vs **Linkerd**
(control plane + linkerd2-proxy sidecars) inside a single `kind` cluster,
across two axes:

- **Control plane / injection** — how fast the mesh injects a sidecar and gets
  a workload Ready, and how much the control plane and per-pod sidecar cost.
- **Data plane** — HTTP request latency and throughput through the mesh,
  compared against a non-meshed baseline.

Both meshes are installed in the same cluster, each owning its own injected
namespace (`bench-istio`, `bench-linkerd`); a third un-injected namespace
(`bench-baseline`) provides the no-sidecar reference for the data-plane numbers.

## Prerequisites

| Dependency | Version |
|------------|---------|
| Docker | 24+ |
| kind | 0.20+ |
| kubectl | 1.28+ |
| istioctl | 1.24.x |
| linkerd CLI | 2.14+ (edge/stable) |
| Go | 1.22+ |
| jq | any |
| RAM | 6 GB free (both control planes) |

Install the mesh CLIs:

```bash
curl -sL https://istio.io/downloadIstio | ISTIO_VERSION=1.24.2 sh -
export PATH="$PWD/istio-1.24.2/bin:$PATH"
curl -sL https://run.linkerd.io/install | sh
export PATH="$HOME/.linkerd2/bin:$PATH"
```

## Quick Start

```bash
cd experiments/mesh/istio-vs-linkerd

./run.sh --smoke-only          # 1 replica / 1 round / 200 requests, fast
./run.sh --clean               # full run (3 replicas / 3 rounds / 5000 requests)
./run.sh --keep-cluster        # leave the kind cluster up for inspection
```

The runner creates the kind cluster, installs both meshes, deploys the echo
workload, and benchmarks each mesh in turn. It emits `results/istio.json`,
`results/linkerd.json`, and the merged `results/summary.json`.

## Benchmarks

| Op | What it measures | How |
|----|------------------|-----|
| `inject:ready-Nr` | injection + sidecar-proxy startup cost | time from Deployment create to all N replicas Ready, ×`ROUNDS`, in an injected namespace |
| `footprint:control-plane-*` | control-plane weight | pod count and summed CPU/memory **requests** in `istio-system` / `linkerd` |
| `footprint:sidecar-*` | per-pod data-plane cost | container count and CPU/memory requests of the injected sidecar on an echo pod |
| `data-plane:meshed:*` | latency/throughput through the mesh | HTTP load against the meshed echo Service (server-side proxy on the path) |
| `data-plane:baseline:*` | no-mesh reference | same load against the un-injected echo Service |

The mesh overhead is `data-plane:meshed` minus `data-plane:baseline`.

## Fairness Notes

- **Injection & footprint use client-go only** — no metrics-server is required.
  Footprint reports configured resource **requests**, not live usage; it
  reflects what each mesh reserves per pod and for its control plane, which is
  the dimension Linkerd optimizes for. Live RSS would need metrics-server and
  is out of scope here.
- **Data-plane path is inbound-only.** Requests reach the meshed Service via a
  `kubectl port-forward`, so only the **server-side** sidecar is on the path
  (the client isn't meshed). This isolates inbound proxy overhead and keeps the
  two meshes strictly comparable; it is not a full mesh-to-mesh (both-proxies)
  latency figure.
- **Both meshes share one cluster** on default install profiles (Istio
  `minimal` = istiod only, no ingress gateway; Linkerd default control plane).
  They are benchmarked sequentially so control planes don't contend for CPU
  during measurement.
- Same echo workload (`nginx:1.27-alpine`), same request count/concurrency on
  both sides.

## Customization

| Variable | Default | Description |
|----------|---------|--------------|
| `REPLICAS` | 3 | echo replicas for the inject benchmark |
| `ROUNDS` | 3 | inject benchmark rounds |
| `COUNT` | 5000 | data-plane requests |
| `WORKERS` | 25 | data-plane concurrency |
| `SMOKE_REPLICAS` | 1 | replicas in smoke mode |
| `SMOKE_ROUNDS` | 1 | rounds in smoke mode |
| `SMOKE_COUNT` | 200 | requests in smoke mode |
| `SMOKE_WORKERS` | 5 | concurrency in smoke mode |
| `ISTIO_VERSION` | 1.24.2 | Istio release installed by `istioctl` |
| `SKIP_BUILD` | false | reuse an existing `loadgen-mesh` binary |

## Sample Results (kind, MacBook Pro M2 Pro — shape, not absolute)

```
Mesh     Operation                          p50(ms)    p95(ms)    p99(ms)        Value
-------- --------------------------------  --------   --------   --------  ------------
istio    inject:ready-3r                    8200.0    9100.0     9100.0             -
linkerd  inject:ready-3r                    5400.0    6100.0     6100.0             -
istio    footprint:control-plane-pods           -          -          -      1 pods
linkerd  footprint:control-plane-pods           -          -          -      3 pods
istio    footprint:sidecar-mem-req              -          -          -    128 MiB
linkerd  footprint:sidecar-mem-req              -          -          -     20 MiB
istio    data-plane:meshed:latency             2.1       4.8        7.0            -
istio    data-plane:baseline:latency           1.4       3.0        4.5            -
linkerd  data-plane:meshed:latency             1.8       3.9        5.6            -
linkerd  data-plane:baseline:latency           1.4       3.0        4.5            -
```

**Key takeaway**: Linkerd's purpose-built Rust micro-proxy typically shows a
lower per-pod memory reservation and faster sidecar startup, while Istio's
Envoy is heavier but far more feature-rich (rich traffic policy, extensibility,
multi-cluster). Both add single-digit-millisecond inbound latency at this
scale. Read these as characterizing sidecar/control-plane cost, not a verdict
on either mesh's overall capabilities.

## Troubleshooting

**`istioctl`/`linkerd` not found** — Install the CLIs (see Prerequisites) and
ensure they're on `PATH` before running.

**Linkerd control plane not Ready** — `linkerd check` reports what's missing.
On kind this is usually a transient image pull; re-run with `--clean`.

**`port-forward` connection refused** — The echo Service wasn't Ready before
the forward opened. The loadgen probes for up to 30s; if it still fails, check
`kubectl -n bench-istio get pods` for a stuck sidecar.

**Out of memory / pods Pending** — Running both control planes needs ~6 GB free
for the Docker VM. Lower it by benchmarking one mesh at a time, or raise the
Docker memory limit.
