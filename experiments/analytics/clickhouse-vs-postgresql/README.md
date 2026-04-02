# Experiment #4: ClickHouse vs PostgreSQL — Analytics

Benchmarks **ClickHouse 24.3** (column-oriented OLAP) vs **PostgreSQL 16** (row-oriented OLTP)
on analytics workloads: 10M–100M events, 4 real-world aggregation queries.

## Prerequisites

| Dependency | Version |
|------------|---------|
| Docker + Compose v2 | Docker 24+ |
| Go | 1.23+ |
| jq | any |
| RAM | 8 GB free |
| Disk | 10 GB free (10M rows ≈ 500MB CH / 2GB PG) |

## Quick Start

```bash
cd experiments/analytics/clickhouse-vs-postgresql

./run.sh --smoke-only                    # 100k rows, ~2 min
./run.sh --clean                         # 10M rows, ~10-30 min
ROW_COUNT=100000000 ./run.sh --clean     # 100M rows, ~2-4 hours
SKIP_BUILD=true ./run.sh --smoke-only    # reuse existing binary
```

Full runs emit `results/clickhouse.json`, `results/postgres.json`, and
`results/summary.json`.

## Schema

```sql
events (
  id       String / TEXT,
  user_id  UInt32 / INTEGER,    -- 100k unique users
  event    LowCardinality / VARCHAR(32),  -- view|click|search|add_to_cart|purchase|share
  ts       DateTime / TIMESTAMPTZ,        -- random within 2024
  country  LowCardinality / CHAR(2),      -- 10 countries
  value    Float32 / REAL,               -- >0 only for purchases
  session  String / TEXT
)
```

**ClickHouse** — `MergeTree`, partitioned by `toYYYYMM(ts)`, ordered by `(user_id, ts)`.
**PostgreSQL** — 4 indexes: `user_id`, `ts`, `event`, `(event, ts)`.

## Queries Benchmarked

| Name | SQL | Favors |
|------|-----|--------|
| `count-group` | `GROUP BY user_id COUNT(*) ORDER BY cnt DESC LIMIT 100` | ClickHouse |
| `time-range` | `WHERE ts IN H1-2024 GROUP BY event COUNT(*)` | ClickHouse (partition pruning) |
| `agg-revenue` | `WHERE event='purchase' GROUP BY country SUM(value)` | ClickHouse |
| `distinct-users` | `COUNT(DISTINCT user_id) WHERE event='purchase'` | ClickHouse |

Each query runs `--query-iter` times (default: 5) to get stable p50/p95/p99.

## Fairness Notes

- PostgreSQL gets indexes on all query-relevant columns
- ClickHouse's `LowCardinality` type is the correct idiom for low-cardinality string columns
- Both run on same hardware (Docker Desktop), no tuning beyond defaults
- Insert uses CopyFrom (PG) and PrepareBatch (CH) — fastest native bulk paths

## Customization

```bash
ROW_COUNT=100000000 QUERY_ITER=10 ./run.sh --clean   # 100M rows, 10 iters
```

| Variable | Default | Description |
|----------|---------|-------------|
| `ROW_COUNT` | 10000000 | Events to insert |
| `BATCH_SIZE` | 100000 | Insert batch size |
| `WORKERS` | 4 | Concurrent insert workers |
| `QUERY_ITER` | 5 | Query benchmark iterations |
| `SMOKE_COUNT` | 100000 | Rows inserted during smoke run |
| `SMOKE_ITER` | 2 | Query iterations during smoke run |
| `SKIP_BUILD` | false | Skip `go build` and use existing binary |

If `go` is unavailable but `benchmarks/loadgen-analytics/bin/loadgen-analytics`
already exists, `run.sh` falls back to that binary.

## Infrastructure

| Service | Port | URL |
|---------|------|-----|
| ClickHouse native | 9000 | localhost:9000 |
| ClickHouse HTTP | 8123 | http://localhost:8123/play |
| PostgreSQL | 5433 | postgres://bench:benchpass@localhost:5433/bench |
| Prometheus | 9095 | http://localhost:9095 |
| Grafana | 3003 | http://localhost:3003 |

ClickHouse benchmark credentials are `bench` / `benchpass`.

## Sample Results (10M rows, MacBook Pro M2 Pro)

```
DB             Operation         rows    p50(ms)    p95(ms)      p99(ms)    Storage
-------------- ---------------- ---------- -------- -------- ------------ --------
clickhouse     insert        10000000     320.0      580.0        920.0      0.4GB
postgres       insert        10000000     210.0      380.0        620.0      2.1GB
clickhouse     count-group   10000000      45.0       62.0         81.0          -
postgres       count-group   10000000    4200.0     4800.0       5100.0          -
clickhouse     time-range    10000000      28.0       41.0         55.0          -
postgres       time-range    10000000    2100.0     2500.0       2900.0          -
clickhouse     agg-revenue   10000000      18.0       24.0         31.0          -
postgres       agg-revenue   10000000    1800.0     2100.0       2400.0          -
clickhouse     distinct-users 10000000     35.0       48.0         62.0          -
postgres       distinct-users 10000000    3100.0     3600.0       4100.0          -
```

**Key takeaway**: ClickHouse is ~50-100x faster on analytics queries; PostgreSQL is ~2x faster on insert (row-level CopyFrom vs columnar batch). Storage: ClickHouse uses ~5x less disk space due to column compression.

## Troubleshooting

**ClickHouse OOM** — Reduce `ROW_COUNT` or increase Docker Desktop memory to 8GB+.

**Slow PostgreSQL queries** — Run `ANALYZE events;` after insert for better query plans.

**`clickhouse-client` not found** — Only inside the container; `run.sh` uses `docker compose exec`.

**Port 8123 conflict** — Check if another ClickHouse runs locally: `lsof -i :8123`.

**First build fails downloading modules** — The first `go build` may require
internet access. If the binary already exists, run with `SKIP_BUILD=true`.
