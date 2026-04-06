# Experiment #16: gRPC vs REST

Side-by-side latency and throughput comparison of **gRPC (HTTP/2)** vs **REST (HTTP/1.1)** using identical JSON payloads and in-process server logic.

## What we measure

| Operation | Description |
|---|---|
| `echo` | Echo a small string — measures raw protocol overhead |
| `get-user` | Fetch a fake user by ID — light read |
| `create-order` | Submit an order — light write with JSON body |

**Metrics**: p50 / p95 / p99 latency (ms), throughput (RPS), error count.

## Design decisions

- **No protobuf / protoc**: the gRPC server uses a custom JSON codec that replaces gRPC's default `proto` codec (`encoding.RegisterCodec`). Both protocols send the exact same JSON bytes — isolating HTTP/2 multiplexing vs HTTP/1.1 keep-alive overhead.
- **Single binary server**: REST (`:8080`) and gRPC (`:50051`) run in the same process with identical in-memory handlers — no DB, no I/O, pure protocol overhead.
- **Connection reuse**: REST uses a tuned `http.Transport` with `MaxIdleConnsPerHost = workers`; gRPC uses one shared `ClientConn`.
- **Live metrics**: the experiment starts Prometheus and Grafana, and `bench-api` exposes Prometheus metrics on `GET /metrics`.

## How to run

```bash
# Quick smoke test (1000 requests per op)
./run.sh --smoke-only

# Full benchmark (default: 100k requests, 50 workers)
./run.sh

# Custom scale
COUNT=500000 WORKERS=100 ./run.sh

# Keep Prometheus + Grafana running after the benchmark
./run.sh --keep-stack

# Reuse prebuilt binaries
SKIP_BUILD=1 ./run.sh --smoke-only
```

Results are saved to `results/`.

`results/rest.json` and `results/grpc.json` are per-protocol `Summary` objects.
`results/summary.json` combines both once the full run completes.

While the run is active you can watch:

- Prometheus: `http://localhost:9096`
- Grafana: `http://localhost:3004`

Grafana auto-provisions the dashboard `gRPC vs REST Overview` under the
`Tech Comparison Lab` folder.

By default the Prometheus/Grafana stack is stopped on exit. Use `--keep-stack`
or `KEEP_STACK=1` if you want to inspect metrics after the benchmark finishes.

## Expected results

gRPC typically wins on throughput with many concurrent workers (HTTP/2 multiplexing avoids head-of-line blocking). REST with keep-alive is competitive at lower concurrency. The `echo` operation shows the clearest protocol gap since there is zero application logic.

## Stack

| Component | Details |
|---|---|
| Server | `apps/bench-api` — Go `net/http` + `google.golang.org/grpc` |
| Load generator | `benchmarks/loadgen-http` — `go-redis`-free, pure Go |
| gRPC codec | JSON (replaces proto) — no `.proto` files needed |
| Metrics | Prometheus scrape of `bench-api` `/metrics` |
| Dashboards | Auto-provisioned Grafana board on `localhost:3004` |
