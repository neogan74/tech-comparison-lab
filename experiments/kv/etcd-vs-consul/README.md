# etcd vs Consul — KV Store Benchmark

Compares **etcd** and **HashiCorp Consul** as distributed KV stores with consensus guarantees. Both are widely used for service discovery, distributed configuration, and leader election in cloud-native systems.

## Test Scenarios

| Operation | What is measured |
|-----------|-----------------|
| `write` | KV Put throughput and latency (ops/sec, p50/p99) |
| `read` | KV Get throughput and latency |
| `mixed` | 80% read / 20% write workload |
| `watch` | Time from key write to watch/blocking-query notification |
| `election` | Time for leader campaign / lock acquire to succeed |

## How to Run

```bash
# Smoke test only (fast, ~90s)
./run.sh --smoke-only

# Full benchmark (~5 min)
./run.sh

# Clean start (remove volumes)
./run.sh --clean

# Custom parameters
COUNT=50000 WORKERS=16 ./run.sh
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `COUNT` | `10000` | KV operations per benchmark run |
| `WORKERS` | `8` | Concurrent goroutines (write/read/mixed) |
| `WATCH_COUNT` | `100` | Watch notification measurements |
| `ELECTION_COUNT` | `20` | Leader election measurements |
| `SMOKE_COUNT` | `200` | Reduced count for smoke test |
| `SKIP_BUILD` | `false` | Skip Go build if binary exists |

## Results

Results are saved to `results/`:
- `etcd.json` — etcd raw results
- `consul.json` — Consul raw results
- `summary.json` — merged comparison (schema: `results-summary/v1`)

## Architecture

- **etcd** v3.5 — single-node, no auth (dev mode). Client: gRPC v3 API. Election via `concurrency.NewElection`.
- **Consul** v1.18 — single-node `-dev` mode (in-memory). Client: HTTP REST API. Election via Session + KV Lock acquire.

## Key Differences

| Aspect | etcd | Consul |
|--------|------|--------|
| Protocol | gRPC | HTTP REST |
| Watch mechanism | Server-push (true streaming) | Blocking query (long-poll) |
| Election API | Built-in election primitive | Session + KV acquire |
| Primary use case | Kubernetes backend, config | Service mesh, service discovery |
| Consistency model | Linearizable (Raft) | Linearizable (Raft) |

## Ports

| Service | Port |
|---------|------|
| etcd client | 2379 |
| Consul HTTP API | 8500 |
| Consul UI | 8500/ui |
| consul-exporter | 9107 |
| Prometheus | 9095 |
| Grafana | 3005 |
