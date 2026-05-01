#!/usr/bin/env bash
# Experiment: MySQL vs PostgreSQL OLTP workload benchmark
# Usage: ./run.sh [--clean] [--smoke-only]
#
# Environment overrides:
#   INSERT_COUNT        (default: 10000000)
#   QUERY_ITERATIONS    (default: 1000)
#   AGG_ITERATIONS      (default: 10)
#   UPDATE_ITERATIONS   (default: 100)
#   WORKERS             (default: 8)
#   BATCH_SIZE          (default: 1000)
#   MYSQL_PASSWORD      (default: benchpass)
#   POSTGRES_PASSWORD   (default: benchpass)

set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/../../.." && pwd)
COMPOSE_DIR="$REPO_ROOT/deployments/docker-compose/mysql-vs-postgres"
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
SMOKE_ONLY="${SMOKE_ONLY:-false}"
SKIP_BUILD="${SKIP_BUILD:-false}"

CLEAN=false
for arg in "$@"; do
  case $arg in
    --clean)       CLEAN=true ;;
    --smoke-only)  SMOKE_ONLY=true ;;
    -h|--help)
      cat <<'EOF'
Usage: ./run.sh [--clean] [--smoke-only]

Options:
  --clean       remove benchmark volumes before starting the stack
  --smoke-only  run the 1k-document smoke benchmark only

Environment overrides:
  INSERT_COUNT, QUERY_ITERATIONS, AGG_ITERATIONS, UPDATE_ITERATIONS
  WORKERS, BATCH_SIZE, SMOKE_COUNT
  MYSQL_PASSWORD, POSTGRES_PASSWORD
  SMOKE_ONLY=true
  SKIP_BUILD=true   use existing binary at benchmarks/loadgen-db/bin/loadgen-db
EOF
      exit 0
      ;;
    *)
      echo "error: unknown argument '$arg'" >&2
      exit 1
      ;;
  esac
done

# --- Helpers ---
log() { echo "[$(date '+%H:%M:%S')] $*"; }

have_cmd() { command -v "$1" >/dev/null 2>&1; }

check_deps() {
  for cmd in docker jq; do
    if ! command -v "$cmd" &>/dev/null; then
      echo "error: '$cmd' not found. Install it and re-run." >&2
      exit 1
    fi
  done
  if ! docker compose version &>/dev/null; then
    echo "error: Docker Compose v2 is required." >&2
    exit 1
  fi
  if ! docker info &>/dev/null; then
    echo "error: Docker daemon is not running." >&2; exit 1
  fi
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
    log "Building loadgen-db..."
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
  log "Starting Docker Compose stack..."
  if [ "$CLEAN" = true ]; then
    log "  --clean: removing existing volumes"
    docker compose -f "$COMPOSE_DIR/docker-compose.yml" down -v --remove-orphans 2>/dev/null || true
  fi
  docker compose -f "$COMPOSE_DIR/docker-compose.yml" up -d
}

wait_for_mysql() {
  log "Waiting for MySQL..."
  local max=48 i=0  # 48 × 5s = 240s
  until docker compose -f "$COMPOSE_DIR/docker-compose.yml" exec -T mysql \
      mysqladmin ping -h 127.0.0.1 -u bench \
      --password="${MYSQL_PASSWORD:-benchpass}" --silent &>/dev/null; do
    i=$((i+1))
    if [ $i -ge $max ]; then
      echo "error: MySQL did not become ready in 240s" >&2; exit 1
    fi
    sleep 5
  done
  log "MySQL ready."
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

load_env() {
  MYSQL_PASSWORD="${MYSQL_PASSWORD:-benchpass}"
  POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-benchpass}"
  MYSQL_DSN="bench:${MYSQL_PASSWORD}@tcp(localhost:3306)/bench?parseTime=true&charset=utf8mb4"
  PG_DSN="postgres://bench:${POSTGRES_PASSWORD}@localhost:5432/bench?sslmode=disable"
}

smoke_test() {
  log "--- Smoke test (${SMOKE_COUNT} docs each DB) ---"
  "$BINARY" --db mysql --op all --count "$SMOKE_COUNT" \
    --batch 100 --workers 4 --truncate --dsn "$MYSQL_DSN"
  "$BINARY" --db postgres --op all --count "$SMOKE_COUNT" \
    --batch 100 --workers 4 --truncate --dsn "$PG_DSN"
  log "Smoke test passed."
}

run_full() {
  log "--- Full benchmark: INSERT_COUNT=$INSERT_COUNT ---"
  mkdir -p "$RESULTS_DIR"

  log "Running MySQL benchmark..."
  "$BINARY" \
    --db          mysql \
    --op          all \
    --count       "$INSERT_COUNT" \
    --query-iter  "$QUERY_ITERATIONS" \
    --agg-iter    "$AGG_ITERATIONS" \
    --update-iter "$UPDATE_ITERATIONS" \
    --batch       "$BATCH_SIZE" \
    --workers     "$WORKERS" \
    --truncate \
    --dsn         "$MYSQL_DSN" \
    --out         "$RESULTS_DIR/mysql.json"

  log "Running PostgreSQL benchmark..."
  "$BINARY" \
    --db          postgres \
    --op          all \
    --count       "$INSERT_COUNT" \
    --query-iter  "$QUERY_ITERATIONS" \
    --agg-iter    "$AGG_ITERATIONS" \
    --update-iter "$UPDATE_ITERATIONS" \
    --batch       "$BATCH_SIZE" \
    --workers     "$WORKERS" \
    --truncate \
    --dsn         "$PG_DSN" \
    --out         "$RESULTS_DIR/postgres.json"
}

merge_results() {
  log "Merging results..."
  jq -s '{
    schema_version: "results-summary/v1",
    experiment: {
      id: "mysql-vs-postgres",
      name: "MySQL vs PostgreSQL",
      category: "databases",
      path: "experiments/databases/mysql-vs-postgres"
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
      batch_size: '"$BATCH_SIZE"'
    },
    sources: [
      {name: "mysql", file: "results/mysql.json"},
      {name: "postgres", file: "results/postgres.json"}
    ],
    results: [.[].results[]]
  }' "$RESULTS_DIR/mysql.json" "$RESULTS_DIR/postgres.json" \
    > "$RESULTS_DIR/summary.json"
}

verify_results() {
  for path in \
    "$RESULTS_DIR/mysql.json" \
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
wait_for_mysql
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
log "Grafana:    http://localhost:3000"
log "Prometheus: http://localhost:9090"