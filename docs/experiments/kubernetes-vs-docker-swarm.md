# Kubernetes vs Docker Swarm

Generated from `experiments/orchestration/kubernetes-vs-docker-swarm/results/summary.json`.

## Metadata

| Field | Value |
|-------|-------|
| Experiment | Kubernetes vs Docker Swarm |
| Category | `orchestration` |
| Run Timestamp | `2026-07-17T14:12:43.111312Z` |
| Mode | `smoke` |
| Run ID | `kubernetes-vs-docker-swarm-2026-07-17T14:12:43.111312Z` |
| Result Count | 8 |

## Config

| Key | Value |
|-----|-------|
| `rounds` | `1` |
| `replicas` | `3` |

## Sources

| Name | File |
|------|------|
| `kubernetes` | `results/kubernetes.json` |
| `swarm` | `results/swarm.json` |

## Highlights

- `deploy:1-replica`: no throughput metric; lowest p95 `n/a` (625.072 ms, 17.36x better than `n/a`)
- `recover:1-instance`: no throughput metric; lowest p95 `n/a` (643.387 ms, 8.8x better than `n/a`)
- `scale:1-to-3`: no throughput metric; lowest p95 `n/a` (617.303 ms, 2.66x better than `n/a`)
- `scale:3-to-1`: no throughput metric; lowest p95 `n/a` (8.706 ms, 24.03x better than `n/a`)

## Results

| Subject | Operation | Count | p50 ms | p95 ms | p99 ms | Total ms | Throughput | Errors | Storage / Memory |
|---------|-----------|-------|--------|--------|--------|----------|------------|--------|------------------|
| `n/a` | `deploy:1-replica` | 1 | 625.072 | 625.072 | 625.072 | - | - | 0 | - |
| `n/a` | `scale:1-to-3` | 1 | 617.303 | 617.303 | 617.303 | - | - | 0 | - |
| `n/a` | `recover:1-instance` | 1 | 643.387 | 643.387 | 643.387 | - | - | 0 | - |
| `n/a` | `scale:3-to-1` | 1 | 209.168 | 209.168 | 209.168 | - | - | 0 | - |
| `n/a` | `deploy:1-replica` | 1 | 10852.437 | 10852.437 | 10852.437 | - | - | 0 | - |
| `n/a` | `scale:1-to-3` | 1 | 1640.602 | 1640.602 | 1640.602 | - | - | 0 | - |
| `n/a` | `recover:1-instance` | 1 | 5659.908 | 5659.908 | 5659.908 | - | - | 0 | - |
| `n/a` | `scale:3-to-1` | 1 | 8.706 | 8.706 | 8.706 | - | - | 0 | - |

