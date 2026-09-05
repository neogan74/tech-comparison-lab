# GraphQL vs REST

Generated from `experiments/api/graphql-vs-rest/results/summary.json`.

## Metadata

| Field | Value |
|-------|-------|
| Experiment | GraphQL vs REST |
| Category | `api` |
| Run Timestamp | `2026-06-30T18:22:40Z` |
| Mode | `full` |
| Run ID | `rest-1782843760` |
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
| `graphql` | `results/graphql.json` |

## Highlights

- `create-order`: throughput leader `graphql` (67735.62 rps, 88.62x vs `rest`); lowest p95 `graphql` (1.515 ms, 149x better than `rest`)
- `echo`: throughput leader `graphql` (19495.09 rps, 25.78x vs `rest`); lowest p95 `graphql` (1.628 ms, 186.91x better than `rest`)
- `get-user`: throughput leader `graphql` (64811.14 rps, 5.05x vs `rest`); lowest p95 `graphql` (1.549 ms, 1.12x better than `rest`)

## Results

| Subject | Operation | Count | p50 ms | p95 ms | p99 ms | Total ms | Throughput | Errors | Storage / Memory |
|---------|-----------|-------|--------|--------|--------|----------|------------|--------|------------------|
| `rest` | `echo` | 100000 | 2.523 | 304.294 | 1527.871 | 132239 | 756.2 rps | 0 | - |
| `rest` | `get-user` | 100000 | 0.657 | 1.734 | 40.322 | 7790 | 12836.07 rps | 0 | - |
| `rest` | `create-order` | 100000 | 2.981 | 225.736 | 1556.757 | 130556 | 764.38 rps | 205 | - |
| `graphql` | `echo` | 100000 | 0.646 | 1.628 | 26.717 | 5129 | 19495.09 rps | 0 | - |
| `graphql` | `get-user` | 100000 | 0.621 | 1.549 | 2.054 | 1542 | 64811.14 rps | 0 | - |
| `graphql` | `create-order` | 100000 | 0.627 | 1.515 | 1.989 | 1476 | 67735.62 rps | 0 | - |

