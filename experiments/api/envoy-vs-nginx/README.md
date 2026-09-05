# Experiment: Envoy vs NGINX — Reverse Proxy

Benchmarks **Envoy v1.32** vs **NGINX 1.27** as HTTP reverse proxies in front of
the same Go REST backend (`apps/bench-api`). Both proxies forward identical
requests to one upstream, so the measured difference is proxy overhead —
routing, connection handling, and header processing — not backend work.

The same [`loadgen-http`](../../../benchmarks/loadgen-http) client drives both
proxies over plain HTTP/1.1 REST, so the two runs are directly comparable.

This is the L7-proxy counterpart to
[Traefik vs NGINX](../traefik-vs-nginx): NGINX is the shared baseline, and Envoy
brings a different architecture — a fully async C++ dataplane with an xDS
control-plane API, here driven by a static bootstrap.

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
cd experiments/api/envoy-vs-nginx

./run.sh --smoke-only                 # 1000 requests per op, ~1 min
./run.sh                              # 100k requests per op, ~5-10 min
COUNT=500000 WORKERS=100 ./run.sh     # heavier load
SKIP_BUILD=1 ./run.sh --smoke-only    # reuse existing binaries
./run.sh --keep-stack                 # leave proxies running afterward
```

The runner builds `bench-api` and `loadgen-http`, starts the backend locally on
`:8080`, brings up the proxy stack, and benchmarks each proxy in turn. Full runs
emit `results/nginx.json`, `results/envoy.json`, and `results/summary.json`.

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
  (NGINX `proxy_pass`, Envoy static bootstrap) — no caching, TLS, compression,
  retries, or access logging on either side, so the numbers reflect bare
  HTTP/1.1 proxying.
- Upstream keep-alive is enabled on both (`keepalive 32` in NGINX,
  `HttpProtocolOptions` with `explicit_http_config` in Envoy) so neither pays a
  per-request upstream connect.
- Each proxy is benchmarked separately (not simultaneously) so they don't
  contend for CPU during measurement.
- Envoy's worker count follows its default (one per available core), matching
  NGINX's `worker_processes auto`.

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
| NGINX proxy | 8093 | http://localhost:8093 |
| Envoy proxy | 8094 | http://localhost:8094 |
| Envoy admin / metrics | 8095 | http://localhost:8095/stats/prometheus |
| Prometheus | 9099 | http://localhost:9099 |
| Grafana | 3008 | http://localhost:3008 |

Prometheus scrapes both proxies (Envoy's admin `/stats/prometheus` and NGINX via
`nginx-prometheus-exporter`); Grafana auto-provisions a Prometheus datasource.

Ports are distinct from the [Traefik vs NGINX](../traefik-vs-nginx) stack
(`8090`–`8092`, `9098`, `3006`), so the two experiments can coexist — but they
share the single `bench-api` backend on `:8080`, so run them one at a time.

## Troubleshooting

**`bench-api did not start in time`** — Port 8080 is already in use, or the
backend failed to build. Check `lsof -i :8080` and re-run with a freed port.

**A proxy never becomes ready** — The proxy container can't reach the backend
via `host.docker.internal`. On Linux the compose stack adds a `host-gateway`
mapping; confirm `docker compose -f
deployments/docker-compose/edge/docker-compose.yml ps` shows both proxies up.
For Envoy, `curl http://localhost:8095/ready` and
`docker compose -f deployments/docker-compose/edge/docker-compose.yml logs envoy`
show whether the bootstrap parsed.

**Port conflicts** — Check `lsof -i :8093`, `:8094`, `:8095`, `:9099`, `:3008`
if the stack fails to start.

**Non-zero `errors` on `echo` / `create-order` in full runs** — On macOS the
Docker port-forwarding path runs out of ephemeral ports at `WORKERS=50` and
resets connections; both proxies are affected roughly equally (the same is
visible in the committed [Kong vs Traefik](../kong-vs-traefik) results). It
distorts throughput and the tail, so treat full-run numbers from a Mac as
indicative only — run on Linux, or lower `WORKERS`, for clean measurements.
`get-user` stays error-free and is the most trustworthy operation locally.
