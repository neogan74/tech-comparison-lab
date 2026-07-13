# Kong vs Traefik

Measures the **proxy overhead** of two popular API gateways — **Kong** (OpenResty/nginx + Lua) and **Traefik** (Go) — placed in front of the *same* backend.

Both gateways forward every request to a single `bench-api` Go process on port 8080, so the only variable is the gateway itself. The load generator exercises three operations — echo, get-user, and create-order — through each gateway and reports throughput and latency percentiles.

## Topology

```
loadgen-http ──▶ Kong    (:8000) ──┐
                                   ├──▶ bench-api (host :8080)
loadgen-http ──▶ Traefik (:8081) ──┘
```

| Gateway | Image        | Config mode           | Proxy port |
|---------|--------------|-----------------------|------------|
| Kong    | `kong:3.9`   | DB-less (declarative) | `:8000`    |
| Traefik | `traefik:v3.3` | file provider       | `:8081`    |

## Operations

| Operation      | Request                       |
|----------------|-------------------------------|
| `echo`         | `GET /echo?msg=hello`         |
| `get-user`     | `GET /users/{id}`             |
| `create-order` | `POST /orders` (JSON body)    |

Each gateway uses a catch-all route (`/`) with the original path preserved, so requests reach the upstream verbatim.

## Fairness Notes

- Identical upstream (`bench-api`) and identical client transport (connection pooling, keep-alives) for both gateways.
- Both gateways run with access logging disabled to isolate raw proxy cost.
- Kong runs in DB-less mode (no Postgres round-trips); Traefik uses the static file provider (no discovery churn). Both are the lightest-weight production-representative configs.
- Gateways reach the host-run backend via `host.docker.internal`, so both share the same host-networking hop.
- The upstream work is deliberately cheap, so measured latency is dominated by gateway processing rather than business logic.

## Prerequisites

- Go 1.23+
- Docker + Docker Compose v2
- `jq`, `curl`

## Running

```bash
# Smoke test only (fast, ~1000 requests per gateway)
./run.sh --smoke-only

# Full benchmark (100k requests, 50 workers)
./run.sh

# Custom scale
COUNT=500000 WORKERS=100 ./run.sh

# Keep the gateway stack running after the run
KEEP_STACK=1 ./run.sh
```

## Results

Per-gateway JSON is written to `results/kong.json` and `results/traefik.json`, and a
merged [`results-summary/v1`](../../../docs/results-summary-v1.md) document is written
to `results/summary.json`. A side-by-side RPS / p50 / p99 table is printed at the end
of a full run.

## Endpoints

- Kong proxy: `http://localhost:8000` (admin API: `http://localhost:8001`)
- Traefik proxy: `http://localhost:8081`
