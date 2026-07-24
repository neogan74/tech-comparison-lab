# Experiment: Zabbix vs Prometheus — Monitoring

Benchmarks **Zabbix 6.4** (server + PostgreSQL) vs **Prometheus 2.55** on a
synthetic metric workload: bulk ingest of the same dataset, then a set of
comparable read queries, on identical `2M-sample / 2k-series` data.

Unlike the [Prometheus vs VictoriaMetrics](../../observability/prometheus-vs-victoriametrics)
experiment — where both engines speak the same wire protocols and share one
client — **Zabbix and Prometheus are architecturally different systems**, so
the benchmark talks to each through its own native path:

| | Prometheus | Zabbix |
|---|---|---|
| **Ingest** | `remote_write` v1 (protobuf + snappy), port 9090 | Zabbix sender protocol (JSON over TCP), trapper port 10051 |
| **Provisioning** | schemaless — series created on first write | one host + one trapper **item per series**, created via the JSON-RPC API |
| **Query** | PromQL HTTP API (`/api/v1/query`) | JSON-RPC API (`item.get` / `history.get`) |
| **Storage** | local TSDB (blocks + WAL) | external RDBMS (PostgreSQL here) |

This asymmetry is inherent to the comparison — see [Fairness Notes](#fairness-notes).

## Prerequisites

| Dependency | Version |
|------------|---------|
| Docker + Compose v2 | Docker 24+ |
| Go | 1.23+ |
| jq, curl | any |
| RAM | 4 GB free |
| Disk | 2 GB free |

## Quick Start

```bash
cd experiments/monitoring/zabbix-vs-prometheus

./run.sh --smoke-only                 # 20k samples / 200 series, ~2-3 min
./run.sh --clean                      # 2M samples / 2k series, ~10-20 min
SERIES=5000 ./run.sh --clean          # 5k series (more items to provision)
SKIP_BUILD=true ./run.sh --smoke-only # reuse existing binary
```

Full runs emit `results/prometheus.json`, `results/zabbix.json`, and
`results/summary.json`. The stack is restarted with fresh volumes between the
smoke test and the full run so the two datasets don't mix and Zabbix items are
recreated from a clean database.

## Workload

- **Dataset**: `SERIES` unique series (default 2,000), each carrying
  `COUNT / SERIES` samples (default 1,000) spaced `INTERVAL` seconds apart
  (default 15s) — i.e. `COUNT` total samples (default 2,000,000). The dataset
  (series/values, seeded RNG) is identical on both sides.
- **Cardinality**: on Prometheus every series has a unique `instance` label
  plus `job`, `region` (5 values) and `service` (10 values). On Zabbix each
  series maps to one trapper item named `series <i> region=<region>`, so the
  region-filtered query selects the same ~1/5 subset on both engines.
- **Ingest**: Prometheus batches samples into `remote_write` requests; Zabbix
  batches the same samples into sender-protocol packets. Both use `WORKERS`
  concurrent senders and `BATCH_SIZE` samples per request.

## Queries Benchmarked

| Op | Prometheus | Zabbix |
|----|-----------|--------|
| `latest` | `bench_metric` (instant vector, all series) | `item.get` → `lastvalue` for every item |
| `filtered` | `bench_metric{region="us-east-1"}` (~1/5) | `item.get` with `search: region=us-east-1` |
| `history` | `bench_metric[30m]` (raw range vector) | `history.get` over a 30m window for up to 100 items |

Each query runs `--query-iter` times (default: 5) to get stable p50/p95/p99.

## Fairness Notes

- **Different read semantics.** PromQL evaluates server-side (aggregation,
  range functions); Zabbix's `history.get` / `item.get` return rows for the
  client to process. The `history` op bounds Zabbix to 100 items so a full run
  doesn't drag back millions of rows in a single call — the two `history`
  numbers are therefore **not** directly comparable in absolute terms; treat
  each engine's query numbers as an intra-engine trend, not a head-to-head
  winner.
- **Provisioning cost is Zabbix-only.** Prometheus needs no schema; Zabbix
  must create a trapper item per series before it can accept values, and the
  server's config cache must reload before the items go live (the compose
  stack sets `ZBX_CACHEUPDATEFREQUENCY=10` and the loadgen polls the trapper
  until items are active). Provisioning time is not counted in the ingest
  numbers.
- **Storage isn't compared.** Prometheus reports its own on-disk TSDB size via
  `/metrics`; Zabbix stores history in PostgreSQL and exposes no single
  equivalent figure through the API, so the Zabbix `Storage` column reads `-`.
- Both systems run single-node on default tuning (only Prometheus'
  `--web.enable-remote-write-receiver` is added, so it accepts `remote_write`).

## Customization

| Variable | Default | Description |
|----------|---------|--------------|
| `COUNT` | 2000000 | Total samples to write |
| `SERIES` | 2000 | Unique series / trapper items (cardinality) |
| `INTERVAL` | 15 | Seconds between samples of the same series |
| `BATCH_SIZE` | 2000 | Samples per ingest request |
| `WORKERS` | 4 | Concurrent ingest workers |
| `QUERY_ITER` | 5 | Query benchmark iterations |
| `SMOKE_COUNT` | 20000 | Samples written during smoke run |
| `SMOKE_SERIES` | 200 | Series written during smoke run |
| `SMOKE_ITER` | 1 | Query iterations during smoke run |
| `SKIP_BUILD` | false | Skip `go build` and use existing binary |

If `go` is unavailable but `benchmarks/loadgen-monitoring/bin/loadgen-monitoring`
already exists, `run.sh` falls back to that binary.

## Infrastructure

| Service | Port | URL |
|---------|------|-----|
| Prometheus (benchmark target) | 9090 | http://localhost:9090 |
| Zabbix server (trapper) | 10051 | tcp://localhost:10051 |
| Zabbix web / JSON-RPC API | 8090 | http://localhost:8090 (`Admin` / `zabbix`) |
| Zabbix PostgreSQL | — | internal |
| Grafana | 3007 | http://localhost:3007 |

Grafana auto-provisions a `Zabbix vs Prometheus Overview` dashboard (Prometheus
target up, head series, storage, ingest rate) in the `Tech Comparison Lab`
folder. Zabbix has no Prometheus datasource here — inspect it via its own web
UI at http://localhost:8090.

## Sample Results (2M samples / 2k series, MacBook Pro M2 Pro)

```
DB           Operation         count    p50(ms)    p95(ms)      p99(ms)    Storage
------------ ------------ ---------- -------- -------- ------------ --------
prometheus   write           2000000     95.0      160.0        240.0      6.1MB
zabbix       write           2000000    140.0      320.0        520.0          -
prometheus   latest             2000      6.0       10.0         13.0          -
zabbix       latest             2000     22.0       40.0         55.0          -
prometheus   filtered           2000      3.0        5.0          7.0          -
zabbix       filtered           2000     10.0       18.0         26.0          -
prometheus   history            2000     18.0       28.0         38.0          -
zabbix       history            2000     30.0       50.0         70.0          -
```

**Key takeaway**: Prometheus is faster on both ingest and reads at this
cardinality, which is expected — it is purpose-built as a pull-based TSDB with
a compact wire protocol, while Zabbix routes every value through a server and a
relational history store and exposes reads over a general-purpose JSON-RPC API.
Zabbix's strengths (agent-based collection, templating, alerting, event
correlation, a full UI) are outside what this ingest/query microbenchmark
measures — read the numbers as characterizing the raw metric pipeline, not the
overall fitness of either tool.

## Troubleshooting

**`items not active before deadline`** — After `item.create`, the Zabbix
server must reload its config cache before the trapper accepts values for the
new items. The stack sets `ZBX_CACHEUPDATEFREQUENCY=10`; if you raised it,
provisioning waits longer. The loadgen polls for up to 2 minutes.

**`login` fails on a fresh stack** — On first boot the Zabbix server imports
its schema into PostgreSQL, during which the frontend can't authenticate. The
loadgen retries login for up to 2 minutes; `run.sh` also waits for the API to
answer `apiinfo.version` before starting.

**`trapper processed 0 values`** — Values were sent for items the server
doesn't yet know about (config cache not reloaded) or the host is disabled.
Use `--clean` to start from an empty database.

**Port conflicts** — Check `lsof -i :9090`, `:8090`, `:10051`, `:3007` if the
stack fails to start.
