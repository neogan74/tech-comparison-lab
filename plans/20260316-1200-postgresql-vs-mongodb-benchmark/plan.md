# Plan: PostgreSQL vs MongoDB JSON Workload Benchmark

**Date:** 2026-03-16
**Status:** pending
**Priority:** high
**Experiment:** experiments.md §1 — JSON workload

## Goal

Reproducible benchmark comparing PostgreSQL 16 (JSONB + GIN) vs MongoDB 7 on
JSON document workloads (insert, query, aggregation, update). Single `run.sh`
entry point. Results emitted as stdout table + `results/summary.json`.

## Scope

- 10M document dataset (~500 bytes/doc)
- 4 operation types across both databases
- p50/p95/p99 latency + throughput + storage size metrics
- Docker Compose stack (no Kubernetes required)
- Self-contained Go benchmark tool (`loadgen-db`)

## Out of Scope

- Sharding / replication topology
- TLS / auth hardening for benchmark containers
- Grafana dashboards (observability layer is future work)
- CI pipeline integration (follow-up experiment)

## Phases

| # | File | Description | Status |
|---|------|-------------|--------|
| 1 | [phase-01-infrastructure.md](./phase-01-infrastructure.md) | Docker Compose stack (PG16 + Mongo7 + Prometheus + Grafana) | pending |
| 2 | [phase-02-loadgen-tool.md](./phase-02-loadgen-tool.md) | Go CLI benchmark tool (`loadgen-db`) | pending |
| 3 | [phase-03-experiment-runner.md](./phase-03-experiment-runner.md) | `run.sh` orchestrator + experiment README | pending |

## Deliverables

```
deployments/docker-compose/
  docker-compose.yml
  postgres/init.sql

benchmarks/loadgen-db/
  go.mod
  main.go
  internal/postgres/bench.go
  internal/mongo/bench.go
  internal/report/report.go

experiments/databases/postgresql-vs-mongodb/
  README.md
  run.sh
  results/.gitkeep
```

## Dependencies

- Docker + Docker Compose v2 on host
- Go 1.23+ (build only; binary runs in container or locally)
- No cloud resources required

## Success Criteria

- `run.sh` completes end-to-end without manual steps
- Results JSON contains p50/p95/p99/throughput for all 8 operation-db combos
- Storage sizes captured post-insert for both engines
