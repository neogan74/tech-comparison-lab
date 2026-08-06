# Experiment: Jaeger vs Zipkin — Distributed Tracing

Benchmarks **Jaeger 1.62** (all-in-one) vs **Zipkin 3.4** (single-node) on a
synthetic tracing workload: bulk span ingest via the Zipkin v2 collector API,
then trace-query latency, on the same dataset.

Both backends accept the same **Zipkin v2 JSON** on `POST /api/v2/spans` —
Zipkin natively, Jaeger through its Zipkin-compatible collector
(`COLLECTOR_ZIPKIN_HOST_PORT`) — so the benchmark tool ingests through a single
encoder. Only the query API paths differ, which the tool abstracts per backend.

## Prerequisites

| Dependency | Version |
|------------|---------|
| Docker + Compose v2 | Docker 24+ |
| Go | 1.23+ |
| jq, curl | any |
| RAM | 2 GB free |
| Disk | 1 GB free |

## Quick Start

```bash
cd experiments/observability/jaeger-vs-zipkin

./run.sh --smoke-only                 # 5k spans / 10 services, ~1 min
./run.sh --clean                      # 200k spans / 50 services, ~2-5 min
COUNT=1000000 ./run.sh --clean        # 1M spans, ~10-20 min
SKIP_BUILD=true ./run.sh --smoke-only # reuse existing binary
```

Full runs emit `results/jaeger.json`, `results/zipkin.json`, and
`results/summary.json`. The stack is restarted with fresh volumes between the
smoke test and the full run so each backend starts from an empty store.

## Workload

- **Dataset**: `COUNT` total spans (default 200,000), grouped into fixed-shape
  traces of 5 spans each (one `SERVER` root + four `CLIENT` children), spread
  across `SERVICES` distinct services (default 50).
- **Timestamps**: every trace is anchored to a recent wall-clock time (within
  the last 60s) so it falls inside both backends' default query lookback window.
- **Ingest**: spans are posted as Zipkin v2 JSON to `POST /api/v2/spans` in
  batches of `BATCH_SIZE` spans, across `WORKERS` concurrent goroutines. Spans
  are generated per batch (not pre-materialized) to keep memory bounded on
  large runs.
- Both backends run with **in-memory storage** — the fair, apples-to-apples
  default that isolates ingest/query engine cost from disk-backend choice.

## Queries Benchmarked

| Name | Jaeger endpoint | Zipkin endpoint | Tests |
|------|-----------------|-----------------|-------|
| `list-services` | `GET /api/services` | `GET /api/v2/services` | service discovery |
| `find-traces` | `GET /api/traces?service=…&limit=20` | `GET /api/v2/traces?serviceName=…&limit=20` | recent-traces-by-service search |
| `find-operations` | `GET /api/services/{svc}/operations` | `GET /api/v2/spans?serviceName=…` | per-service operation listing |
| `find-trace` | `GET /api/traces/{id}` | `GET /api/v2/trace/{id}` | single-trace point lookup |

Each query runs `--query-iter` times (default: 20) to get stable p50/p95/p99.
`find-trace` first samples a real trace id from the backend so the point lookup
uses an id in the backend's exact stored form.

## Fairness Notes

- Both backends run as single-node instances with in-memory storage and default
  tuning — the only non-default setting is `COLLECTOR_ZIPKIN_HOST_PORT` on
  Jaeger, required for it to accept Zipkin-format spans.
- The same synthetic dataset (identical traces/spans, seeded RNG) is pushed to
  both backends through the same ingest encoder.
- A short indexing pause follows ingest before queries run, so both backends
  have settled before their read path is measured.
- Storage footprint is **not** compared: both use volatile in-memory stores, so
  there is no meaningful on-disk size to read.

## Customization

```bash
COUNT=1000000 SERVICES=100 ./run.sh --clean   # 1M spans, 100 services
```

| Variable | Default | Description |
|----------|---------|--------------|
| `COUNT` | 200000 | Total spans to write |
| `SERVICES` | 50 | Distinct synthetic services |
| `BATCH_SIZE` | 500 | Spans per ingest request |
| `WORKERS` | 4 | Concurrent ingest workers |
| `QUERY_ITER` | 20 | Query benchmark iterations |
| `SMOKE_COUNT` | 5000 | Spans written during smoke run |
| `SMOKE_SERVICES` | 10 | Services written during smoke run |
| `SMOKE_ITER` | 3 | Query iterations during smoke run |
| `SKIP_BUILD` | false | Skip `go build` and use existing binary |

If `go` is unavailable but `benchmarks/loadgen-tracing/bin/loadgen-tracing`
already exists, `run.sh` falls back to that binary.

## Infrastructure

| Service | Port | URL |
|---------|------|-----|
| Jaeger query API + UI | 16686 | http://localhost:16686 |
| Jaeger Zipkin collector (ingest) | 9411 | http://localhost:9411 |
| Zipkin (ingest + query + UI) | 9412 | http://localhost:9412/zipkin |

Jaeger separates its ingest port (9411, Zipkin collector) from its query port
(16686); Zipkin serves ingest and query on a single port (mapped to host 9412
to avoid colliding with Jaeger's collector).

## Troubleshooting

**`connect failed` on startup** — The backend wasn't ready yet. `run.sh` waits
up to 120s on `GET /api/services` (Jaeger) and `GET /health` (Zipkin); check
`docker compose logs jaeger` / `zipkin` if it times out.

**`find-trace` returns few results** — Traces age out of the default lookback
window. This benchmark anchors every trace to the last 60s, so run the queries
promptly after ingest (the runner does this automatically).

**Port conflicts** — Check `lsof -i :16686`, `:9411`, `:9412` if the stack
fails to start.
