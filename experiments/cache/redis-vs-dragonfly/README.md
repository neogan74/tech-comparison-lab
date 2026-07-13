# Redis vs Dragonfly

Side-by-side benchmark comparing **Redis 7** and **Dragonfly 1.38** on identical
in-memory cache workloads. Both servers use the Redis-compatible RESP protocol,
so the same Go client, values, concurrency, and request sequence drive both sides.

## What It Measures

| Operation | Description | Unit |
|-----------|-------------|------|
| `pipeline-set` | Pipelined writes | commands/sec |
| `get` | Random single-key reads | operations/sec |
| `pipeline-get` | Pipelined random reads | commands/sec |
| `mixed` | 80% GET / 20% SET | commands/sec |

The report includes throughput, p50/p95/p99 latency, server-reported memory after
loading, and key count.

## Fairness Notes

- Redis and Dragonfly each receive a 3 GiB memory limit and use an eviction policy.
- Persistence is disabled for Redis; Dragonfly runs in cache mode.
- Dragonfly uses one proactor thread so this default experiment compares the
  engines under the same single-core execution budget rather than giving its
  multi-threaded architecture extra CPU.
- Both servers are accessed from the host through Docker port forwarding.
- Values are approximately 200-byte JSON documents and keys are deterministic.

## Prerequisites

- Go 1.23+
- Docker and Docker Compose v2
- `jq`
- About 8 GiB of free RAM for the default full run

## Running

```bash
cd experiments/cache/redis-vs-dragonfly

# Fast end-to-end check
./run.sh --smoke-only

# Full benchmark; removes old data first
./run.sh --clean

# Smaller custom run
KEY_COUNT=1000000 ITERATIONS=20000 WORKERS=8 ./run.sh --clean
```

Environment variables: `KEY_COUNT` (default 10,000,000), `ITERATIONS`
(100,000), `PIPE_SIZE` (100), `WORKERS` (16), `SMOKE_COUNT` (1,000),
`SMOKE_ITER` (100), and `SKIP_BUILD=true`.

## Infrastructure

| Service | Host port |
|---------|-----------|
| Redis | 6383 |
| Dragonfly | 6384 |
| Prometheus | 9097 |
| Grafana | 3007 |

The pinned Dragonfly image is
`docker.dragonflydb.io/dragonflydb/dragonfly:v1.38.0`.

## Results

Full runs create:

- `results/redis.json`
- `results/dragonfly.json`
- `results/summary.json`

The merged summary follows [`results-summary/v1`](../../../docs/results-summary-v1.md).
Raw figures are hardware-dependent; compare both servers from the same run rather
than comparing results collected on different machines.
