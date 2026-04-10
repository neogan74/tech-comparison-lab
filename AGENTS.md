# Tech Comparison Lab — Agent Instructions

## Repository Structure

- `experiments/` — Technology comparison experiments (each has `run.sh` and `README.md`)
- `benchmarks/` — Go load generators for specific domains (db, http, cache, k8s, etc.)
- `apps/bench-api` — Go test server (REST + gRPC) for HTTP benchmarks
- `deployments/docker-compose/` — Infrastructure (PostgreSQL, MongoDB, Redis, Prometheus, Grafana)
- `scripts/` — CI validation and smoke test scripts

## Running Experiments

Each experiment directory contains a `run.sh` script with consistent flags:
- `--smoke-only` — Quick test (~1k docs or requests, ~30 sec)
- `--clean` — Remove Docker volumes before starting (database experiments)

```bash
cd experiments/databases/postgresql-vs-mongodb
./run.sh --smoke-only    # Quick validation
./run.sh --clean          # Full benchmark (10M docs default)
```

Environment variables for tuning:
- `INSERT_COUNT` / `COUNT` — Dataset size (default varies by experiment)
- `WORKERS` — Concurrency
- `SKIP_BUILD=1` — Use existing binary instead of rebuilding
- `SMOKE_COUNT` — Override smoke test size

## Validation

CI runs validation scripts in `scripts/`:
1. `bash -n` — Syntax check on experiment runners
2. `go test` — Unit tests in benchmark packages
3. End-to-end smoke test

Run locally:
```bash
bash ./scripts/validate-postgresql-vs-mongodb.sh
bash ./scripts/smoke-postgresql-vs-mongodb.sh
```

## Build Patterns

- Go 1.23+ required
- Each benchmark has its own Go module (`benchmarks/*/go.mod`)
- Binary output:
  - `benchmarks/loadgen-db/bin/loadgen-db`
  - `benchmarks/loadgen-http/loadgen-http`
  - `benchmarks/loadgen-k8s/loadgen-k8s`
  - `apps/bench-api/bench-api`

If `go` unavailable but binary exists, `run.sh` falls back to it.

## Infrastructure

Docker Compose v2 required (`docker compose`, not `docker-compose`).
Stack managed in `deployments/docker-compose/docker-compose.yml`:
- `postgres` :5432, `mongo` :27017, `redis` :6379, `rabbitmq` :5672
- `prometheus` :9090, `grafana` :3000

Use `.env.example` in `deployments/docker-compose/` to override ports/passwords.

## Testing

```bash
cd benchmarks/loadgen-db      # or any loadgen-* directory
go test ./...
```

Tests are unit-level; integration happens via smoke tests in `scripts/`.

## Status Matrix

Check `README.md` for experiment status:
- **smoke-tested** — Has CI validation and passing smoke test
- **in progress** — Implementation exists but no CI yet

When adding new experiments, follow the pattern of `postgresql-vs-mongodb`:
1. Add benchmark in `benchmarks/loadgen-*/`
2. Add experiment in `experiments/category/name/` with `run.sh`
3. Add validation script in `scripts/validate-*.sh`
4. Add smoke test in `scripts/smoke-*.sh`
5. Update `.github/workflows/ci.yaml` with new jobs
