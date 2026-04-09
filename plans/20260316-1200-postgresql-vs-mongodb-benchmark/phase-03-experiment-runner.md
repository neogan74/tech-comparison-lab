# Phase 03 — Experiment Runner (run.sh + README)

**Parent plan:** [plan.md](./plan.md)
**Dependencies:** Phase 01 (docker-compose), Phase 02 (loadgen-db binary)
**Status:** completed
**Priority:** high
**Date:** 2026-03-16

---

## Overview

A single `run.sh` orchestrates the full benchmark end-to-end: starts the
Docker Compose stack, waits for health, runs a smoke test, executes the full
10M-document benchmark against both databases, collects storage sizes, prints
a side-by-side comparison table, and saves `results/summary.json`. The
experiment `README.md` explains how to run, interpret results, and reproduce
findings.

---

## Key Insights

- **Single entry point is the core UX requirement.** `run.sh` must handle
  the full lifecycle — no pre-steps required beyond Docker and Go being
  installed.
- **Wait strategy matters.** `docker compose up -d` returns before services
  are ready. Health-check polling loop (`until docker compose exec ...`)
  is more reliable than `sleep N`. The compose health checks defined in
  phase 01 enable `docker compose ps --format json | jq` health status checks.
- **Smoke test before full run.** A 1k-doc smoke run validates connectivity
  and schema correctness in ~10 seconds before committing to a 10M-doc run
  that could take 30+ minutes. Fail fast.
- **Storage size collection** must happen after insert completes and before
  any cleanup. The binary's `--op insert` returns storage size in its JSON
  output, so no separate query step needed if the tool is designed to emit it.
- **Side-by-side table** is the human-readable payoff. Format:
  `printf "%-15s %-10s %10s %10s %10s %15s\n"` — no dependencies.
- **results/summary.json** merges both DB results. The binary writes one JSON
  file per run; `run.sh` merges using `jq` (widely available, or bundled via
  Docker image). Alternatively the binary accepts `--append` to merge into an
  existing file — simpler.
- **Idempotency:** `run.sh` should be re-runnable. It drops and recreates
  the Docker volumes between runs (`docker compose down -v`) to ensure a clean
  baseline. This is documented prominently.
- **Build step:** `run.sh` calls `go build` at the start to ensure the binary
  is current. If Go is not installed, it falls back to a pre-built binary in
  `benchmarks/loadgen-db/bin/` (gitignored, but built by the script).

---

## Requirements

### Functional
- `./run.sh` from `experiments/databases/postgresql-vs-mongodb/` runs end-to-end
- Steps in order:
  1. Build `loadgen-db` binary
  2. `docker compose up -d` (from `deployments/docker-compose/`)
  3. Wait for postgres and mongo health (poll, max 120s, fail if exceeded)
  4. Smoke test: 1k docs insert + query + agg + update on both DBs
  5. Full run: 10M insert on both DBs (parallel or sequential)
  6. Full run: query (1000 iterations), agg (10 iterations), update (100 iterations)
  7. Collect storage sizes (embedded in binary output)
  8. Print side-by-side comparison table to stdout
  9. Save merged `results/summary.json`
- Exit code 0 on success, non-zero on any failure
- Print progress with timestamps: `[HH:MM:SS] Starting smoke test...`

### Non-Functional
- `run.sh` uses `set -euo pipefail` for safety
- All Docker Compose commands use `--project-directory` flag for portability
- No hardcoded absolute paths; use `SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)`
- `run.sh` is idempotent: clean state via `docker compose down -v` at start
  (with `--clean` flag; default preserves existing data for re-runs)

### README.md Requirements
- Prerequisites section (Docker, Go, minimum specs)
- Quick start (3 commands)
- What the benchmark measures (operations explained)
- Document schema description
- How to interpret results (p50 vs p99, throughput)
- Sample results table (placeholder with expected shape)
- Customization (env vars to override count, workers, etc.)
- Troubleshooting section

---

## Architecture

### run.sh Flow

```bash
#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/../../.." && pwd)
COMPOSE_DIR="$REPO_ROOT/deployments/docker-compose"
LOADGEN_DIR="$REPO_ROOT/benchmarks/loadgen-db"
RESULTS_DIR="$SCRIPT_DIR/results"
BINARY="$LOADGEN_DIR/bin/loadgen-db"

# Config (overridable via env)
INSERT_COUNT="${INSERT_COUNT:-10000000}"
QUERY_ITERATIONS="${QUERY_ITERATIONS:-1000}"
AGG_ITERATIONS="${AGG_ITERATIONS:-10}"
UPDATE_ITERATIONS="${UPDATE_ITERATIONS:-100}"
WORKERS="${WORKERS:-8}"
BATCH_SIZE="${BATCH_SIZE:-1000}"
SMOKE_COUNT="${SMOKE_COUNT:-1000}"

# Step 1: Build
log "Building loadgen-db..."
(cd "$LOADGEN_DIR" && go build -o bin/loadgen-db ./...)

# Step 2: Start stack
log "Starting Docker Compose stack..."
docker compose -f "$COMPOSE_DIR/docker-compose.yml" up -d

# Step 3: Wait for health
wait_for_postgres()   # polls pg_isready via docker compose exec
wait_for_mongo()      # polls mongosh ping via docker compose exec

# Step 4: Smoke test (1k docs)
smoke_test()          # runs --count 1000 --op all for both DBs, fails on error

# Step 5 & 6: Full benchmark
run_benchmark "postgres" "$PG_DSN" "$INSERT_COUNT" ...
run_benchmark "mongo"    "$MONGO_DSN" "$INSERT_COUNT" ...

# Step 7: Merge results
jq -s '{run_id: .[0].run_id, timestamp: .[0].timestamp, results: [.[].results[]]}' \
  "$RESULTS_DIR/postgres.json" "$RESULTS_DIR/mongo.json" \
  > "$RESULTS_DIR/summary.json"

# Step 8: Print table
print_comparison_table "$RESULTS_DIR/summary.json"

log "Done. Results saved to $RESULTS_DIR/summary.json"
```

### Side-by-Side Table Format

```
Operation       | PostgreSQL                    | MongoDB
                | p50     p95     p99    ops/s  | p50     p95     p99    ops/s
----------------+-------------------------------+-------------------------------
insert          | 1.2ms   2.1ms   3.8ms  8432   | 0.9ms   1.7ms   2.9ms  11203
query           | 4.3ms   7.1ms  12.4ms  231    | 3.1ms   5.8ms   9.7ms  320
agg             | 892ms   1.1s    1.4s   1.1    | 543ms   720ms   980ms  1.8
update          | 6.1ms  10.2ms  18.7ms  163    | 4.2ms   8.1ms  14.3ms  237
storage (bytes) | 4,823,412,000                 | 3,241,087,000
```

Table generated by `print_comparison_table()` bash function using `jq` +
`printf` — no Python or awk dependency.

### File Layout

```
experiments/databases/postgresql-vs-mongodb/
├── README.md                  # human guide
├── run.sh                     # executable entry point
└── results/
    ├── .gitkeep               # keeps empty dir in git
    ├── postgres.json          # per-db output (gitignored)
    ├── mongo.json             # per-db output (gitignored)
    └── summary.json           # merged final output (gitignored)
```

Add to `.gitignore` (root or experiment-level):
```
experiments/databases/postgresql-vs-mongodb/results/*.json
benchmarks/loadgen-db/bin/
```

---

## Related Code Files

- `experiments/databases/postgresql-vs-mongodb/run.sh`
- `experiments/databases/postgresql-vs-mongodb/README.md`
- `experiments/databases/postgresql-vs-mongodb/results/.gitkeep`
- `deployments/docker-compose/docker-compose.yml` (phase 01)
- `benchmarks/loadgen-db/main.go` (phase 02)

---

## Implementation Steps

1. Create `experiments/databases/postgresql-vs-mongodb/` directory
2. Create `results/` subdirectory with `.gitkeep`
3. Write `run.sh`
   - `set -euo pipefail`; path derivation via `SCRIPT_DIR`
   - `log()` helper with timestamp
   - `build_binary()`: `go build` with fallback check
   - `start_stack()`: `docker compose up -d`
   - `wait_for_postgres()`: loop with `docker compose exec postgres pg_isready`
   - `wait_for_mongo()`: loop with `docker compose exec mongo mongosh --quiet --eval "db.adminCommand('ping')"`
   - `smoke_test()`: calls binary with `--count 1000 --op all` for each DB; fails loudly
   - `run_full()`: calls binary for each op; captures per-db JSON
   - `merge_results()`: `jq -s` merge
   - `print_table()`: `jq` extraction + `printf` formatting
4. `chmod +x run.sh`
5. Write `README.md`
   - Prerequisites, Quick Start, Operations description, Schema description,
     Results interpretation, Customization env vars, Troubleshooting
6. Add `.gitignore` entries for results JSON and binary
7. Validate `bash -n run.sh` (syntax check)
8. Dry-run test: `--smoke-only` flag that skips full benchmark (useful in CI)

---

## Todo

- [x] Create directory structure with `.gitkeep`
- [x] Write `run.sh` with all helper functions
- [x] `chmod +x run.sh`
- [x] Write `README.md` with all required sections
- [x] Add `.gitignore` entries for results and binary
- [x] Validate with `bash -n run.sh`
- [x] Test `wait_for_*` functions against running stack
- [x] Test `smoke_test()` catches a failure (kill postgres mid-smoke)
- [x] Verify `print_table()` output aligns correctly for various value widths

---

## Success Criteria

- `bash -n run.sh` exits 0 (syntax valid)
- Full run completes without manual intervention on a clean machine with Docker + Go
- `results/summary.json` is valid JSON with 8 result entries (4 ops × 2 DBs)
- Each result entry contains: db, op, count, p50_ms, p95_ms, p99_ms, throughput_ops_sec, storage_bytes (for insert)
- Side-by-side table prints to stdout with aligned columns
- Re-running `run.sh` after first completion produces fresh results (idempotent via `down -v`)

---

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| `jq` not installed on host | medium | medium | Check for `jq` at script start; print install instructions; alternatively use Python fallback for JSON merge |
| 10M insert takes >60min causing CI timeout | medium | high | Make `INSERT_COUNT` env-overridable; default to 1M for CI; document 10M as "full benchmark" |
| Mongo health check via `mongosh` not available in older images | low | medium | Use `mongo:7.0` which ships `mongosh`; or use `mongostat --rowcount 1` as fallback |
| Parallel insert for both DBs overloads developer machine | medium | medium | Default to sequential; add `--parallel` flag as opt-in with resource warning |
| Results directory write permissions | low | low | `mkdir -p` before writes; check exit code |
| go build fails due to network (module download) | medium | low | Pre-download modules in `go mod download` step; document proxy setting |

---

## Security Considerations

- `run.sh` does not eval user input; all variables are controlled by env or
  script internals
- `docker compose exec` commands are non-interactive (`-T` flag) to prevent
  TTY injection
- DSNs constructed from env vars with documented safe defaults; never logged
  with credentials in plain text (mask password in log output)
- Results JSON may contain timing data; no credentials or PII in output

---

## Next Steps

After phase 03 is complete:
- Tag repository with `experiment/01-postgresql-vs-mongodb-v1`
- Capture actual results in a committed `results/summary-YYYYMMDD.json` (named
  with date to distinguish from ephemeral run output)
- Follow-up experiments can reuse the Docker Compose base stack by adding
  services to `docker-compose.yml`
- CI integration: run smoke test only (`SMOKE_ONLY=1 ./run.sh`) on PRs
