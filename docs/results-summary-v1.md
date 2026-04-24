# Results Summary Schema v1

This document defines the experiment-level `results/summary.json` contract.

The schema applies to merged summaries produced by experiment runners such as:

- `experiments/databases/postgresql-vs-mongodb/run.sh`
- `experiments/api/grpc-vs-rest/run.sh`
- `experiments/cache/redis-vs-valkey/run.sh`
- `experiments/analytics/clickhouse-vs-postgresql/run.sh`
- `experiments/messaging/kafka-vs-rabbitmq/run.sh`
- `experiments/orchestration/k8s-vs-openshift/run.sh`

It does not replace the raw per-target JSON emitted directly by benchmark binaries.

## Top-level shape

```json
{
  "schema_version": "results-summary/v1",
  "experiment": {
    "id": "postgresql-vs-mongodb",
    "name": "PostgreSQL vs MongoDB",
    "category": "databases",
    "path": "experiments/databases/postgresql-vs-mongodb"
  },
  "run_id": "postgres-1719999999",
  "timestamp": "2026-04-03T10:00:00Z",
  "mode": "full",
  "config": {},
  "sources": [
    { "name": "postgres", "file": "results/postgres.json" },
    { "name": "mongo", "file": "results/mongo.json" }
  ],
  "results": []
}
```

## Field rules

- `schema_version`: fixed string for the merged summary contract.
- `experiment`: stable experiment metadata.
- `run_id`: copied from one of the raw benchmark summaries for traceability.
- `timestamp`: copied from one of the raw benchmark summaries.
- `mode`: currently `full` for `summary.json`. Smoke runs do not emit merged summaries.
- `config`: experiment-specific runner configuration for the full run.
- `sources`: raw per-target summaries used to build the merged artifact.
- `results`: flattened array from all source summaries.

## Notes

- `results` entries remain experiment-specific. This schema standardizes the wrapper, not every metric field inside each result item.
- Consumers should rely on `experiment.id`, `config`, and `sources` for cross-experiment automation, then interpret `results` according to the experiment domain.
