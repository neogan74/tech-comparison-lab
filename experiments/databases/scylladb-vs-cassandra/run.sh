#!/usr/bin/env bash
# Experiment: ScyllaDB vs Cassandra distributed NoSQL workload benchmark
# Usage: ./run.sh [--clean] [--smoke-only]

set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/../../.." && pwd)
COMPOSE_DIR="$REPO_ROOT/deployments/docker-compose/scylladb-vs-cassandra"
LOADGEN_DIR="$REPO_ROOT/benchmarks/loadgen-db"
RESULTS_DIR="$SCRIPT_DIR/results"
BINARY="$LOADGEN_DIR/bin/loadgen-db"

INSERT_COUNT="${INSERT_COUNT:-1000000}"
QUERY_ITERATIONS="${QUERY_ITERATIONS:-1000}"
AGG_ITERATIONS="${AGG_ITERATIONS:-1}"
UPDATE_ITERATIONS="${UPDATE_ITERATIONS:-10}"
WORKERS="${WORKERS:-16}"
BATCH_SIZE="${BATCH_SIZE:-100}"
SMOKE_COUNT="${SMOKE_COUNT:-500}"
SMOKE_ONLY="${SMOKE_ONLY:-false}"
SKIP_BUILD="${SKIP_BUILD:-false}"

CLEAN=false
for arg in "$@"; do
  case $arg in
    --clean) CLEAN=true ;;
    --smoke-only) SMOKE_ONLY=true ;;
    -h|--help)
      cat <<'EOF'
Usage: ./run.sh [--clean] [--smoke-only]

Options:
  --clean       remove benchmark volumes before starting the stack
  --smoke-only  run the small smoke benchmark only

Environment overrides:
  INSERT_COUNT, QUERY_ITERATIONS, AGG_ITERATIONS, UPDATE_ITERATIONS
  WORKERS, BATCH_SIZE, SMOKE_COUNT
  CASSANDRA_PORT, SCYLLA_PORT
  SMOKE_ONLY=true
  SKIP_BUILD=true   use benchmarks/loadgen-db/bin/loadgen-db
EOF
      exit 0
      ;;
    *) echo "error: unknown argument '$arg'" >&2; exit 1 ;;
  esac
done

log() { echo "[$(date '+%H:%M:%S')] $*"; }
have_cmd() { command -v "$1" >/dev/null 2>&1; }

check_deps() {
  for cmd in docker jq; do
    have_cmd "$cmd" || { echo "error: '$cmd' not found." >&2; exit 1; }
  done
  docker compose version >/dev/null 2>&1 || { echo "error: Docker Compose v2 is required." >&2; exit 1; }
  docker info >/dev/null 2>&1 || { echo "error: Docker daemon is not running." >&2; exit 1; }
}

load_env() {
  CASSANDRA_PORT="${CASSANDRA_PORT:-9042}"
  SCYLLA_PORT="${SCYLLA_PORT:-9043}"
  CASSANDRA_DSN="localhost:${CASSANDRA_PORT}"
  SCYLLA_DSN="localhost:${SCYLLA_PORT}"
}

build_binary() {
  if [ "$SKIP_BUILD" = true ]; then
    [ -x "$BINARY" ] || { echo "error: binary is missing: $BINARY" >&2; exit 1; }
    log "Using existing binary: $BINARY"
  elif have_cmd go; then
    log "Building loadgen-db..."
    mkdir -p "$LOADGEN_DIR/bin"
    (cd "$LOADGEN_DIR" && go build -o "$BINARY" .)
  elif [ ! -x "$BINARY" ]; then
    echo "error: 'go' not found and no prebuilt binary exists at $BINARY" >&2
    exit 1
  fi
}

start_stack() {
  if [ "$CLEAN" = true ]; then
    log "Removing existing benchmark volumes..."
    docker compose -f "$COMPOSE_DIR/docker-compose.yml" down -v --remove-orphans >/dev/null 2>&1 || true
  fi
  log "Starting Cassandra and ScyllaDB..."
  docker compose -f "$COMPOSE_DIR/docker-compose.yml" up -d cassandra scylladb
}

wait_for_cassandra() {
  log "Waiting for Cassandra (first startup can take several minutes)..."
  local i=0
  until docker compose -f "$COMPOSE_DIR/docker-compose.yml" exec -T cassandra \
      cqlsh -e "SELECT release_version FROM system.local" >/dev/null 2>&1; do
    i=$((i+1))
    if [ "$i" -ge 60 ]; then
      docker compose -f "$COMPOSE_DIR/docker-compose.yml" logs --tail=80 cassandra >&2 || true
      echo "error: Cassandra did not become ready in 300s" >&2
      exit 1
    fi
    sleep 5
  done
  docker compose -f "$COMPOSE_DIR/docker-compose.yml" exec -T cassandra \
    cqlsh -f /schema/init.cql
  log "Cassandra schema ready."
}

wait_for_scylladb() {
  log "Waiting for ScyllaDB..."
  local i=0
  until docker compose -f "$COMPOSE_DIR/docker-compose.yml" exec -T scylladb \
      cqlsh -e "SELECT release_version FROM system.local" >/dev/null 2>&1; do
    i=$((i+1))
    if [ "$i" -ge 60 ]; then
      docker compose -f "$COMPOSE_DIR/docker-compose.yml" logs --tail=80 scylladb >&2 || true
      echo "error: ScyllaDB did not become ready in 300s" >&2
      exit 1
    fi
    sleep 5
  done
  docker compose -f "$COMPOSE_DIR/docker-compose.yml" exec -T scylladb \
    cqlsh -f /schema/init.cql
  log "ScyllaDB schema ready."
}

run_target() {
  local db=$1 count=$2 out=${3:-}
  local dsn=$CASSANDRA_DSN
  [ "$db" = scylladb ] && dsn=$SCYLLA_DSN
  local args=(--db "$db" --op all --count "$count" --batch "$BATCH_SIZE" --workers "$WORKERS" --truncate --dsn "$dsn")
  if [ -n "$out" ]; then
    args+=(--query-iter "$QUERY_ITERATIONS" --agg-iter "$AGG_ITERATIONS" --update-iter "$UPDATE_ITERATIONS" --out "$out")
  else
    args+=(--query-iter 10 --agg-iter 1 --update-iter 1)
  fi
  "$BINARY" "${args[@]}"
}

smoke_test() {
  log "--- Smoke test (${SMOKE_COUNT} documents per database) ---"
  run_target cassandra "$SMOKE_COUNT"
  run_target scylladb "$SMOKE_COUNT"
  log "Smoke test passed."
}

run_full() {
  mkdir -p "$RESULTS_DIR"
  log "Running Cassandra benchmark..."
  run_target cassandra "$INSERT_COUNT" "$RESULTS_DIR/cassandra.json"
  log "Running ScyllaDB benchmark..."
  run_target scylladb "$INSERT_COUNT" "$RESULTS_DIR/scylladb.json"
}

merge_results() {
  jq -s '{
    schema_version: "results-summary/v1",
    experiment: {
      id: "scylladb-vs-cassandra",
      name: "ScyllaDB vs Cassandra",
      category: "databases",
      path: "experiments/databases/scylladb-vs-cassandra"
    },
    run_id: .[0].run_id,
    timestamp: .[0].timestamp,
    mode: "full",
    config: {
      insert_count: '"$INSERT_COUNT"',
      query_iterations: '"$QUERY_ITERATIONS"',
      agg_iterations: '"$AGG_ITERATIONS"',
      update_iterations: '"$UPDATE_ITERATIONS"',
      workers: '"$WORKERS"',
      batch_size: '"$BATCH_SIZE"',
      buckets: 32
    },
    sources: [
      {name: "cassandra", file: "results/cassandra.json"},
      {name: "scylladb", file: "results/scylladb.json"}
    ],
    results: [.[].results[]]
  }' "$RESULTS_DIR/cassandra.json" "$RESULTS_DIR/scylladb.json" > "$RESULTS_DIR/summary.json"
}

verify_results() {
  for path in "$RESULTS_DIR/cassandra.json" "$RESULTS_DIR/scylladb.json" "$RESULTS_DIR/summary.json"; do
    [ -s "$path" ] || { echo "error: missing result: $path" >&2; exit 1; }
  done
}

print_table() {
  printf "\n%-12s %-10s %10s %10s %10s %12s\n" "DB" "Operation" "p50(ms)" "p95(ms)" "p99(ms)" "ops/s"
  jq -r '.results[] | [.db,.op,.p50_ms,.p95_ms,.p99_ms,(.throughput_ops_sec|floor)] | @tsv' "$RESULTS_DIR/summary.json" |
    while IFS=$'\t' read -r db op p50 p95 p99 ops; do
      printf "%-12s %-10s %10s %10s %10s %12s\n" "$db" "$op" "$p50" "$p95" "$p99" "$ops"
    done
}

check_deps
load_env
build_binary
start_stack
wait_for_cassandra
wait_for_scylladb

if [ "$SMOKE_ONLY" = true ]; then
  smoke_test
  exit 0
fi

smoke_test
run_full
merge_results
verify_results
print_table
log "Done. Results: $RESULTS_DIR/summary.json"
