# Kong vs Traefik

Generated from `experiments/api/kong-vs-traefik/results/summary.json`.

## Metadata

| Field | Value |
|-------|-------|
| Experiment | Kong vs Traefik |
| Category | `api` |
| Run Timestamp | `2026-07-13T17:17:59Z` |
| Mode | `full` |
| Run ID | `rest-1783963079` |
| Result Count | 6 |

## Config

| Key | Value |
|-----|-------|
| `count` | `100000` |
| `workers` | `50` |

## Sources

| Name | File |
|------|------|
| `kong` | `results/kong.json` |
| `traefik` | `results/traefik.json` |

## Highlights

- `create-order`: throughput leader `rest` (907.97 rps, 1.1x vs `rest`); lowest p95 `rest` (162.153 ms, 1.08x better than `rest`)
- `echo`: throughput leader `rest` (905.79 rps, 1.01x vs `rest`); lowest p95 `rest` (89.381 ms, 1.54x better than `rest`)
- `get-user`: throughput leader `rest` (15066.22 rps, 2.29x vs `rest`); lowest p95 `rest` (4.352 ms, 6.11x better than `rest`)

## Results

| Subject | Operation | Count | p50 ms | p95 ms | p99 ms | Total ms | Throughput | Errors | Storage / Memory |
|---------|-----------|-------|--------|--------|--------|----------|------------|--------|------------------|
| `rest` | `echo` | 100000 | 17.888 | 89.381 | 803.449 | 98648 | 897.13 rps | 11499 | - |
| `rest` | `get-user` | 100000 | 4.449 | 26.604 | 52.248 | 15187 | 6584.52 rps | 0 | - |
| `rest` | `create-order` | 100000 | 29.45 | 162.153 | 289.656 | 106562 | 907.97 rps | 3244 | - |
| `rest` | `echo` | 100000 | 31.857 | 137.779 | 456.455 | 104419 | 905.79 rps | 5418 | - |
| `rest` | `get-user` | 100000 | 2.364 | 4.352 | 44.957 | 6637 | 15066.22 rps | 0 | - |
| `rest` | `create-order` | 100000 | 33.572 | 175.347 | 555.244 | 120637 | 826.78 rps | 259 | - |

