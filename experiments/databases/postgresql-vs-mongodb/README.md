# Experiment #1: PostgreSQL vs MongoDB — JSON Workload

Side-by-side benchmark comparing **PostgreSQL 16 (JSONB + GIN index)** vs
**MongoDB 7** on JSON document workloads.

## Prerequisites

| Dependency | Minimum version |
|------------|----------------|
| Docker + Compose v2 | Docker 24+ |
| Go | 1.23+ |
| jq | any |
| RAM | 8 GB free |
| Disk | 20 GB free for 10M docs |

## Quick Start

```bash
# 1. Enter experiment directory
cd experiments/databases/postgresql-vs-mongodb

# 2. Recommended first run: clean volumes, then execute the full benchmark
./run.sh --clean

# 3. Or smoke test only (1k docs, ~30 sec)
./run.sh --smoke-only
```

Results are printed to stdout and saved under `results/`:

- `results/postgres.json`
- `results/mongo.json`
- `results/summary.json`

## What Is Benchmarked

| Operation | Description | Iterations |
|-----------|-------------|------------|
| **insert** | Batch insert of N JSON documents (batch=1000, workers=8) | N docs total |
| **query** | `WHERE user.country = 'US' LIMIT 100` (uses expression index) | 1000 iters |
| **agg** | `GROUP BY user.id, SUM(quantity) ORDER BY total DESC LIMIT 100` | 10 iters |
| **update** | Set `metadata.session` for 1000 docs matching `user.country = 'US'` | 100 iters |

## Document Schema (~500 bytes)

```json
{
  "id": "550e8400-...",
  "user": { "id": "...", "country": "US", "tier": "premium" },
  "product": { "id": "...", "category": "electronics", "price": 299.99 },
  "quantity": 3,
  "status": "shipped",
  "tags": ["flash-sale", "gift"],
  "metadata": { "source": "web", "session": "abc123" },
  "created_at": "2024-01-15T10:30:00Z",
  "updated_at": "2024-01-15T10:30:00Z"
}
```

~40% of documents have `user.country = "US"` (ensures query hits are realistic).

## Index Strategy

**PostgreSQL**
- GIN index on the full `doc` JSONB column
- Expression index on `doc->>'status'`
- Expression index on `doc->'user'->>'country'` ← used by query + update

**MongoDB**
- `{ "user.country": 1 }` ← used by query + update
- `{ "user.id": 1 }`
- `{ "status": 1, "created_at": -1 }`

## Interpreting Results

| Metric | Meaning |
|--------|---------|
| `p50_ms` | Median latency — typical case |
| `p95_ms` | 95th percentile — occasional slow ops |
| `p99_ms` | 99th percentile — worst 1% |
| `ops/s` | Throughput (docs/s for insert, iterations/s for others) |
| `storage_bytes` | Total on-disk size after insert (incl. indexes) |

For **insert**: latency is per batch (1000 docs), throughput is docs/sec.
For **query/agg/update**: latency is per iteration, throughput is iterations/sec.

## Customization

Override via environment variables before running:

```bash
INSERT_COUNT=1000000 WORKERS=4 ./run.sh    # 1M docs, 4 workers
SMOKE_ONLY=true ./run.sh                   # equivalent to --smoke-only
SKIP_BUILD=true ./run.sh --smoke-only      # use existing binary without rebuilding
```

| Variable | Default | Description |
|----------|---------|-------------|
| `INSERT_COUNT` | 10000000 | Total documents to insert |
| `WORKERS` | 8 | Concurrent insert workers |
| `BATCH_SIZE` | 1000 | Docs per batch |
| `QUERY_ITERATIONS` | 1000 | Query benchmark iterations |
| `AGG_ITERATIONS` | 10 | Aggregation benchmark iterations |
| `UPDATE_ITERATIONS` | 100 | Update benchmark iterations |
| `POSTGRES_PASSWORD` | benchpass | PG password |
| `MONGO_PASSWORD` | benchpass | Mongo password |
| `SMOKE_ONLY` | false | Run only the 1k-document smoke test |
| `SKIP_BUILD` | false | Skip `go build` and use existing binary |

If `go` is unavailable but `benchmarks/loadgen-db/bin/loadgen-db` already
exists, `run.sh` automatically falls back to that binary.

## Infrastructure

The stack runs via Docker Compose (`deployments/docker-compose/`):

| Service | Port | Description |
|---------|------|-------------|
| postgres | 5432 | PostgreSQL 16 |
| mongo | 27017 | MongoDB 7 |
| prometheus | 9090 | Metrics collection |
| grafana | 3000 | Visualization (user: admin / admin) |
| postgres-exporter | internal | PG metrics for Prometheus |
| mongodb-exporter | internal | Mongo metrics for Prometheus |

## Troubleshooting

**`docker compose` not found**
Ensure Docker Desktop is running and uses Compose v2 (not `docker-compose`).

**Port conflicts**
Copy `.env.example` to `.env` in `deployments/docker-compose/` and override ports.

**`mongosh` not found in health check**
You're on `mongo:7`. If health check fails, the image may lack `mongosh` — try
pulling the latest: `docker pull mongo:7`.

**First build fails downloading modules**
The first `go build` may need internet access to download Go modules. If the
binary already exists, run with `SKIP_BUILD=true`.

**Results directory permission error**
`mkdir -p experiments/databases/postgresql-vs-mongodb/results`

**10M insert taking >60 min**
Normal on a laptop. Reduce with `INSERT_COUNT=1000000 ./run.sh`.

## Sample Results

> From a MacBook Pro M2 Pro, 16GB RAM, Docker Desktop 4.x

```
DB         Operation      p50(ms)    p95(ms)    p99(ms)       ops/s      Storage
---------- ------------ -------- -------- -------- ---------- ----------
postgres   insert           2.31       4.87       9.12       4312     4.8GB
mongo      insert           1.94       3.71       7.23       5104     3.1GB
postgres   query            3.42       6.81      11.34        290         -
mongo      query            2.87       5.12       8.64        348         -
postgres   agg            823.00    1102.00    1450.00          1         -
mongo      agg            512.00     734.00     991.00          2         -
postgres   update           5.21       9.43      16.78        190         -
mongo      update           4.87       8.91      15.23        205         -
```

The sample table above only shows the expected shape of the output.
Actual results will vary; run on your hardware for authoritative numbers.
