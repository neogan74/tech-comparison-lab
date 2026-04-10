# PostgreSQL vs MongoDB

Generated from `experiments/databases/postgresql-vs-mongodb/results/summary.json`.

## Metadata

| Field | Value |
|-------|-------|
| Experiment | PostgreSQL vs MongoDB |
| Category | `databases` |
| Run Timestamp | `2026-04-04T05:02:34Z` |
| Mode | `full` |
| Run ID | `postgres-1775278954` |
| Result Count | 8 |

## Config

| Key | Value |
|-----|-------|
| `insert_count` | `10000000` |
| `query_iterations` | `1000` |
| `agg_iterations` | `10` |
| `update_iterations` | `100` |
| `workers` | `8` |
| `batch_size` | `1000` |

## Sources

| Name | File |
|------|------|
| `postgres` | `results/postgres.json` |
| `mongo` | `results/mongo.json` |

## Highlights

- `agg`: throughput leader `postgres` (0.01 ops/s, 1.49x vs `mongo`); lowest p95 `postgres` (125149.798 ms, 1.46x better than `mongo`)
- `insert`: throughput leader `mongo` (8053.32 ops/s, 2.48x vs `postgres`); lowest p95 `mongo` (2795.716 ms, 2.39x better than `postgres`)
- `query`: throughput leader `postgres` (773.03 ops/s, 4.66x vs `mongo`); lowest p95 `postgres` (2.865 ms, 3.96x better than `mongo`)
- `update`: throughput leader `mongo` (22.97 ops/s, 6.86x vs `postgres`); lowest p95 `mongo` (52.232 ms, 12.71x better than `postgres`)

## Results

| Subject | Operation | Count | p50 ms | p95 ms | p99 ms | Total ms | Throughput | Errors | Storage / Memory |
|---------|-----------|-------|--------|--------|--------|----------|------------|--------|------------------|
| `postgres` | `insert` | 10000000 | 1084.142 | 6681.793 | 24110.997 | 3075997 | 3250.98 ops/s | - | 21030715392 |
| `postgres` | `query` | 1000 | 0.955 | 2.865 | 4.646 | 1293 | 773.03 ops/s | - | - |
| `postgres` | `agg` | 10 | 93795.648 | 125149.798 | 125149.798 | 989836 | 0.01 ops/s | - | - |
| `postgres` | `update` | 100 | 204.375 | 664.083 | 2232.382 | 29845 | 3.35 ops/s | - | - |
| `mongo` | `insert` | 10000000 | 608.918 | 2795.716 | 4933.423 | 1241723 | 8053.32 ops/s | - | 2244898816 |
| `mongo` | `query` | 1000 | 5.37 | 11.357 | 18.427 | 6032 | 165.77 ops/s | - | - |
| `mongo` | `agg` | 10 | 136257.82 | 182866.361 | 182866.361 | 1476336 | 0.01 ops/s | - | - |
| `mongo` | `update` | 100 | 27.295 | 52.232 | 76.102 | 4352 | 22.97 ops/s | - | - |

