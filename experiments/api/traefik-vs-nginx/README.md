# Experiment: Traefik vs NGINX — Reverse Proxy

Benchmarks **Traefik v3.3** vs **NGINX 1.27** as HTTP reverse proxies in front
of the same Go REST backend (`apps/bench-api`). Both proxies forward identical
requests to one upstream, so the measured difference is proxy overhead —
routing, connection handling, and header processing — not backend work.

The same [`loadgen-http`](../../../benchmarks/loadgen-http) client drives both
proxies over plain HTTP/1.1 REST, so the two runs are directly comparable.

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
cd experiments/api/traefik-vs-nginx

./run.sh --smoke-only                 # 1000 requests per op, ~1 min
./run.sh                              # 100k requests per op, ~5-10 min
COUNT=500000 WORKERS=100 ./run.sh     # heavier load
SKIP_BUILD=1 ./run.sh --smoke-only    # reuse existing binaries
./run.sh --keep-stack                 # leave proxies running afterward
```

The runner builds `bench-api` and `loadgen-http`, starts the backend locally on
`:8080`, brings up the proxy stack, and benchmarks each proxy in turn. Full runs
emit `results/nginx.json`, `results/traefik.json`, and `results/summary.json`.

## Workload

- **Backend**: `bench-api` REST server (`:8080`), reached by both proxies via
  `host.docker.internal`.
- **Operations** (`--op all`): `echo`, `get-user`, `create-order` — a mix of a
  trivial passthrough, a GET with a small JSON body, and a POST.
- **Load**: `COUNT` requests per operation (default 100,000) across `WORKERS`
  concurrent connections (default 50), replayed independently against each proxy.

## Metrics

Per operation and proxy: throughput (RPS), and p50 / p95 / p99 latency. The
runner prints a side-by-side RPS + p50/p99 table and writes the merged
[`results-summary/v1`](../../../docs/results-summary-v1.md) JSON.

## Fairness Notes

- Both proxies forward to the **same** backend instance over the same
  `host.docker.internal` gateway, so upstream latency is shared and cancels out
  of the comparison.
- Both run on default tuning with a minimal static route to the backend
  (NGINX `proxy_pass`, Traefik file provider) — no caching, TLS, or compression
  on either side, so the numbers reflect bare HTTP/1.1 proxying.
- Each proxy is benchmarked separately (not simultaneously) so they don't
  contend for CPU during measurement.

## Customization

| Variable | Default | Description |
|----------|---------|--------------|
| `COUNT` | 100000 | Requests per operation (full run) |
| `WORKERS` | 50 | Concurrent connections |
| `SMOKE_COUNT` | 1000 | Requests per operation (smoke run) |
| `SMOKE_ONLY` | 0 | Set to `1` (or pass `--smoke-only`) to skip the full run |
| `SKIP_BUILD` | 0 | Set to `1` to reuse existing binaries |
| `KEEP_STACK` | 0 | Set to `1` (or pass `--keep-stack`) to leave the stack up |

## Infrastructure

| Service | Port | URL |
|---------|------|-----|
| bench-api backend (REST) | 8080 | http://localhost:8080 |
| NGINX proxy | 8090 | http://localhost:8090 |
| Traefik proxy | 8091 | http://localhost:8091 |
| Traefik dashboard / metrics | 8092 | http://localhost:8092/dashboard/ |
| Prometheus | 9098 | http://localhost:9098 |
| Grafana | 3006 | http://localhost:3006 |

Prometheus scrapes both proxies (Traefik's built-in metrics and NGINX via
`nginx-prometheus-exporter`); Grafana auto-provisions a Prometheus datasource.

## Troubleshooting

**`bench-api did not start in time`** — Port 8080 is already in use, or the
backend failed to build. Check `lsof -i :8080` and re-run with a freed port.

**A proxy never becomes ready** — The proxy container can't reach the backend
via `host.docker.internal`. On Linux the compose stack adds a
`host-gateway` mapping; confirm `docker compose -f
deployments/docker-compose/proxy/docker-compose.yml ps` shows both proxies up.

**Port conflicts** — Check `lsof -i :8090`, `:8091`, `:8092`, `:9098`, `:3006`
if the stack fails to start.
