# ScyllaDB vs Cassandra — Distributed NoSQL

Compares **Cassandra 5.0** and **ScyllaDB 5.4** on the *identical* bucketed
data model and workload, since both speak CQL. Unlike the MongoDB vs Cassandra
experiment, this one holds the data model and queries fixed and only swaps the
engine — the same `orders_by_country` schema, the same 32-bucket partitioning,
the same insert/query/agg/update operations, driven by the same
`loadgen-db --db cassandra|scylladb` code path.

## Quick start

Prerequisites: Docker with Compose v2, Go 1.23+, `jq`, and at least 4 GB of free
RAM. Cassandra's first startup can take several minutes; ScyllaDB is
comparatively fast to boot.

```bash
cd experiments/databases/scylladb-vs-cassandra
./run.sh --smoke-only
./run.sh --clean
```

The full run defaults to 1,000,000 documents. Override it when iterating:

```bash
INSERT_COUNT=100000 WORKERS=8 ./run.sh --clean
```

## Workload

Both engines run the exact same CQL against the exact same schema
(`bench.orders_by_country`, partitioned by `(country, bucket)` with 32 stable
buckets — see [MongoDB vs Cassandra](../mongodb-vs-cassandra/README.md) for
why bucketing exists):

| Operation | Query |
|---|---|
| insert | concurrent individual CQL writes across `WORKERS` goroutines |
| query | fan-out over country buckets, `LIMIT 100` total |
| agg | stream all partitions and reduce/sort top users in the client |
| update | find keys across buckets, update by full primary key |

The client-side aggregation cost (`agg`) is identical on both sides — CQL has
no efficient arbitrary global `GROUP BY` regardless of which engine executes
it — so this experiment isolates engine performance (compaction strategy,
concurrency model, driver overhead) rather than data-model tradeoffs.

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
| `CASSANDRA_PORT` | 9042 | published Cassandra CQL port |
| `SCYLLA_PORT` | 9043 | published ScyllaDB CQL port |

`BATCH_SIZE` is a load-generator work chunk, not a cross-partition CQL
`BATCH` statement (those are an anti-pattern on both engines).

## Results

Full runs write:

- `results/cassandra.json`
- `results/scylladb.json`
- `results/summary.json` (`results-summary/v1`)

For insert results, throughput is documents per second while latency is
measured per generated work chunk. Other operations report iterations per
second. `storage_bytes` is not collected for either engine — portable
per-table disk accounting isn't exposed through CQL.

## Fairness notes

- Single-node for both engines — this measures per-node engine behavior
  (SEDA vs ScyllaDB's shard-per-core architecture), not multi-node
  replication, repair, or gossip overhead.
- ScyllaDB runs with `--smp 1 --overprovisioned 1 --developer-mode 1`: a
  single shard, tuned for shared CI hardware rather than production. Its
  shard-per-core design is built to scale near-linearly with cores — running
  it at `--smp 1` intentionally removes that main advantage so the comparison
  stays apples-to-apples on constrained hardware. Results here should be read
  as single-shard engine efficiency, not ScyllaDB's peak throughput.
- Both use the default `SimpleStrategy` keyspace with `replication_factor: 1`
  (irrelevant on a single node, kept for schema parity with the
  MongoDB vs Cassandra experiment's `init.cql`).

## Infrastructure and cleanup

The Compose stack lives at
`deployments/docker-compose/scylladb-vs-cassandra/docker-compose.yml` and
starts a single-node Cassandra 5.0 and a single-node ScyllaDB 5.4, each on its
own CQL port (`9042` / `9043`) so both can run side by side.

```bash
docker compose -f deployments/docker-compose/scylladb-vs-cassandra/docker-compose.yml down -v
```
