#!/usr/bin/env bash
# Experiment #4: ClickHouse vs PostgreSQL analytics benchmark
# Usage: ./run.sh [--clean] [--smoke-only]
#
# Environment overrides:
#   ROW_COUNT       (default: 10000000  — 10M rows)
#   BATCH_SIZE      (default: 100000)
#   WORKERS         (default: 4)
#   QUERY_ITER      (default: 5)

set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/../../.." && pwd)
COMPOSE_DIR="$REPO_ROOT/deployments/docker-compose/analytics"
LOADGEN_DIR="$REPO_ROOT/benchmarks/loadgen-analytics"
RESULTS_DIR="$SCRIPT_DIR/results"
BINARY="$LOADGEN_DIR/bin/loadgen-analytics"

ROW_COUNT="${ROW_COUNT:-10000000}"
BATCH_SIZE="${BATCH_SIZE:-100000}"
WORKERS="${WORKERS:-4}"
QUERY_ITER="${QUERY_ITER:-5}"
SMOKE_COUNT="${SMOKE_COUNT:-100000}"
SMOKE_ITER="${SMOKE_ITER:-2}"
SKIP_BUILD="${SKIP_BUILD:-false}"

CH_ADDR="localhost:9000"
PG_DSN="postgres://bench:benchpass@localhost:5433/bench?sslmode=disable"

CLEAN=false
SMOKE_ONLY=false
for arg in "$@"; do
  case $arg in
    --clean)      CLEAN=true ;;
    --smoke-only) SMOKE_ONLY=true ;;
    -h|--help)
      cat <<'EOF'
Usage: ./run.sh [--clean] [--smoke-only]

Options:
  --clean       remove existing volumes before starting the stack
  --smoke-only  run the smoke benchmark only

Environment overrides:
  ROW_COUNT
  BATCH_SIZE
  WORKERS
  QUERY_ITER
  SMOKE_COUNT
  SMOKE_ITER
  SKIP_BUILD=true
EOF
      exit 0
      ;;
    *)
      echo "error: unknown argument '$arg'" >&2
      exit 1
      ;;
  esac
done

log() { echo "[$(date '+%H:%M:%S')] $*"; }
have_cmd() { command -v "$1" >/dev/null 2>&1; }

check_deps() {
  for cmd in docker jq; do
    if ! command -v "$cmd" &>/dev/null; then
      echo "error: '$cmd' not found." >&2; exit 1
    fi
  done
  if ! docker compose version &>/dev/null; then
    echo "error: Docker Compose v2 is required." >&2; exit 1
  fi
  docker info &>/dev/null || { echo "error: Docker not running." >&2; exit 1; }
}

build_binary() {
  if [ "$SKIP_BUILD" = true ]; then
    if [ ! -x "$BINARY" ]; then
      echo "error: SKIP_BUILD=true but binary is missing: $BINARY" >&2
      exit 1
    fi
    log "Using existing binary: $BINARY"
    return
  fi

  if have_cmd go; then
    log "Building loadgen-analytics..."
    mkdir -p "$LOADGEN_DIR/bin"
    (cd "$LOADGEN_DIR" && go build -o "$BINARY" .)
    log "Binary: $BINARY"
    return
  fi

  if [ -x "$BINARY" ]; then
    log "Go not found; using existing binary: $BINARY"
    return
  fi

  echo "error: 'go' not found and no prebuilt binary is available at $BINARY" >&2
  exit 1
}

start_stack() {
  log "Starting Docker Compose stack (analytics)..."
  if [ "$CLEAN" = true ]; then
    docker compose -f "$COMPOSE_DIR/docker-compose.yml" down -v --remove-orphans 2>/dev/null || true
  fi
  docker compose -f "$COMPOSE_DIR/docker-compose.yml" up -d
}

wait_for_clickhouse() {
  log "Waiting for ClickHouse (port 9000)..."
  local max=24 i=0
  until docker compose -f "$COMPOSE_DIR/docker-compose.yml" exec -T clickhouse \
      clickhouse-client --query "SELECT 1" &>/dev/null; do
    i=$((i+1))
    [ $i -ge $max ] && { echo "error: ClickHouse not ready after 120s" >&2; exit 1; }
    sleep 5
  done
  log "ClickHouse ready."
}

wait_for_postgres() {
  log "Waiting for PostgreSQL (port 5433)..."
  local max=24 i=0
  until docker compose -f "$COMPOSE_DIR/docker-compose.yml" exec -T postgres \
      pg_isready -U bench -d bench &>/dev/null; do
    i=$((i+1))
    [ $i -ge $max ] && { echo "error: PostgreSQL not ready after 120s" >&2; exit 1; }
    sleep 5
  done
  log "PostgreSQL ready."
}

smoke_test() {
  log "--- Smoke test (${SMOKE_COUNT} rows, ${SMOKE_ITER} query iters) ---"
  "$BINARY" --db clickhouse --op all --count "$SMOKE_COUNT" --batch 10000 \
    --workers 2 --query-iter "$SMOKE_ITER" --truncate --addr "$CH_ADDR"
  "$BINARY" --db postgres   --op all --count "$SMOKE_COUNT" --batch 10000 \
    --workers 2 --query-iter "$SMOKE_ITER" --truncate --addr "$PG_DSN"
  log "Smoke test passed."
}

run_full() {
  log "--- Full benchmark: ROW_COUNT=$ROW_COUNT ---"
  mkdir -p "$RESULTS_DIR"

  log "Running ClickHouse benchmark..."
  "$BINARY" \
    --db         clickhouse \
    --op         all \
    --count      "$ROW_COUNT" \
    --batch      "$BATCH_SIZE" \
    --workers    "$WORKERS" \
    --query-iter "$QUERY_ITER" \
    --truncate \
    --addr       "$CH_ADDR" \
    --out        "$RESULTS_DIR/clickhouse.json"

  log "Running PostgreSQL benchmark..."
  "$BINARY" \
    --db         postgres \
    --op         all \
    --count      "$ROW_COUNT" \
    --batch      "$BATCH_SIZE" \
    --workers    "$WORKERS" \
    --query-iter "$QUERY_ITER" \
    --truncate \
    --addr       "$PG_DSN" \
    --out        "$RESULTS_DIR/postgres.json"
}

merge_results() {
  log "Merging results..."
  jq -s '{
    schema_version: "results-summary/v1",
    experiment: {
      id: "clickhouse-vs-postgresql",
      name: "ClickHouse vs PostgreSQL",
      category: "analytics",
      path: "experiments/analytics/clickhouse-vs-postgresql"
    },
    run_id: .[0].run_id,
    timestamp: .[0].timestamp,
    mode: "full",
    config: {
      row_count: '"$ROW_COUNT"',
      batch_size: '"$BATCH_SIZE"',
      workers: '"$WORKERS"',
      query_iter: '"$QUERY_ITER"'
    },
    sources: [
      {name: "clickhouse", file: "results/clickhouse.json"},
      {name: "postgres", file: "results/postgres.json"}
    ],
    results: [.[].results[]]
  }' "$RESULTS_DIR/clickhouse.json" "$RESULTS_DIR/postgres.json" \
    > "$RESULTS_DIR/summary.json"
}

verify_results() {
  for path in \
    "$RESULTS_DIR/clickhouse.json" \
    "$RESULTS_DIR/postgres.json" \
    "$RESULTS_DIR/summary.json"; do
    if [ ! -s "$path" ]; then
      echo "error: expected results file is missing or empty: $path" >&2
      exit 1
    fi
  done
}

print_table() {
  log "--- Side-by-side comparison ---"
  echo ""
  printf "%-14s %-16s %10s %10s %10s %12s %10s\n" \
    "DB" "Operation" "rows" "p50(ms)" "p95(ms)" "p99(ms)" "Storage"
  printf "%-14s %-16s %10s %10s %10s %12s %10s\n" \
    "--------------" "----------------" "----------" "--------" "--------" "------------" "--------"
  jq -r '.results[] | [
    .db, .op, (.count|tostring),
    (.p50_ms|tostring), (.p95_ms|tostring), (.p99_ms|tostring),
    (if .storage_bytes > 0 then (.storage_bytes/1e9|.*10|floor/10|tostring)+"GB" else "-" end)
  ] | @tsv' "$RESULTS_DIR/summary.json" | \
  while IFS=$'\t' read -r db op cnt p50 p95 p99 storage; do
    printf "%-14s %-16s %10s %10s %10s %12s %10s\n" \
      "$db" "$op" "$cnt" "$p50" "$p95" "$p99" "$storage"
  done
  echo ""
}

# --- Main ---
check_deps
build_binary
start_stack
wait_for_clickhouse
wait_for_postgres

if [ "$SMOKE_ONLY" = true ]; then
  smoke_test
  log "Smoke-only mode: done."
  exit 0
fi

smoke_test
run_full
merge_results
verify_results
print_table

log "Done. Results: $RESULTS_DIR/summary.json"
log "ClickHouse HTTP: http://localhost:8123/play"
log "Prometheus:      http://localhost:9095"
log "Grafana:         http://localhost:3003"
