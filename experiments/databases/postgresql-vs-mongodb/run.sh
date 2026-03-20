#!/usr/bin/env bash
# Experiment #1: PostgreSQL vs MongoDB JSON workload benchmark
# Usage: ./run.sh [--clean] [--smoke-only]
#
# Environment overrides:
#   INSERT_COUNT        (default: 10000000)
#   QUERY_ITERATIONS    (default: 1000)
#   AGG_ITERATIONS      (default: 10)
#   UPDATE_ITERATIONS   (default: 100)
#   WORKERS             (default: 8)
#   BATCH_SIZE          (default: 1000)
#   POSTGRES_PASSWORD   (default: benchpass)
#   MONGO_PASSWORD      (default: benchpass)

set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/../../.." && pwd)
COMPOSE_DIR="$REPO_ROOT/deployments/docker-compose"
LOADGEN_DIR="$REPO_ROOT/benchmarks/loadgen-db"
RESULTS_DIR="$SCRIPT_DIR/results"
BINARY="$LOADGEN_DIR/bin/loadgen-db"

# --- Config (env-overridable) ---
INSERT_COUNT="${INSERT_COUNT:-10000000}"
QUERY_ITERATIONS="${QUERY_ITERATIONS:-1000}"
AGG_ITERATIONS="${AGG_ITERATIONS:-10}"
UPDATE_ITERATIONS="${UPDATE_ITERATIONS:-100}"
WORKERS="${WORKERS:-8}"
BATCH_SIZE="${BATCH_SIZE:-1000}"
SMOKE_COUNT="${SMOKE_COUNT:-1000}"

CLEAN=false
SMOKE_ONLY=false
for arg in "$@"; do
  case $arg in
    --clean)       CLEAN=true ;;
    --smoke-only)  SMOKE_ONLY=true ;;
  esac
done

# --- Helpers ---
log() { echo "[$(date '+%H:%M:%S')] $*"; }

check_deps() {
  for cmd in docker go jq; do
    if ! command -v "$cmd" &>/dev/null; then
      echo "error: '$cmd' not found. Install it and re-run." >&2
      exit 1
    fi
  done
  if ! docker info &>/dev/null; then
    echo "error: Docker daemon is not running." >&2; exit 1
  fi
}

build_binary() {
  log "Building loadgen-db..."
  mkdir -p "$LOADGEN_DIR/bin"
  (cd "$LOADGEN_DIR" && go mod tidy && go build -o "$BINARY" .)
  log "Binary: $BINARY"
}

start_stack() {
  log "Starting Docker Compose stack..."
  if [ "$CLEAN" = true ]; then
    log "  --clean: removing existing volumes"
    docker compose -f "$COMPOSE_DIR/docker-compose.yml" down -v --remove-orphans 2>/dev/null || true
  fi
  docker compose -f "$COMPOSE_DIR/docker-compose.yml" up -d
}

wait_for_postgres() {
  log "Waiting for PostgreSQL..."
  local max=24 i=0  # 24 × 5s = 120s
  until docker compose -f "$COMPOSE_DIR/docker-compose.yml" exec -T postgres \
      pg_isready -U bench -d bench &>/dev/null; do
    i=$((i+1))
    if [ $i -ge $max ]; then
      echo "error: PostgreSQL did not become ready in 120s" >&2; exit 1
    fi
    sleep 5
  done
  log "PostgreSQL ready."
}

wait_for_mongo() {
  log "Waiting for MongoDB..."
  local max=24 i=0
  until docker compose -f "$COMPOSE_DIR/docker-compose.yml" exec -T mongo \
      mongosh --quiet \
      --username bench --password "${MONGO_PASSWORD:-benchpass}" \
      --authenticationDatabase admin \
      --eval "db.adminCommand('ping').ok" &>/dev/null; do
    i=$((i+1))
    if [ $i -ge $max ]; then
      echo "error: MongoDB did not become ready in 120s" >&2; exit 1
    fi
    sleep 5
  done
  log "MongoDB ready."
}

load_env() {
  # Source .env if present (docker-compose uses it too)
  if [ -f "$COMPOSE_DIR/.env" ]; then
    set -a; source "$COMPOSE_DIR/.env"; set +a
  fi
  POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-benchpass}"
  MONGO_PASSWORD="${MONGO_PASSWORD:-benchpass}"
  PG_DSN="postgres://bench:${POSTGRES_PASSWORD}@localhost:5432/bench?sslmode=disable"
  MONGO_DSN="mongodb://bench:${MONGO_PASSWORD}@localhost:27017/?authSource=admin"
}

smoke_test() {
  log "--- Smoke test (${SMOKE_COUNT} docs each DB) ---"
  "$BINARY" --db postgres --op all --count "$SMOKE_COUNT" \
    --batch 100 --workers 4 --truncate --dsn "$PG_DSN"
  "$BINARY" --db mongo --op all --count "$SMOKE_COUNT" \
    --batch 100 --workers 4 --truncate --dsn "$MONGO_DSN"
  log "Smoke test passed."
}

run_full() {
  log "--- Full benchmark: INSERT_COUNT=$INSERT_COUNT ---"
  mkdir -p "$RESULTS_DIR"

  log "Running PostgreSQL benchmark..."
  "$BINARY" \
    --db         postgres \
    --op         all \
    --count      "$INSERT_COUNT" \
    --query-iter "$QUERY_ITERATIONS" \
    --agg-iter   "$AGG_ITERATIONS" \
    --update-iter "$UPDATE_ITERATIONS" \
    --batch      "$BATCH_SIZE" \
    --workers    "$WORKERS" \
    --truncate \
    --dsn        "$PG_DSN" \
    --out        "$RESULTS_DIR/postgres.json"

  log "Running MongoDB benchmark..."
  "$BINARY" \
    --db         mongo \
    --op         all \
    --count      "$INSERT_COUNT" \
    --query-iter "$QUERY_ITERATIONS" \
    --agg-iter   "$AGG_ITERATIONS" \
    --update-iter "$UPDATE_ITERATIONS" \
    --batch      "$BATCH_SIZE" \
    --workers    "$WORKERS" \
    --truncate \
    --dsn        "$MONGO_DSN" \
    --out        "$RESULTS_DIR/mongo.json"
}

merge_results() {
  log "Merging results..."
  jq -s '{
    run_id: .[0].run_id,
    timestamp: .[0].timestamp,
    insert_count: '"$INSERT_COUNT"',
    results: [.[].results[]]
  }' "$RESULTS_DIR/postgres.json" "$RESULTS_DIR/mongo.json" \
    > "$RESULTS_DIR/summary.json"
}

print_table() {
  log "--- Side-by-side comparison ---"
  echo ""
  printf "%-10s %-12s %10s %10s %10s %12s %12s\n" \
    "DB" "Operation" "p50(ms)" "p95(ms)" "p99(ms)" "ops/s" "Storage"
  printf "%-10s %-12s %10s %10s %10s %12s %12s\n" \
    "----------" "------------" "--------" "--------" "--------" "----------" "----------"
  jq -r '.results[] | [
    .db, .op,
    (.p50_ms|tostring),
    (.p95_ms|tostring),
    (.p99_ms|tostring),
    (.throughput_ops_sec|floor|tostring),
    (if .storage_bytes > 0 then (.storage_bytes/1073741824|.*100|floor/100|tostring)+"GB" else "-" end)
  ] | @tsv' "$RESULTS_DIR/summary.json" | \
  while IFS=$'\t' read -r db op p50 p95 p99 ops storage; do
    printf "%-10s %-12s %10s %10s %10s %12s %12s\n" \
      "$db" "$op" "$p50" "$p95" "$p99" "$ops" "$storage"
  done
  echo ""
}

# --- Main ---
check_deps
load_env
build_binary
start_stack
wait_for_postgres
wait_for_mongo

if [ "$SMOKE_ONLY" = true ]; then
  smoke_test
  log "Smoke-only mode: done."
  exit 0
fi

smoke_test
run_full
merge_results
print_table

log "Done. Results: $RESULTS_DIR/summary.json"
log "Grafana:    http://localhost:3000"
log "Prometheus: http://localhost:9090"
