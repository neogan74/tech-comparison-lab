# ClickHouse vs PostgreSQL

Generated from `experiments/analytics/clickhouse-vs-postgresql/results/summary.json`.

## Metadata

| Field | Value |
|-------|-------|
| Experiment | ClickHouse vs PostgreSQL |
| Category | `analytics` |
| Run Timestamp | `2026-04-06T13:36:55Z` |
| Mode | `full` |
| Run ID | `clickhouse-1775482615` |
| Result Count | 10 |

## Config

| Key | Value |
|-----|-------|
| `row_count` | `10000000` |
| `batch_size` | `100000` |
| `workers` | `4` |
| `query_iter` | `5` |

## Sources

| Name | File |
|------|------|
| `clickhouse` | `results/clickhouse.json` |
| `postgres` | `results/postgres.json` |

## Highlights

- `agg-revenue`: throughput leader `clickhouse` (4848816.92 rows/s, 5.41x vs `postgres`); lowest p95 `clickhouse` (1133.595 ms, 2.35x better than `postgres`)
- `count-group`: throughput leader `clickhouse` (3477295.24 rows/s, 6.1x vs `postgres`); lowest p95 `clickhouse` (983.316 ms, 14.19x better than `postgres`)
- `distinct-users`: throughput leader `clickhouse` (7981999.79 rows/s, 48.69x vs `postgres`); lowest p95 `clickhouse` (518.045 ms, 49.31x better than `postgres`)
- `insert`: throughput leader `clickhouse` (15160.38 rows/s, 3.04x vs `postgres`); lowest p95 `clickhouse` (87651.02 ms, 2.43x better than `postgres`)
- `time-range`: throughput leader `clickhouse` (14325756.47 rows/s, 18.53x vs `postgres`); lowest p95 `clickhouse` (257.609 ms, 13.29x better than `postgres`)

## Results

| Subject | Operation | Count | p50 ms | p95 ms | p99 ms | Total ms | Throughput | Errors | Storage / Memory |
|---------|-----------|-------|--------|--------|--------|----------|------------|--------|------------------|
| `clickhouse` | `insert` | 10000000 | 10384.228 | 87651.02 | 145855.844 | 659613 | 15160.38 rows/s | - | 818594374 |
| `clickhouse` | `count-group` | 10000000 | 535.929 | 983.316 | 983.316 | 2875 | 3477295.24 rows/s | - | - |
| `clickhouse` | `time-range` | 10000000 | 94.58 | 257.609 | 257.609 | 698 | 14325756.47 rows/s | - | - |
| `clickhouse` | `agg-revenue` | 10000000 | 341.434 | 1133.595 | 1133.595 | 2062 | 4848816.92 rows/s | - | - |
| `clickhouse` | `distinct-users` | 10000000 | 174.597 | 518.045 | 518.045 | 1252 | 7981999.79 rows/s | - | - |
| `postgres` | `insert` | 10000000 | 40635.998 | 212889.002 | 263702.736 | 2008496 | 4978.85 rows/s | - | 2354782208 |
| `postgres` | `count-group` | 10000000 | 859.934 | 13957.193 | 13957.193 | 17536 | 570239.06 rows/s | - | - |
| `postgres` | `time-range` | 10000000 | 2438 | 3422.85 | 3422.85 | 12935 | 773080.08 rows/s | - | - |
| `postgres` | `agg-revenue` | 10000000 | 2372.987 | 2663.452 | 2663.452 | 11147 | 897037.09 rows/s | - | - |
| `postgres` | `distinct-users` | 10000000 | 7144.813 | 25543.689 | 25543.689 | 60997 | 163941.41 rows/s | - | - |

