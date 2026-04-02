# Experiments

Here is list of experiments to run:

## Databases

### 1. JSON workload

`PostgreSQL vs MongoDB`

Experiment

```
    dataset: 10M JSON documents
    operations:
    - insert 
    - filter by nested field
    - aggregation
- update nested field
```
Metrics
```
    latency
    storage size
    index speed
```

### 2. Distributed SQL

`PostgreSQL vs CockroachDB`

Test
```
simulate multi region writes
network latency: 50ms
```
We will check:
```
    transaction latency
    consistency
    failover
```

### 3. Wide column vs document

`Cassandra vs MongoDB`

Test
```
write heavy workload
100k writes/sec
```
Metrics
```
    throughput
    replication lag
```
4. Analytics

ClickHouse vs PostgreSQL

Dataset

1B events

Query

SELECT count(*) GROUP BY user_id

Метрики

query time

memory usage

⚡ Cache / KV
5. Redis replacement

Redis vs Valkey

Test

10M keys
SET / GET
pipeline

Метрики

QPS

memory usage

6. Cache performance

Redis vs Memcached

Test

1M keys
cache hit test
7. KV store consensus

etcd vs Consul

Test

leader failover
100 writes/sec

Метрики

leader election time

consistency

📡 Messaging
8. Streaming vs queue

Apache Kafka vs RabbitMQ

Test

1M messages
3 consumers

Метрики

throughput

consumer lag

9. High throughput messaging

Apache Kafka vs NATS

Test

100k messages/sec
10. Next-gen messaging

Apache Kafka vs Apache Pulsar

Test

multi tenant workload
☸️ Kubernetes
11. Scheduler comparison

Kubernetes vs Nomad

Test

deploy 1000 pods

Метрики

scheduling time

resource usage

12. GitOps

Argo CD vs Flux CD

Test

deploy 100 services
13. Service mesh

Istio vs Linkerd

Test

1000 rps service mesh

Метрики

latency overhead

CPU usage

📊 Observability
14. Metrics storage

Prometheus vs VictoriaMetrics

Test

10M time series
15. Logs

Loki vs Elasticsearch

Test

100GB logs/day
🌐 API
16. RPC vs HTTP

gRPC vs REST

Test

100k rps
17. API query models

GraphQL vs REST

Test

complex nested queries
🚀 CI/CD
18. CI systems

GitLab CI vs GitHub Actions

Test

build 100 pipelines
19. Kubernetes pipelines

Tekton vs Argo Workflows

20. Reverse proxy

Envoy vs NGINX

Test

100k rps