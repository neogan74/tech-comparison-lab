# Plan: etcd vs Consul KV Experiment

**Date:** 2026-07-01  
**Category:** kv (KV store / consensus)  
**Status:** 🔲 Not started

## Overview

Add a new experiment comparing **etcd** and **Consul** as KV stores with consensus semantics. Follows the exact repo pattern (loadgen Go tool → Docker Compose stack → run.sh → validate/smoke scripts → CI jobs).

## Phases

| # | Phase | Status | File |
|---|-------|--------|------|
| 01 | Go benchmark tool (`loadgen-kv`) | 🔲 | [phase-01-loadgen-kv.md](phase-01-loadgen-kv.md) |
| 02 | Docker Compose stack (`kv`) | 🔲 | [phase-02-docker-compose.md](phase-02-docker-compose.md) |
| 03 | Experiment runner (`run.sh`) | 🔲 | [phase-03-experiment-runner.md](phase-03-experiment-runner.md) |
| 04 | Validate & smoke scripts | 🔲 | [phase-04-scripts.md](phase-04-scripts.md) |
| 05 | CI + Makefile + README | 🔲 | [phase-05-ci-integration.md](phase-05-ci-integration.md) |

## Deliverables

```
benchmarks/loadgen-kv/            ← Phase 01
deployments/docker-compose/kv/    ← Phase 02
experiments/kv/etcd-vs-consul/    ← Phase 03
scripts/validate-etcd-vs-consul.sh ← Phase 04
scripts/smoke-etcd-vs-consul.sh    ← Phase 04
Makefile                           ← Phase 05 (add entry)
.github/workflows/ci.yaml          ← Phase 05 (add jobs)
README.md                          ← Phase 05 (update table)
```
