# MongoDB vs Cassandra — Distributed NoSQL

Compares MongoDB 7 and Cassandra 5 on the same generated order documents and
four operations: insert, country lookup, global aggregation, and targeted
update. The experiment measures latency percentiles and throughput while also
showing the cost of Cassandra's query-first data model.

## Quick start

Prerequisites: Docker with Compose v2, Go 1.23+, `jq`, and at least 6 GB of free
RAM. Cassandra's first startup can take several minutes.

```bash
cd experiments/databases/mongodb-vs-cassandra
./run.sh --smoke-only
./run.sh --clean
```

The full run defaults to 1,000,000 documents. Override it when iterating:

```bash
INSERT_COUNT=100000 WORKERS=8 ./run.sh --clean
```

## Workload

| Operation | MongoDB | Cassandra |
|---|---|---|
| insert | unordered `InsertMany` | concurrent individual CQL writes |
| query | indexed `user.country=US`, limit 100 | fan-out over country buckets, limit 100 total |
| agg | server-side group/sort by user | stream all partitions and reduce/sort in the client |
| update | find 1000 US IDs, `UpdateMany` | find keys across buckets, update by full primary key |

MongoDB stores the nested document directly. Cassandra stores the same logical
fields in `orders_by_country`, partitioned by `(country, bucket)` with 32 stable
buckets. Bucketing prevents the 40% US share from creating one unbounded hot
partition.

The aggregation intentionally exposes a major modeling difference: Cassandra
does not provide an efficient arbitrary global `GROUP BY`, so the load generator
performs the cross-partition reduction. For a production Cassandra design this
would normally be a separately maintained rollup table. Treat the result as the
cost of answering an unmodeled analytical query, not as a pure database-engine
microbenchmark.

## Configuration

| Variable | Default | Description |
|---|---:|---|
| `INSERT_COUNT` | 1000000 | documents per database |
| `QUERY_ITERATIONS` | 1000 | country lookup iterations |
| `AGG_ITERATIONS` | 1 | global aggregation iterations |
| `UPDATE_ITERATIONS` | 10 | update iterations |
| `WORKERS` | 16 | concurrent insert workers |
| `BATCH_SIZE` | 100 | generated documents per worker chunk |
| `SMOKE_COUNT` | 500 | documents per smoke target |
| `MONGO_PASSWORD` | benchpass | MongoDB root password |
| `MONGO_PORT` | 27017 | published MongoDB port |
| `CASSANDRA_PORT` | 9042 | published Cassandra CQL port |

`BATCH_SIZE` is a load-generator work chunk for Cassandra; it deliberately does
not create cross-partition CQL `BATCH` statements, which are an anti-pattern.

## Results

Full runs write:

- `results/mongo.json`
- `results/cassandra.json`
- `results/summary.json` (`results-summary/v1`)

For insert results, throughput is documents per second while latency is measured
per generated work chunk. Other operations report iterations per second. A zero
`storage_bytes` value for Cassandra means storage size is not collected because
portable per-table disk accounting is not exposed through CQL.

## Infrastructure and cleanup

The Compose stack lives at
`deployments/docker-compose/mongodb-vs-cassandra/docker-compose.yml` and starts
MongoDB 7 plus a single-node Cassandra 5 cluster. This measures local engine and
data-model behavior; it is not a multi-node replication benchmark.

```bash
docker compose -f deployments/docker-compose/mongodb-vs-cassandra/docker-compose.yml down -v
```
