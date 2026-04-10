# gRPC vs REST

Generated from `experiments/api/grpc-vs-rest/results/summary.json`.

## Metadata

| Field | Value |
|-------|-------|
| Experiment | gRPC vs REST |
| Category | `api` |
| Run Timestamp | `2026-04-04T08:53:52Z` |
| Mode | `full` |
| Run ID | `rest-1775292832` |
| Result Count | 6 |

## Config

| Key | Value |
|-----|-------|
| `count` | `100000` |
| `workers` | `50` |

## Sources

| Name | File |
|------|------|
| `rest` | `results/rest.json` |
| `grpc` | `results/grpc.json` |

## Highlights

- `create-order`: throughput leader `grpc` (47371.58 rps, 50.49x vs `rest`); lowest p95 `grpc` (2.02 ms, 76.87x better than `rest`)
- `echo`: throughput leader `grpc` (3099.89 rps, 3.18x vs `rest`); lowest p95 `grpc` (2.422 ms, 66.76x better than `rest`)
- `get-user`: throughput leader `grpc` (49325.89 rps, 19.99x vs `rest`); lowest p95 `grpc` (1.887 ms, 36.07x better than `rest`)

## Results

| Subject | Operation | Count | p50 ms | p95 ms | p99 ms | Total ms | Throughput | Errors | Storage / Memory |
|---------|-----------|-------|--------|--------|--------|----------|------------|--------|------------------|
| `rest` | `echo` | 100000 | 26.607 | 161.683 | 325.719 | 102073 | 975.99 rps | 377 | - |
| `rest` | `get-user` | 100000 | 10.882 | 68.063 | 148.352 | 40526 | 2467.5 rps | 0 | - |
| `rest` | `create-order` | 100000 | 30.178 | 155.272 | 304.861 | 105508 | 938.15 rps | 1017 | - |
| `grpc` | `echo` | 100000 | 0.883 | 2.422 | 5.9 | 32259 | 3099.89 rps | 0 | - |
| `grpc` | `get-user` | 100000 | 0.89 | 1.887 | 2.817 | 2027 | 49325.89 rps | 0 | - |
| `grpc` | `create-order` | 100000 | 0.882 | 2.02 | 4.236 | 2110 | 47371.58 rps | 0 | - |

