# Argo CD vs Flux CD

Generated from `experiments/platform/argocd-vs-fluxcd/results/summary.json`.

## Metadata

| Field | Value |
|-------|-------|
| Experiment | Argo CD vs Flux CD |
| Category | `platform` |
| Run Timestamp | `2026-07-09T19:06:15Z` |
| Mode | `full` |
| Run ID | `argocd-vs-fluxcd-2026-07-09T19:06:15Z` |
| Result Count | 6 |

## Config

| Key | Value |
|-----|-------|
| `count` | `20` |
| `bulk_size` | `10` |
| `sync_timeout` | `300s` |

## Sources

| Name | File |
|------|------|
| `argocd` | `results/argocd.json` |
| `flux` | `results/flux.json` |

## Highlights

- `bulk`: no throughput metric; no p95 metric
- `reconcile`: no throughput metric; lowest p95 `n/a` (10615.967 ms, 15.28x better than `n/a`)
- `sync-latency`: no throughput metric; lowest p95 `n/a` (10614.418 ms, 1.91x better than `n/a`)

## Results

| Subject | Operation | Count | p50 ms | p95 ms | p99 ms | Total ms | Throughput | Errors | Storage / Memory |
|---------|-----------|-------|--------|--------|--------|----------|------------|--------|------------------|
| `n/a` | `sync-latency` | 20 | 19677.483 | 20223.406 | 20342.745 | 269044 | - | - | - |
| `n/a` | `reconcile` | 20 | 162043.948 | 162237.895 | 162342.493 | 2988233 | - | - | - |
| `n/a` | `bulk` | 10 | - | - | - | 14161 | - | - | - |
| `n/a` | `sync-latency` | 20 | 9904.762 | 10614.418 | 10734.805 | 197066 | - | - | - |
| `n/a` | `reconcile` | 20 | 10103.594 | 10615.967 | 10646.046 | 202594 | - | - | - |
| `n/a` | `bulk` | 10 | - | - | - | 16161 | - | - | - |

