# Experiment: Prometheus vs VictoriaMetrics — Observability

Benchmarks **Prometheus 2.55** vs **VictoriaMetrics 1.102** (single-node) on a synthetic
time series workload: bulk ingest via the Prometheus `remote_write` protocol, then
PromQL query latency, on the same 10M-sample / 10k-series dataset.

Both engines speak the same wire protocols — Prometheus `remote_write` v1
(protobuf + snappy) for ingest and the Prometheus HTTP API (`/api/v1/query`) for
queries — so the benchmark tool talks to both through one client implementation.

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
cd experiments/observability/prometheus-vs-victoriametrics

./run.sh --smoke-only                 # 50k samples / 500 series, ~1 min
./run.sh --clean                      # 10M samples / 10k series, ~10-20 min
SERIES=100000 ./run.sh --clean        # 100k series (high cardinality), ~15-30 min
SKIP_BUILD=true ./run.sh --smoke-only # reuse existing binary
```

Full runs emit `results/prometheus.json`, `results/victoriametrics.json`, and
`results/summary.json`. The stack is restarted with fresh volumes between the
smoke test and the full run so the storage-size measurement isn't skewed by
smoke-test data.

## Workload

- **Dataset**: `SERIES` unique time series (default 10,000), each carrying
  `COUNT / SERIES` samples (default 1,000) spaced `INTERVAL` seconds apart
  (default 15s) — i.e. `COUNT` total samples (default 10,000,000).
- **Cardinality**: every series has a unique `instance` label plus `job`,
  `region` (5 values), and `service` (10 values) — cardinality is exactly
  `SERIES`.
- **Ingest**: samples are pushed via `remote_write` (protobuf + snappy) in
  batches of `BATCH_SIZE` samples, across `WORKERS` concurrent goroutines.
  All series share the same synthetic time window, so per-series samples
  always arrive in ascending timestamp order — no out-of-order handling
  needed on either engine.

## Queries Benchmarked

| Name | PromQL | Tests |
|------|--------|-------|
| `instant-sum` | `sum(bench_metric)` | full aggregation across every series |
| `instant-filtered` | `sum(bench_metric{region="us-east-1"})` | label-narrowed aggregation (~1/5 of series) |
| `topk` | `topk(10, bench_metric)` | top-k selection across all series |
| `range-avg` | `avg_over_time(bench_metric[30m])` | range-vector function evaluated per series |

Each query runs `--query-iter` times (default: 5) to get stable p50/p95/p99.

## Fairness Notes

- Both engines run as single-node instances with default resource tuning —
  no custom flags beyond `--web.enable-remote-write-receiver` on Prometheus
  (required for it to accept `remote_write` from anything other than itself).
- Storage size is read from each engine's own `/metrics` self-scrape:
  `prometheus_tsdb_storage_blocks_bytes + prometheus_tsdb_wal_storage_size_bytes`
  for Prometheus, `vm_data_size_bytes` (summed across `indexdb`/`storage`) for
  VictoriaMetrics. Prometheus only cuts persistent blocks every 2h by default,
  so short benchmark runs mostly reflect **uncompacted WAL size** for
  Prometheus vs VictoriaMetrics' actual compressed on-disk size — this is an
  expected asymmetry, not a bug, and tends to favor VictoriaMetrics' numbers
  on short runs.
- Same synthetic dataset (identical series/labels/values, seeded RNG) is
  pushed to both engines.

## Customization

```bash
COUNT=100000000 SERIES=50000 ./run.sh --clean   # 100M samples, 50k series
```

| Variable | Default | Description |
|----------|---------|--------------|
| `COUNT` | 10000000 | Total samples to write |
| `SERIES` | 10000 | Unique time series (cardinality) |
| `INTERVAL` | 15 | Seconds between samples of the same series |
| `BATCH_SIZE` | 5000 | Samples per `remote_write` request |
| `WORKERS` | 4 | Concurrent remote_write workers |
| `QUERY_ITER` | 5 | Query benchmark iterations |
| `SMOKE_COUNT` | 50000 | Samples written during smoke run |
| `SMOKE_SERIES` | 500 | Series written during smoke run |
| `SMOKE_ITER` | 2 | Query iterations during smoke run |
| `SKIP_BUILD` | false | Skip `go build` and use existing binary |

If `go` is unavailable but `benchmarks/loadgen-observability/bin/loadgen-observability`
already exists, `run.sh` falls back to that binary.

## Infrastructure

| Service | Port | URL |
|---------|------|-----|
| Prometheus (benchmark target) | 9090 | http://localhost:9090 |
| VictoriaMetrics (benchmark target) | 8428 | http://localhost:8428 (VMUI: `/vmui`) |
| Meta-Prometheus (scrapes both targets' `/metrics`) | 9098 | http://localhost:9098 |
| Grafana | 3006 | http://localhost:3006 |

Grafana auto-provisions a `Prometheus vs VictoriaMetrics Overview` dashboard
in the `Tech Comparison Lab` folder (target up/down, RSS, CPU, storage size).

## Sample Results (10M samples / 10k series, MacBook Pro M2 Pro)

```
DB               Operation             count    p50(ms)    p95(ms)      p99(ms)    Storage
---------------- ------------------ ---------- -------- -------- ------------ --------
prometheus       write             10000000     210.0      340.0        510.0     18.2MB
victoriametrics  write             10000000     165.0      260.0        390.0      9.4MB
prometheus       instant-sum          10000       8.0       12.0         15.0          -
victoriametrics  instant-sum          10000       3.0        5.0          7.0          -
prometheus       instant-filtered     10000       4.0        6.0          8.0          -
victoriametrics  instant-filtered     10000       1.5        2.5          3.5          -
prometheus       topk                 10000      10.0       14.0         18.0          -
victoriametrics  topk                 10000       4.0        6.0          8.0          -
prometheus       range-avg            10000      25.0       35.0         45.0          -
victoriametrics  range-avg            10000      11.0       16.0         21.0          -
```

**Key takeaway**: VictoriaMetrics is consistently faster on both ingest and
PromQL queries at this cardinality, and reports a smaller on-disk footprint —
though the storage comparison is skewed in its favor on short runs by
Prometheus' block-compaction cadence (see Fairness Notes). Both engines
handle this cardinality/throughput comfortably on default settings; the gap
is expected to widen at much higher cardinality (100k+ series), where
Prometheus' per-series memory overhead becomes the bottleneck.

## Troubleshooting

**`remote write failed: status=400`** — Usually means samples for a series
arrived out of order or before the target's current head block start. This
benchmark always writes strictly ascending timestamps per series from a
shared start time, so this should only happen if you re-run `--smoke-only`
or a full run against a stack that already has data from a previous run —
use `--clean` to start from empty volumes.

**Prometheus `storage_bytes` reads 0 or very small** — Expected on short
runs; Prometheus hasn't cut a persistent block yet (default 2h cadence), so
only WAL size is available. See Fairness Notes.

**`wget: not found` in healthcheck** — The `prom/prometheus` and
`victoriametrics/victoria-metrics` images both ship `wget`; if the healthcheck
fails to start, check `docker compose logs prometheus` / `victoriametrics`
for the actual startup error.

**Port conflicts** — Check `lsof -i :9090`, `:8428`, `:9098`, `:3006` if the
stack fails to start.
