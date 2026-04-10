# Redis vs Valkey

Generated from `experiments/cache/redis-vs-valkey/results/summary.json`.

## Metadata

| Field | Value |
|-------|-------|
| Experiment | Redis vs Valkey |
| Category | `cache` |
| Run Timestamp | `2026-04-06T12:39:53Z` |
| Mode | `full` |
| Run ID | `redis-1775479193` |
| Result Count | 8 |

## Config

| Key | Value |
|-----|-------|
| `key_count` | `10000000` |
| `iterations` | `100000` |
| `pipe_size` | `100` |
| `workers` | `16` |

## Sources

| Name | File |
|------|------|
| `redis` | `results/redis.json` |
| `valkey` | `results/valkey.json` |

## Highlights

- `get`: throughput leader `valkey` (7897.46 ops/s, 1.22x vs `redis`); lowest p95 `valkey` (4.431 ms, 1.28x better than `redis`)
- `mixed`: throughput leader `valkey` (57377.9 ops/s, 1.47x vs `redis`); lowest p95 `valkey` (55.01 ms, 1.39x better than `redis`)
- `pipeline-get`: throughput leader `valkey` (49648 ops/s, 1.51x vs `redis`); lowest p95 `valkey` (73.669 ms, 1.38x better than `redis`)
- `pipeline-set`: throughput leader `valkey` (146142.18 ops/s, 2.16x vs `redis`); lowest p95 `valkey` (24.157 ms, 2.7x better than `redis`)

## Results

| Subject | Operation | Count | p50 ms | p95 ms | p99 ms | Total ms | Throughput | Errors | Storage / Memory |
|---------|-----------|-------|--------|--------|--------|----------|------------|--------|------------------|
| `redis` | `pipeline-set` | 10000000 | 13.145 | 65.135 | 168.936 | 147951 | 67589.49 ops/s | - | 2.57G |
| `redis` | `get` | 100000 | 1.818 | 5.665 | 12.222 | 15385 | 6499.59 ops/s | - | - |
| `redis` | `pipeline-get` | 10000000 | 39.674 | 101.859 | 169.092 | 304210 | 32871.94 ops/s | - | - |
| `redis` | `mixed` | 10000000 | 35.405 | 76.601 | 126.42 | 255568 | 39128.5 ops/s | - | - |
| `valkey` | `pipeline-set` | 10000000 | 8.242 | 24.157 | 51.718 | 68426 | 146142.18 ops/s | - | 2.30G |
| `valkey` | `get` | 100000 | 1.61 | 4.431 | 7.798 | 12662 | 7897.46 ops/s | - | - |
| `valkey` | `pipeline-get` | 10000000 | 25.289 | 73.669 | 143.48 | 201417 | 49648 ops/s | - | - |
| `valkey` | `mixed` | 10000000 | 23.38 | 55.01 | 96.291 | 174283 | 57377.9 ops/s | - | - |

