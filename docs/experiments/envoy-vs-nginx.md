# Envoy vs NGINX

Generated from `experiments/api/envoy-vs-nginx/results/summary.json`.

## Metadata

| Field | Value |
|-------|-------|
| Experiment | Envoy vs NGINX |
| Category | `api` |
| Run Timestamp | `2026-09-05T07:20:25Z` |
| Mode | `full` |
| Run ID | `rest-1788592825` |
| Result Count | 6 |

## Config

| Key | Value |
|-----|-------|
| `count` | `100000` |
| `workers` | `50` |

## Sources

| Name | File |
|------|------|
| `nginx` | `results/nginx.json` |
| `envoy` | `results/envoy.json` |

## Highlights

- `create-order`: throughput leader `rest` (954.33 rps, 1.16x vs `rest`); lowest p95 `rest` (74.66 ms, 2.57x better than `rest`)
- `echo`: throughput leader `rest` (855.5 rps, 1.19x vs `rest`); lowest p95 `rest` (79.156 ms, 1.24x better than `rest`)
- `get-user`: throughput leader `rest` (24883.39 rps, 1.21x vs `rest`); lowest p95 `rest` (3.439 ms, 1.1x better than `rest`)

## Results

| Subject | Operation | Count | p50 ms | p95 ms | p99 ms | Total ms | Throughput | Errors | Storage / Memory |
|---------|-----------|-------|--------|--------|--------|----------|------------|--------|------------------|
| `rest` | `echo` | 100000 | 18.298 | 79.156 | 973.464 | 93116 | 855.5 rps | 20339 | - |
| `rest` | `get-user` | 100000 | 2.044 | 3.798 | 6.778 | 4881 | 20487.3 rps | 0 | - |
| `rest` | `create-order` | 100000 | 14.258 | 74.66 | 1171.543 | 95600 | 954.33 rps | 8765 | - |
| `rest` | `echo` | 100000 | 21.109 | 98.051 | 1131.666 | 113881 | 720.23 rps | 17979 | - |
| `rest` | `get-user` | 100000 | 1.731 | 3.439 | 6.962 | 4018 | 24883.39 rps | 0 | - |
| `rest` | `create-order` | 100000 | 14.575 | 191.574 | 1256.912 | 118682 | 821.34 rps | 2521 | - |

