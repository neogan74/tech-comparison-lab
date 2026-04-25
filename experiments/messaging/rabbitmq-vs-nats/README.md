# Experiment #10: RabbitMQ vs NATS — Lightweight Messaging Benchmark

Benchmarks **RabbitMQ 3** vs **NATS 2.10 JetStream** on lightweight messaging workloads:
1M messages, 3 concurrent consumers, measuring throughput and per-batch latency.

## Prerequisites

| Dependency | Version |
|------------|---------|
| Docker + Compose v2 | Docker 24+ |
| Go | 1.23+ |
| jq | any |
| RAM | 2 GB free |

## Quick Start

```bash
cd experiments/messaging/rabbitmq-vs-nats

./run.sh --smoke-only        # 10k messages, ~1 min
./run.sh --clean             # 1M messages, ~5-15 min
SKIP_BUILD=true ./run.sh --smoke-only
```

Full runs emit `results/rabbitmq.json`, `results/nats.json`, and `results/summary.json`.

## Operations

| Operation | Description | Unit |
|-----------|-------------|------|
| **produce** | Send N messages in batches of 1000 | msg/s |
| **consume** | 3 consumers drain the queue/stream concurrently | msg/s |

### Fairness notes
- **RabbitMQ**: Publisher confirms enabled (at-least-once reliability). Queue is durable, messages transient. 3 consumers on same queue (round-robin by broker).
- **NATS JetStream**: Synchronous publish with ack (`js.Publish`). Memory storage, `Replicas=1`. Each consumer gets an independent pull consumer — messages distributed across consumers.
- Both use batch size = 1000 for produce. Consumers run concurrently.

## Message Format (~200 bytes)

```json
{"id":42,"ts":1704067200000000000,"p":"AAAAA..."}
```

Fixed payload keeps message size consistent across runs.

## Infrastructure

| Service | Port | URL |
|---------|------|-----|
| RabbitMQ AMQP | 5672 | amqp://bench:benchpass@localhost:5672/ |
| RabbitMQ Mgmt | 15672 | http://localhost:15672 (bench/benchpass) |
| NATS | 4222 | nats://localhost:4222 |
| NATS Monitoring | 8222 | http://localhost:8222 |
| rabbitmq-exporter | 9419 | Prometheus metrics |
| nats-exporter | 7777 | Prometheus metrics |
| Prometheus | 9096 | http://localhost:9096 |
| Grafana | 3004 | http://localhost:3004 |

## Customization

```bash
MSG_COUNT=100000 CONSUMERS=3 ./run.sh --clean
```

| Variable | Default | Description |
|----------|---------|-------------|
| `MSG_COUNT` | 1000000 | Total messages |
| `BATCH_SIZE` | 1000 | Messages per batch |
| `CONSUMERS` | 3 | Concurrent consumers |
| `SMOKE_COUNT` | 10000 | Messages sent during smoke run |
| `SKIP_BUILD` | false | Skip `go build` and use existing binary |

If `go` is unavailable but `benchmarks/loadgen-msg/bin/loadgen-msg` already
exists, `run.sh` falls back to that binary.

## Sample Results

> MacBook Pro M2 Pro, 16GB RAM, Docker Desktop

```
DB           Operation     msg/s      p50(ms)    p95(ms)      p99(ms)
------------ ---------- ------------ -------- -------- ------------
rabbitmq     produce        42000       23.8       41.2         68.9
nats         produce       210000        4.8        9.3         17.6
rabbitmq     consume       185000          -          -            -
nats         consume       560000          -          -            -
```

*(Actual numbers vary — run on your hardware)*

## Troubleshooting

**RabbitMQ not ready after 300s** — The runner waits for both `rabbitmq-diagnostics ping` and a real AMQP dry-run. On cold starts with old Docker volumes, retry with `./run.sh --clean`.

**NATS not ready** — NATS starts in <5s normally. If it fails, check `docker compose logs nats`.

**Consumer gets 0 messages** — Make sure `produce` ran first (`--op all` does this automatically).

**First build fails downloading modules** — The first `go build` may require internet access. If the binary already exists, run with `SKIP_BUILD=true`.