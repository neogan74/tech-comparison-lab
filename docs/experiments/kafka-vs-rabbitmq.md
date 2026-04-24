# Kafka vs RabbitMQ

Generated from `experiments/messaging/kafka-vs-rabbitmq/results/summary.json`.

## Metadata

| Field | Value |
|-------|-------|
| Experiment | Kafka vs RabbitMQ |
| Category | `messaging` |
| Run Timestamp | `2026-04-22T07:17:38Z` |
| Mode | `full` |
| Run ID | `kafka-1776842258` |
| Result Count | 4 |

## Config

| Key | Value |
|-----|-------|
| `msg_count` | `100000` |
| `batch_size` | `1000` |
| `consumers` | `3` |
| `partitions` | `3` |

## Sources

| Name | File |
|------|------|
| `kafka` | `results/kafka.json` |
| `rabbitmq` | `results/rabbitmq.json` |

## Highlights

- `consume`: throughput leader `rabbitmq` (85310.54 msg/s, 18.35x vs `kafka`); lowest p95 `kafka` (0 ms)
- `produce`: throughput leader `kafka` (95836.51 msg/s, 2.46x vs `rabbitmq`); lowest p95 `kafka` (11.307 ms, 5.48x better than `rabbitmq`)

## Results

| Subject | Operation | Count | p50 ms | p95 ms | p99 ms | Total ms | Throughput | Errors | Storage / Memory |
|---------|-----------|-------|--------|--------|--------|----------|------------|--------|------------------|
| `kafka` | `produce` | 100000 | 9.467 | 11.307 | 13.409 | 1043 | 95836.51 msg/s | - | - |
| `kafka` | `consume` | 100000 | 0 | 0 | 0 | 21507 | 4649.6 msg/s | - | - |
| `rabbitmq` | `produce` | 100000 | 21.915 | 61.931 | 73.736 | 2564 | 38991.96 msg/s | - | - |
| `rabbitmq` | `consume` | 100000 | 0 | 0 | 0 | 1172 | 85310.54 msg/s | - | - |

