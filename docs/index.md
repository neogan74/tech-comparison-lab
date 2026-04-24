# Benchmark Results

Generated from `docs/index.json` at `2026-04-22T07:20:14Z`.

## Runs

| Experiment | Category | Timestamp | Mode | Results | Report |
|------------|----------|-----------|------|---------|--------|
| ClickHouse vs PostgreSQL | `analytics` | `2026-04-06T13:36:55Z` | `full` | 10 | [report](experiments/clickhouse-vs-postgresql.md) |
| gRPC vs REST | `api` | `2026-04-04T08:53:52Z` | `full` | 6 | [report](experiments/grpc-vs-rest.md) |
| Redis vs Valkey | `cache` | `2026-04-06T12:39:53Z` | `full` | 8 | [report](experiments/redis-vs-valkey.md) |
| PostgreSQL vs MongoDB | `databases` | `2026-04-04T05:02:34Z` | `full` | 8 | [report](experiments/postgresql-vs-mongodb.md) |
| Kafka vs RabbitMQ | `messaging` | `2026-04-22T07:17:38Z` | `full` | 4 | [report](experiments/kafka-vs-rabbitmq.md) |

## Snapshot

- **ClickHouse vs PostgreSQL**: top throughput `clickhouse/time-range` = 14325756.47
- **gRPC vs REST**: top throughput `grpc/get-user` = 49325.89
- **Redis vs Valkey**: top throughput `valkey/pipeline-set` = 146142.18
- **PostgreSQL vs MongoDB**: top throughput `mongo/insert` = 8053.32
- **Kafka vs RabbitMQ**: top throughput `kafka/produce` = 95836.51

## Schemas

- [`results-summary/v1`](results-summary-v1.md)
- [`results-index/v1`](results-index-v1.md)

