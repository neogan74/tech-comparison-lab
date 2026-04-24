# Experiment #9: NATS vs Apache Kafka — High-Throughput Messaging Benchmark

Benchmarks **NATS 2.10 JetStream** vs **Apache Kafka 3.9 (KRaft)** on high-throughput messaging workloads:
1M messages, 3 concurrent consumers, measuring throughput and per-batch latency.

## Prerequisites

| Dependency | Version |
|------------|---------|
| Docker + Compose v2 | Docker 24+ |
| Go | 1.23+ |
| jq | any |
| RAM | 3 GB free |

## Quick Start

```bash
cd experiments/messaging/nats-vs-kafka

./run.sh --smoke-only        # 10k messages, ~1 min
./run.sh --clean             # 1M messages, ~5-15 min
SKIP_BUILD=true ./run.sh --smoke-only
```

Full runs emit `results/kafka.json`, `results/nats.json`, and `results/summary.json`.

## Operations

| Operation | Description | Unit |
|-----------|-------------|------|
| **produce** | Send N messages in batches of 1000 | msg/s |
| **consume** | 3 consumers drain the topic/stream concurrently | msg/s |

### Fairness notes
- **Kafka**: `RequiredAcks=1` (leader ack). Batching via `kafka.Writer.WriteMessages`. 3 partitions.
- **NATS JetStream**: Synchronous publish with ack (`js.Publish`). Memory storage, `Replicas=1`. Each consumer gets an independent pull consumer.
- Both use batch size = 1000 for produce. Consumers run concurrently.
- Kafka: 3 partitions → each consumer owns 1 partition.
- NATS: 3 independent pull consumers on the same stream — messages are distributed across consumers.

## Message Format (~200 bytes)

```json
{"id":42,"ts":1704067200000000000,"p":"AAAAA..."}
```

Fixed payload keeps message size consistent across runs.

## Infrastructure

| Service | Port | URL |
|---------|------|-----|
| Kafka | 9093 | localhost:9093 (PLAINTEXT_HOST) |
| NATS | 4222 | nats://localhost:4222 |
| NATS Monitoring | 8222 | http://localhost:8222 |
| nats-exporter | 7777 | Prometheus metrics |
| kafka-exporter | 9308 | Prometheus metrics |
| Prometheus | 9095 | http://localhost:9095 |
| Grafana | 3003 | http://localhost:3003 |

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
kafka        produce        95000       10.5       19.1         35.2
nats         produce       210000        4.8        9.3         17.6
kafka        consume       310000          -          -            -
nats         consume       580000          -          -            -
```

*(Actual numbers vary — run on your hardware)*

## Troubleshooting

**Kafka health check fails (>5 min)** — Kafka KRaft may need ~30-60s to initialize. `./run.sh` waits up to 5 min.

**NATS not ready** — NATS starts in <5s normally. If it fails, check `docker compose logs nats`.

**Port 9093 conflicts** — Check `lsof -i :9093`. Kill the conflicting process or change the port in `docker-compose.yml`.

**NATS JetStream not available** — The container is started with `-js` flag. Verify with `http://localhost:8222/jsz`.

**Consumer gets 0 messages** — Make sure `produce` ran first (`--op all` does this automatically).

**First build fails downloading modules** — The first `go build` may require internet access. If the binary already exists, run with `SKIP_BUILD=true`.