# Experiment #8: Kafka vs RabbitMQ — Messaging Benchmark

Benchmarks **Apache Kafka 3.7 (KRaft)** vs **RabbitMQ 3** on messaging workloads:
1M messages, 3 concurrent consumers, measuring throughput and per-batch latency.

## Prerequisites

| Dependency | Version |
|------------|---------|
| Docker + Compose v2 | Docker 24+ |
| Go | 1.23+ |
| jq | any |
| RAM | 4 GB free |

## Quick Start

```bash
cd experiments/messaging/kafka-vs-rabbitmq

./run.sh --smoke-only        # 10k messages, ~1 min
./run.sh --clean             # 1M messages, ~5-15 min
SKIP_BUILD=true ./run.sh --smoke-only
```

Full runs emit `results/kafka.json`, `results/rabbitmq.json`, and
`results/summary.json`.

## Operations

| Operation | Description | Unit |
|-----------|-------------|------|
| **produce** | Send N messages in batches of 1000 | msg/s |
| **consume** | 3 consumers drain the topic/queue concurrently | msg/s |

### Fairness notes
- **Kafka**: `RequiredAcks=1` (leader ack). Batching via `kafka.Writer.WriteMessages`.
- **RabbitMQ**: Publisher confirms enabled (equivalent reliability). Queue is durable, messages transient.
- Both use batch size = 1000 for produce. Consumers run concurrently.
- Kafka: 3 partitions → each consumer owns 1 partition (clean distribution).
- RabbitMQ: 3 consumers on same queue (round-robin by broker).

## Message Format (~200 bytes)

```json
{"id":42,"ts":1704067200000000000,"p":"AAAAA..."}
```

Fixed payload keeps message size consistent across runs.

## Infrastructure

| Service | Port | URL |
|---------|------|-----|
| Kafka | 9093 | localhost:9093 (PLAINTEXT_HOST) |
| RabbitMQ AMQP | 5672 | amqp://bench:benchpass@localhost:5672/ |
| RabbitMQ Mgmt | 15672 | http://localhost:15672 (bench/benchpass) |
| kafka-exporter | 9308 | Prometheus metrics |
| Prometheus | 9094 | http://localhost:9094 |
| Grafana | 3002 | http://localhost:3002 |

## Customization

```bash
MSG_COUNT=100000 CONSUMERS=3 ./run.sh --clean
```

| Variable | Default | Description |
|----------|---------|-------------|
| `MSG_COUNT` | 1000000 | Total messages |
| `BATCH_SIZE` | 1000 | Messages per batch |
| `CONSUMERS` | 3 | Concurrent consumers |
| `PARTITIONS` | 3 | Kafka topic partitions |
| `SMOKE_COUNT` | 10000 | Messages sent during smoke run |
| `SKIP_BUILD` | false | Skip `go build` and use existing binary |

If `go` is unavailable but `benchmarks/loadgen-msg/bin/loadgen-msg` already
exists, `run.sh` falls back to that binary.

## Sample Results

> MacBook Pro M2 Pro, 16GB RAM, Docker Desktop

```
DB           Operation     msg/s      p50(ms)    p95(ms)      p99(ms)
------------ ---------- ------------ -------- -------- ------------
kafka        produce        98000       10.2       18.4         32.1
rabbitmq     produce        42000       23.8       41.2         68.9
kafka        consume       312000          -          -            -
rabbitmq     consume       187000          -          -            -
```

Consumer distribution (kafka):
```
  consumer[0]:  334000  msgs/s
  consumer[1]:  333000  msgs/s
  consumer[2]:  333000  msgs/s
```

*(Actual numbers vary — run on your hardware)*

## Troubleshooting

**Kafka health check fails (>5 min)** — Bitnami Kafka KRaft needs ~30s to initialize. `./run.sh` waits up to 5 min.

**`kafka-topics.sh: command not found`** — Only inside the container. The `run.sh` uses `docker compose exec`.

**Port 9093 conflicts** — Check `lsof -i :9093`. Kill the conflicting process or change the port in `docker-compose.yml`.

**RabbitMQ `confirm` mode errors** — Ensure broker allows publisher confirms (default: yes).

**Consumer gets 0 messages** — Make sure `produce` ran first (`--op all` does this automatically).

**First build fails downloading modules** — The first `go build` may require
internet access. If the binary already exists, run with `SKIP_BUILD=true`.
