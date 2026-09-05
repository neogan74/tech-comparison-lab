# Benchmark Results

Generated from `docs/index.json` at `2026-09-05T07:25:01Z`.

## Runs

| Experiment | Category | Timestamp | Mode | Results | Report |
|------------|----------|-----------|------|---------|--------|
| ClickHouse vs PostgreSQL | `analytics` | `2026-04-06T13:36:55Z` | `full` | 10 | [report](experiments/clickhouse-vs-postgresql.md) |
| Envoy vs NGINX | `api` | `2026-09-05T07:20:25Z` | `full` | 6 | [report](experiments/envoy-vs-nginx.md) |
| GraphQL vs REST | `api` | `2026-06-30T18:22:40Z` | `full` | 6 | [report](experiments/graphql-vs-rest.md) |
| gRPC vs REST | `api` | `2026-04-04T08:53:52Z` | `full` | 6 | [report](experiments/grpc-vs-rest.md) |
| Kong vs Traefik | `api` | `2026-07-13T17:17:59Z` | `full` | 6 | [report](experiments/kong-vs-traefik.md) |
| Redis vs Valkey | `cache` | `2026-04-06T12:39:53Z` | `full` | 8 | [report](experiments/redis-vs-valkey.md) |
| PostgreSQL vs MongoDB | `databases` | `2026-04-04T05:02:34Z` | `full` | 8 | [report](experiments/postgresql-vs-mongodb.md) |
| Kafka vs RabbitMQ | `messaging` | `2026-04-22T07:17:38Z` | `full` | 4 | [report](experiments/kafka-vs-rabbitmq.md) |
| Kubernetes vs Docker Swarm | `orchestration` | `2026-07-17T14:12:43.111312Z` | `smoke` | 8 | [report](experiments/kubernetes-vs-docker-swarm.md) |
| Argo CD vs Flux CD | `platform` | `2026-07-09T19:06:15Z` | `full` | 6 | [report](experiments/argocd-vs-fluxcd.md) |

## Snapshot

- **ClickHouse vs PostgreSQL**: top throughput `clickhouse/time-range` = 14325756.47
- **Envoy vs NGINX**: top throughput `rest/get-user` = 24883.39
- **GraphQL vs REST**: top throughput `graphql/create-order` = 67735.62
- **gRPC vs REST**: top throughput `grpc/get-user` = 49325.89
- **Kong vs Traefik**: top throughput `rest/get-user` = 15066.22
- **Redis vs Valkey**: top throughput `valkey/pipeline-set` = 146142.18
- **PostgreSQL vs MongoDB**: top throughput `mongo/insert` = 8053.32
- **Kafka vs RabbitMQ**: top throughput `kafka/produce` = 95836.51
- **Kubernetes vs Docker Swarm**: no throughput metric found
- **Argo CD vs Flux CD**: no throughput metric found

## Schemas

- [`results-summary/v1`](results-summary-v1.md)
- [`results-index/v1`](results-index-v1.md)

