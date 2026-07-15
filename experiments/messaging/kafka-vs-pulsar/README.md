# Apache Kafka vs Apache Pulsar — Messaging Benchmark

Compares **Apache Kafka 3.9 (KRaft)** and **Apache Pulsar 4.0 LTS (standalone)**
on acknowledged produce and concurrent consume workloads using the same message
payload, partition count, batch size, and consumer count.

## Prerequisites

| Dependency | Version |
|------------|---------|
| Docker + Compose v2 | Docker 24+ |
| Go | 1.23+ |
| jq | any |
| RAM | 4 GB free |

Pulsar standalone runs its broker, BookKeeper, and metadata store in one JVM,
so its startup is heavier than Kafka's single-node KRaft container.

## Quick Start

```bash
cd experiments/messaging/kafka-vs-pulsar

./run.sh --smoke-only
./run.sh --clean
SKIP_BUILD=true ./run.sh --smoke-only
```

Full runs emit `results/kafka.json`, `results/pulsar.json`, and
`results/summary.json`. The merged file follows `results-summary/v1`.

## Workloads

| Operation | Description | Unit |
|-----------|-------------|------|
| **produce** | Send N messages and wait for broker acknowledgements after each client batch | msg/s and batch latency |
| **consume** | Drain the complete topic through 3 concurrent consumers | msg/s |

Messages use the same approximately 200-byte JSON shape for both brokers:

```json
{"id":42,"ts":1704067200000000000,"p":"AAAAA..."}
```

## Fairness and Semantics

- Both topics have `PARTITIONS=3` and use one broker with replication factor 1.
- Kafka uses `RequiredAcks=1`; Pulsar waits for the broker acknowledgement of
  every asynchronously submitted message.
- Both producers submit `BATCH_SIZE` messages at a time with a 5 ms maximum
  client batching delay.
- Kafka consumers share a unique consumer group. Pulsar consumers share a
  unique `Shared` subscription starting at the earliest retained message. Its
  receiver queue is intentionally small so the first subscriber cannot prefetch
  the complete backlog before the other consumers attach.
- The Pulsar partitioned topic is deleted and recreated before each measured
  workload. Kafka's topic is likewise deleted and recreated by the load
  generator. This prevents a previous run's backlog from changing consume
  results.
- Pulsar standalone is appropriate for reproducible local and CI comparison,
  but it does not represent a production multi-broker Pulsar deployment.

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `MSG_COUNT` | 1000000 | Messages per broker in the full run |
| `BATCH_SIZE` | 1000 | Messages submitted per producer batch |
| `CONSUMERS` | 3 | Concurrent consumers |
| `PARTITIONS` | 3 | Partitions for both topics |
| `SMOKE_COUNT` | 5000 | Messages per broker in smoke mode |
| `KAFKA_ADDR` | `localhost:9093` | Kafka client endpoint |
| `PULSAR_ADDR` | `pulsar://localhost:6650` | Pulsar binary-protocol endpoint |
| `SKIP_BUILD` | `false` | Use the existing loadgen binary |

Example:

```bash
MSG_COUNT=100000 BATCH_SIZE=500 CONSUMERS=3 ./run.sh --clean
```

## Infrastructure

| Service | Port | URL |
|---------|------|-----|
| Kafka | 9093 | `localhost:9093` |
| Pulsar broker | 6650 | `pulsar://localhost:6650` |
| Pulsar admin and metrics | internal 8080 | scraped by Prometheus |
| kafka-exporter | 9308 | `http://localhost:9308/metrics` |
| Prometheus | 9096 | `http://localhost:9096` |
| Grafana | 3004 | `http://localhost:3004` |

The benchmark runner starts the two brokers. Start the optional monitoring
services separately when needed:

```bash
docker compose -f deployments/docker-compose/kafka-vs-pulsar/docker-compose.yml \
  up -d kafka-exporter prometheus grafana
```

## Validation

```bash
make validate EXP=kafka-vs-pulsar
make smoke EXP=kafka-vs-pulsar
```

## Troubleshooting

**Pulsar is not ready after 300 seconds** — inspect
`docker compose -f deployments/docker-compose/kafka-vs-pulsar/docker-compose.yml logs pulsar`.
Cold starts need to initialize the embedded metadata and BookKeeper state.

**The Pulsar container is killed** — allocate at least 4 GB to Docker Desktop.
The runner caps the Pulsar JVM heap at 512 MB and direct memory at 256 MB, but
the full standalone process needs additional native and page-cache memory.

**Port conflict** — verify ports `6650` and `9093` are free, then stop
the conflicting local service or change the port mapping and matching address.

**First build cannot download modules** — the official Pulsar Go client has a
larger dependency graph than the existing messaging clients. Retry with network
access, or use `SKIP_BUILD=true` after a successful build.
