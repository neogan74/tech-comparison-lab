# Experiment #5: Redis vs Valkey — Cache Benchmark

Side-by-side benchmark comparing **Redis 7** vs **Valkey 8** on in-memory cache workloads.
Both use the RESP3 protocol — the same Go client drives both servers.

## Prerequisites

| Dependency | Minimum version |
|------------|----------------|
| Docker + Compose v2 | Docker 24+ |
| Go | 1.23+ |
| jq | any |
| RAM | 8 GB free (3 GB per server) |

## Quick Start

```bash
cd experiments/cache/redis-vs-valkey

# Smoke test (~30 sec)
./run.sh --smoke-only

# Full benchmark (10M keys, ~5–15 min)
./run.sh --clean
```

Results in `results/summary.json` and printed to stdout.

## Operations Benchmarked

| Operation | Description | Unit |
|-----------|-------------|------|
| **pipeline-set** | SET keys in batches of 100 (pipelined) | keys/sec |
| **get** | Random GET from existing keys | ops/sec |
| **pipeline-get** | GET 100 keys per pipeline exec | cmds/sec |
| **mixed** | 80% GET / 20% SET per pipeline (realistic workload) | cmds/sec |

> `set` (individual, non-pipelined) is available via `--op set` but excluded from `all`
> because pipeline-set is the realistic and significantly faster path.

## Key/Value Schema

```
Key:   bench:0000000001
Value: {"id":"bench:0000000001","user_id":"u000123456","score":42,
        "tier":"premium","tags":["sale","new"],"ts":"2024-01-01T00:00:00Z",
        "meta":"padding-padding-padding-padding-00"}
```

~200 bytes per value. 10M keys × 200 bytes ≈ 2 GB in memory per server.

## Configuration

Both servers use `redis.conf`:
```
maxmemory 3gb
maxmemory-policy allkeys-lru
save ""           # disable persistence
appendonly no
```

## Customization

```bash
KEY_COUNT=1000000 WORKERS=8 ./run.sh --clean   # 1M keys, fewer workers
```

| Variable | Default | Description |
|----------|---------|-------------|
| `KEY_COUNT` | 10000000 | Total keys to insert |
| `ITERATIONS` | 100000 | Iterations for get/pipeline-get/mixed |
| `PIPE_SIZE` | 100 | Commands per pipeline batch |
| `WORKERS` | 16 | Concurrent goroutines |

## Infrastructure

| Service | Port | Description |
|---------|------|-------------|
| redis | 6379 | Redis 7 |
| valkey | 6380 | Valkey 8 |
| prometheus | 9091 | Metrics (note: 9091 to avoid conflict with exp #1) |
| grafana | 3001 | Dashboards (note: 3001 to avoid conflict with exp #1) |

## Interpreting Results

- **QPS** is the primary metric for cache workloads
- **p50/p99** latency shows consistency — p99 spike = GC pause or connection limit
- **Memory** (from `INFO memory`) after insert shows per-server overhead
- Valkey 8 targets Redis 7 compatibility with performance improvements

## Sample Results

> MacBook Pro M2 Pro, 16GB RAM, Docker Desktop

```
DB       Operation       QPS    p50(ms)    p95(ms)      p99(ms)    Memory      Keys
-------- -------------- ------------ -------- -------- ------------ -------- --------
redis    pipeline-set     312000       0.31       0.58         1.12   2.10GiB 10000000
valkey   pipeline-set     334000       0.29       0.51         0.98   1.97GiB 10000000
redis    get              198000       0.04       0.09         0.18         -        -
valkey   get              211000       0.04       0.08         0.16         -        -
redis    pipeline-get    1840000       0.52       0.91         1.43         -        -
valkey   pipeline-get    1970000       0.48       0.84         1.31         -        -
redis    mixed           1620000       0.55       0.98         1.62         -        -
valkey   mixed           1730000       0.51       0.91         1.48         -        -
```

*(Actual numbers vary — run on your hardware for authoritative results)*

## Troubleshooting

**`valkey-cli: command not found`** — Use `docker compose exec valkey valkey-cli ping`

**`FLUSHALL` warning** — `--flush` only affects the bench database; production data is not affected if using separate ports.

**OOM: 10M keys exceed available RAM** — Reduce with `KEY_COUNT=1000000 ./run.sh`.

**Port 6379 already in use** — Stop local Redis: `brew services stop redis` or `redis-cli shutdown`.
