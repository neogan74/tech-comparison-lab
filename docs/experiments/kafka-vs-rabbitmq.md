# Kafka vs RabbitMQ

Generated from `experiments/messaging/kafka-vs-rabbitmq/results/summary.json`.

## Metadata

| Field | Value |
|-------|-------|
| Experiment | Kafka vs RabbitMQ |
| Category | `messaging` |
| Run Timestamp | `2026-04-07T09:57:27Z` |
| Mode | `full` |
| Run ID | `kafka-1775555847` |
| Result Count | 4 |

## Config

| Key | Value |
|-----|-------|
| `msg_count` | `1000000` |
| `batch_size` | `1000` |
| `consumers` | `3` |
| `partitions` | `3` |

## Sources

| Name | File |
|------|------|
| `kafka` | `results/kafka.json` |
| `rabbitmq` | `results/rabbitmq.json` |

## Highlights

- `consume`: throughput leader `rabbitmq` (75720.39 msg/s, 52.84x vs `kafka`); lowest p95 `kafka` (0 ms)
- `produce`: throughput leader `rabbitmq` (29470.05 msg/s, 3.42x vs `kafka`); lowest p95 `rabbitmq` (75.356 ms, 4.06x better than `kafka`)

## Results

| Subject | Operation | Count | p50 ms | p95 ms | p99 ms | Total ms | Throughput | Errors | Storage / Memory |
|---------|-----------|-------|--------|--------|--------|----------|------------|--------|------------------|
| `kafka` | `produce` | 1000000 | 70.8 | 305.769 | 944.53 | 116072 | 8615.32 msg/s | - | - |
| `kafka` | `consume` | 1000000 | 0 | 0 | 0 | 697810 | 1433.05 msg/s | - | - |
| `rabbitmq` | `produce` | 1000000 | 28.384 | 75.356 | 131.128 | 33932 | 29470.05 msg/s | - | - |
| `rabbitmq` | `consume` | 1000000 | 0 | 0 | 0 | 13206 | 75720.39 msg/s | - | - |

