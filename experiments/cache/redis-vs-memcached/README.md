# Experiment: Redis vs Memcached — Cache Benchmark

Side-by-side benchmark comparing **Redis 7** vs **Memcached 1.6** on in-memory cache workloads.

Key protocol differences:
- **Redis** uses RESP3 with native pipeline batching for both reads and writes
- **Memcached** uses the ASCII text protocol with native `GetMulti` for batched reads (no write pipeline)

## Prerequisites

| Dependency | Minimum version |
|------------|----------------|
| Docker + Compose v2 | Docker 24+ |
| Go | 1.23+ |
| jq | any |
| RAM | 2 GB free |

## Quick Start

```bash
cd experiments/cache/redis-vs-memcached

# Smoke test (~30 sec)
./run.sh --smoke-only

# Full benchmark (1M keys, ~5–10 min)
./run.sh --clean
```

Results in `results/summary.json` and printed to stdout.
Full runs also emit `results/redis.json` and `results/memcached.json`.

## Operations Benchmarked

| Operation | Redis | Memcached | Notes |
|-----------|-------|-----------|-------|
| `pipeline-set` | Pipelined SET (N cmds/batch) | Individual SET with workers | Memcached has no write pipeline |
| `get` | Individual GET | Individual GET | Single round-trip each |
| `pipeline-get` | Pipelined GET | `GetMulti` (native) | Memcached multi-get is a single TCP request |
| `mixed` | 80% GET / 20% SET pipeline | 80% GetMulti / 20% SET | Realistic read-heavy workload |

## Environment Overrides

| Variable | Default | Description |
|----------|---------|-------------|
| `KEY_COUNT` | 1000000 | Total keys for set/pipeline-set |
| `ITERATIONS` | 100000 | Iterations for get/pipeline-get/mixed |
| `PIPE_SIZE` | 100 | Commands per pipeline / multi-get batch |
| `WORKERS` | 16 | Concurrent goroutines |
| `SMOKE_COUNT` | 1000 | Keys for smoke test |
| `SMOKE_ITER` | 100 | Iterations for smoke test |

## Ports

| Service | Host Port |
|---------|-----------|
| Redis | 6382 |
| Memcached | 11211 |
| Prometheus | 9092 |
| Grafana | 3002 |

## Result Schema

Results follow `results-summary/v1`. See `results/summary.json` after a run.

## Key Differences to Keep in Mind

- **Memory efficiency**: Memcached is leaner (no persistence, pub/sub, data types)
- **Features**: Redis supports data structures, TTL, pub/sub, Lua scripting; Memcached is pure cache
- **Pipeline-set**: Memcached QPS may appear lower since it lacks write batching — individual SETs are still concurrent via workers
- **Multi-get**: Memcached's `GetMulti` maps to Redis `pipeline-get` for a fair read-throughput comparison