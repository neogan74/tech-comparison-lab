# Experiment: Loki vs Elasticsearch — Log Storage

Benchmarks **Loki 3.2** (single binary, filesystem storage) vs
**Elasticsearch 8.15** (single node) on a synthetic logging workload: bulk log
ingest, then log-query latency, on the same dataset.

Loki and Elasticsearch share no wire format — Loki takes label-keyed streams
of `(nanosecond, line)` pairs on `POST /loki/api/v1/push`, Elasticsearch takes
NDJSON action/document pairs on `POST /_bulk`. The benchmark tool encodes each
backend's native format from one shared synthetic entry model, and normalizes
the query surface to four operations both backends support.

## Prerequisites

| Dependency | Version |
|------------|---------|
| Docker + Compose v2 | Docker 24+ |
| Go | 1.23+ |
| jq, curl | any |
| RAM | 3 GB free (Elasticsearch heap + Loki) |
| Disk | 2 GB free |

## Quick Start

```bash
cd experiments/observability/loki-vs-elasticsearch

./run.sh --smoke-only                 # 5k entries / 10 services, ~1 min
./run.sh --clean                      # 500k entries / 50 services, ~3-6 min
COUNT=5000000 ./run.sh --clean        # 5M entries, ~15-30 min
SKIP_BUILD=true ./run.sh --smoke-only # reuse existing binary
```

Full runs emit `results/loki.json`, `results/elasticsearch.json`, and
`results/summary.json`. The stack is restarted with fresh volumes between the
smoke test and the full run so each backend starts from an empty store.

## Workload

- **Dataset**: `COUNT` total log entries (default 500,000), spread across
  `SERVICES` distinct synthetic services (default 50) with a mix of log
  levels.
- **Timestamps**: every entry is anchored within the last `WINDOW` seconds
  (default 300s) so it falls inside both backends' query lookback window.
- **Ingest**: entries are pushed in batches of `BATCH_SIZE` entries, across
  `WORKERS` concurrent goroutines. Loki batches are grouped into one stream
  per `(service, level)` label pair; Elasticsearch batches are sent as
  `_bulk` NDJSON. Entries are generated per batch (not pre-materialized) to
  keep memory bounded on large runs.
- Loki runs single-binary with its default filesystem/boltdb-shipper config;
  Elasticsearch runs single-node with security disabled — the fair,
  apples-to-apples default that isolates ingest/query engine cost from
  cluster-topology or auth overhead.

## Queries Benchmarked

| Name | Loki | Elasticsearch | Tests |
|------|------|---------------|-------|
| `label-values` | `GET /loki/api/v1/label/service/values` | `GET logs-bench/_search` (terms agg on `service`) | label/field cardinality discovery |
| `query-range` | `GET /loki/api/v1/query_range` (`{service="…"}`) | `GET logs-bench/_search` (filter by `service`, sorted, paged) | recent-logs-by-service search |
| `filter-match` | `GET /loki/api/v1/query_range` (`|= "token"`) | `GET logs-bench/_search` (`match` on `message`) | full-text line filtering |
| `count-over-time` | `GET /loki/api/v1/query_range` (`count_over_time`) | `GET logs-bench/_search` (date-histogram agg) | log-volume aggregation |

Each query runs `--query-iter` times (default: 20) to get stable p50/p95/p99.

## Fairness Notes

- Both backends run as single-node instances with default tuning — the only
  non-default settings are `xpack.security.enabled=false` on Elasticsearch
  (avoids TLS/auth overhead skewing latency) and a 1 GiB JVM heap cap.
- The same synthetic dataset (identical entries) is pushed to both backends
  through the same ingest encoder, just re-serialized per backend's wire
  format.
- After ingest, the runner flushes/refreshes and polls until the written
  entries are queryable before timing any query, so both backends have
  settled before their read path is measured.
- Storage footprint is **not** compared in this benchmark; both use ephemeral
  container volumes reset between runs.

## Customization

```bash
COUNT=5000000 SERVICES=100 ./run.sh --clean   # 5M entries, 100 services
```

| Variable | Default | Description |
|----------|---------|--------------|
| `COUNT` | 500000 | Total log entries to write |
| `SERVICES` | 50 | Distinct synthetic services |
| `BATCH_SIZE` | 2000 | Entries per ingest request |
| `WORKERS` | 4 | Concurrent ingest workers |
| `QUERY_ITER` | 20 | Query benchmark iterations |
| `WINDOW` | 300 | Seconds entries are spread over / queries look back |
| `LIMIT` | 100 | Page size for line-returning queries |
| `SMOKE_COUNT` | 5000 | Entries written during smoke run |
| `SMOKE_SERVICES` | 10 | Services written during smoke run |
| `SMOKE_ITER` | 3 | Query iterations during smoke run |
| `SKIP_BUILD` | false | Skip `go build` and use existing binary |

If `go` is unavailable but `benchmarks/loadgen-logs/bin/loadgen-logs` already
exists, `run.sh` falls back to that binary.

## Infrastructure

| Service | Port | URL |
|---------|------|-----|
| Loki (push + query) | 3100 | http://localhost:3100 |
| Elasticsearch (bulk + query) | 9200 | http://localhost:9200 |

## Troubleshooting

**`connect failed` on startup** — The backend wasn't ready yet. `run.sh` waits
up to 120s on `GET /ready` (Loki) and `GET /_cluster/health` (Elasticsearch);
check `docker compose logs loki` / `elasticsearch` if it times out.

**Elasticsearch exits immediately / OOM** — Increase Docker's memory limit;
the container needs at least ~2 GiB total (1 GiB JVM heap + overhead).

**`resource_already_exists_exception` on setup** — Harmless; it means a
leftover `logs-bench` index survived from an earlier run with an identical
mapping. Use `--clean` to start from empty volumes.

**Port conflicts** — Check `lsof -i :3100`, `:9200` if the stack fails to
start.
