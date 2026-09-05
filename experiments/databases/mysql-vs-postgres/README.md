# MySQL vs PostgreSQL — OLTP Workloads

Benchmarks **MySQL 8.0** (InnoDB) vs **PostgreSQL 16** on a standard OLTP mix:
bulk insert, point queries, aggregation, and updates, using the shared
`loadgen-db` binary against both engines with equivalent schemas.

## Prerequisites

| Dependency | Version |
|------------|---------|
| Docker + Compose v2 | Docker 24+ |
| Go | 1.23+ |
| jq | any |
| RAM | 4 GB free |
| Disk | 5 GB free (10M rows ≈ few GB per engine) |

## Quick Start

```bash
cd experiments/databases/mysql-vs-postgres

./run.sh --smoke-only                 # 1k rows, ~1 min
./run.sh --clean                      # 10M rows, full run
INSERT_COUNT=1000000 ./run.sh --clean # smaller full run
SKIP_BUILD=true ./run.sh --smoke-only # reuse existing binary
```

Full runs emit `results/mysql.json`, `results/postgres.json`, and
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
| `MYSQL_PASSWORD` / `POSTGRES_PASSWORD` | `benchpass` | DB credentials |

## Stack

`deployments/docker-compose/mysql-vs-postgres/docker-compose.yml` starts:

- `mysql` (8.0, InnoDB buffer pool 512M) + `mysql-exporter`
- `postgres` (16) + `postgres-exporter`
- `prometheus` + `grafana` (anonymous admin, dashboards auto-provisioned)

Both engines get an equivalent schema and index set via their respective
`init.sql`, applied on container startup.

## Operations Benchmarked

`loadgen-db --op all` runs, against each engine in turn:

1. **insert** — bulk insert `INSERT_COUNT` rows, batched, `WORKERS` concurrent workers
2. **query** — point lookups by primary key, `QUERY_ITERATIONS` times
3. **agg** — `GROUP BY` aggregation, `AGG_ITERATIONS` times
4. **update** — single-row updates, `UPDATE_ITERATIONS` times

Each op reports `p50_ms`/`p95_ms`/`p99_ms`, `throughput_ops_sec`, and (for
insert) `storage_bytes` on disk after the load.

## Fairness Notes

- Same row shape and index coverage on both engines (see `init.sql` under
  each `deployments/docker-compose/mysql-vs-postgres/{mysql,postgres}/`).
- Table truncated (`--truncate`) before both the smoke test and the full run
  so neither engine benefits from a pre-warmed table.
- Both containers run with default resource limits from the Compose file —
  not tuned beyond the InnoDB buffer pool size set explicitly for MySQL.

## Observability

- Prometheus: http://localhost:9090
- Grafana: http://localhost:3000 (anonymous Admin)
- `mysql-exporter` / `postgres-exporter` expose engine-level metrics scraped
  by Prometheus during the run.

## Validation & CI

- `scripts/validate-mysql-vs-postgres.sh` — static checks (Compose config,
  `run.sh`, `loadgen-db` build)
- `scripts/smoke-mysql-vs-postgres.sh` — end-to-end `--smoke-only` run
- CI jobs: `validate-mysql-vs-postgres`, `smoke-mysql-vs-postgres`
