# PostgreSQL vs CockroachDB — Distributed SQL

Benchmarks **PostgreSQL 16** (single-node, row-store) vs **CockroachDB**
(single-node insecure, in-memory store) on the same OLTP mix used across the
other database experiments: bulk insert, point queries, aggregation, and
updates — driven through `loadgen-db`, which talks to CockroachDB over the
PostgreSQL wire protocol (pgx).

## Prerequisites

| Dependency | Version |
|------------|---------|
| Docker + Compose v2 | Docker 24+ |
| Go | 1.23+ |
| jq | any |
| RAM | 4 GB free |
| Disk | 5 GB free |

## Quick Start

```bash
cd experiments/databases/postgres-vs-cockroachdb

./run.sh --smoke-only                 # 1k rows, ~1 min
./run.sh --clean                      # 10M rows, full run
INSERT_COUNT=1000000 ./run.sh --clean # smaller full run
SKIP_BUILD=true ./run.sh --smoke-only # reuse existing binary
```

Full runs emit `results/postgres.json`, `results/cockroachdb.json`, and
`results/summary.json` (schema: [`results-summary/v1`](/docs/results-summary-v1.md)).

## Environment Overrides

| Variable | Default | Purpose |
|----------|---------|---------|
| `INSERT_COUNT` | `10000000` | Rows inserted per engine |
| `QUERY_ITERATIONS` | `1000` | Point-query iterations |
| `AGG_ITERATIONS` | `10` | Aggregation-query iterations |
| `UPDATE_ITERATIONS` | `100` | Update iterations |
| `WORKERS` | `8` | Concurrent load-gen workers |
| `BATCH_SIZE` | `1000` | Insert batch size |
| `POSTGRES_PASSWORD` | `benchpass` | PostgreSQL credentials |

## Stack

`deployments/docker-compose/postgres-vs-cockroachdb/docker-compose.yml` starts:

- `postgres` (16) + `postgres-exporter`
- `cockroachdb` — `start-single-node --insecure`, in-memory store (1GiB),
  SQL port exposed on `26258` (the cluster `listen-addr` on `26257` stays
  internal), admin UI on `8080`
- `prometheus` + `grafana` (anonymous admin, dashboards auto-provisioned)

CockroachDB has no built-in Prometheus exporter wired up here (it exposes
its own `/_status/vars` endpoint); only PostgreSQL is scraped via
`postgres-exporter`.

## Operations Benchmarked

`loadgen-db --op all` runs, against each engine in turn:

1. **insert** — bulk insert `INSERT_COUNT` rows, batched, `WORKERS` concurrent workers
2. **query** — point lookups by primary key, `QUERY_ITERATIONS` times
3. **agg** — `GROUP BY` aggregation, `AGG_ITERATIONS` times
4. **update** — single-row updates, `UPDATE_ITERATIONS` times

Each op reports `p50_ms`/`p95_ms`/`p99_ms`, `throughput_ops_sec`, and (for
insert) `storage_bytes` on disk after the load.

## Fairness Notes

- Same row shape and index coverage on both engines — CockroachDB schema is
  applied via `cockroach sql --file /cockroach/init.sql` after the SQL
  interface is confirmed ready.
- Single-node CockroachDB is not a distributed cluster; this experiment
  measures single-node SQL-layer overhead, not CockroachDB's distributed
  consensus/replication story. A multi-node variant is a candidate follow-up.
- CockroachDB uses an in-memory store (`--store=type=mem,size=1GiB`) for CI
  speed, so its storage-bytes figures aren't directly comparable to
  PostgreSQL's on-disk figures — read `storage_bytes` for PostgreSQL only.
- Table truncated (`--truncate`) before both the smoke test and the full run.

## Observability

- CockroachDB Admin UI: http://localhost:8080
- Prometheus: http://localhost:9090
- Grafana: http://localhost:3000 (anonymous Admin)

## Validation & CI

- `scripts/validate-postgres-vs-cockroachdb.sh` — static checks (Compose
  config, `run.sh`, `loadgen-db` build)
- `scripts/smoke-postgres-vs-cockroachdb.sh` — end-to-end `--smoke-only` run
- CI jobs: `validate-postgres-vs-cockroachdb`, `smoke-postgres-vs-cockroachdb`
