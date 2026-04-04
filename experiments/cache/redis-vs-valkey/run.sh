#!/usr/bin/env bash
# Experiment #5: Redis vs Valkey cache benchmark
# Usage: ./run.sh [--clean] [--smoke-only]
#
# Environment overrides:
#   KEY_COUNT        (default: 10000000)
#   ITERATIONS       (default: 100000)
#   PIPE_SIZE        (default: 100)
#   WORKERS          (default: 16)

set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/../../.." && pwd)
COMPOSE_DIR="$REPO_ROOT/deployments/docker-compose/cache"
LOADGEN_DIR="$REPO_ROOT/benchmarks/loadgen-cache"
RESULTS_DIR="$SCRIPT_DIR/results"
BINARY="$LOADGEN_DIR/bin/loadgen-cache"

KEY_COUNT="${KEY_COUNT:-10000000}"
ITERATIONS="${ITERATIONS:-100000}"
PIPE_SIZE="${PIPE_SIZE:-100}"
WORKERS="${WORKERS:-16}"
SMOKE_COUNT="${SMOKE_COUNT:-1000}"
SMOKE_ITER="${SMOKE_ITER:-100}"
SKIP_BUILD="${SKIP_BUILD:-false}"

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
  KEY_COUNT
  ITERATIONS
  PIPE_SIZE
  WORKERS
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
    log "Building loadgen-cache..."
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
  log "Starting Docker Compose stack (cache)..."
  if [ "$CLEAN" = true ]; then
    log "  --clean: removing existing volumes"
    docker compose -f "$COMPOSE_DIR/docker-compose.yml" down -v --remove-orphans 2>/dev/null || true
  fi
  docker compose -f "$COMPOSE_DIR/docker-compose.yml" up -d
}

wait_for_redis() {
  log "Waiting for Redis (port 6379)..."
  local max=20 i=0
  until docker compose -f "$COMPOSE_DIR/docker-compose.yml" exec -T redis redis-cli ping 2>/dev/null | grep -q PONG; do
    i=$((i+1))
    [ $i -ge $max ] && { echo "error: Redis not ready" >&2; exit 1; }
    sleep 3
  done
  log "Redis ready."
}

wait_for_valkey() {
  log "Waiting for Valkey (port 6380)..."
  local max=20 i=0
  until docker compose -f "$COMPOSE_DIR/docker-compose.yml" exec -T valkey valkey-cli ping 2>/dev/null | grep -q PONG; do
    i=$((i+1))
    [ $i -ge $max ] && { echo "error: Valkey not ready" >&2; exit 1; }
    sleep 3
  done
  log "Valkey ready."
}

smoke_test() {
  log "--- Smoke test (${SMOKE_COUNT} keys, ${SMOKE_ITER} iterations) ---"
  "$BINARY" --db redis  --op all --count "$SMOKE_COUNT" --iterations "$SMOKE_ITER" \
    --pipe-size 10 --workers 4 --flush --addr localhost:6379
  "$BINARY" --db valkey --op all --count "$SMOKE_COUNT" --iterations "$SMOKE_ITER" \
    --pipe-size 10 --workers 4 --flush --addr localhost:6380
  log "Smoke test passed."
}

run_full() {
  log "--- Full benchmark: KEY_COUNT=$KEY_COUNT ---"
  mkdir -p "$RESULTS_DIR"

  log "Running Redis benchmark..."
  "$BINARY" \
    --db         redis \
    --op         all \
    --count      "$KEY_COUNT" \
    --iterations "$ITERATIONS" \
    --pipe-size  "$PIPE_SIZE" \
    --workers    "$WORKERS" \
    --flush \
    --addr       localhost:6379 \
    --out        "$RESULTS_DIR/redis.json"

  log "Running Valkey benchmark..."
  "$BINARY" \
    --db         valkey \
    --op         all \
    --count      "$KEY_COUNT" \
    --iterations "$ITERATIONS" \
    --pipe-size  "$PIPE_SIZE" \
    --workers    "$WORKERS" \
    --flush \
    --addr       localhost:6380 \
    --out        "$RESULTS_DIR/valkey.json"
}

merge_results() {
  log "Merging results..."
  jq -s '{
    schema_version: "results-summary/v1",
    experiment: {
      id: "redis-vs-valkey",
      name: "Redis vs Valkey",
      category: "cache",
      path: "experiments/cache/redis-vs-valkey"
    },
    run_id: .[0].run_id,
    timestamp: .[0].timestamp,
    mode: "full",
    config: {
      key_count: '"$KEY_COUNT"',
      iterations: '"$ITERATIONS"',
      pipe_size: '"$PIPE_SIZE"',
      workers: '"$WORKERS"'
    },
    sources: [
      {name: "redis", file: "results/redis.json"},
      {name: "valkey", file: "results/valkey.json"}
    ],
    results: [.[].results[]]
  }' "$RESULTS_DIR/redis.json" "$RESULTS_DIR/valkey.json" \
    > "$RESULTS_DIR/summary.json"
}

verify_results() {
  for path in \
    "$RESULTS_DIR/redis.json" \
    "$RESULTS_DIR/valkey.json" \
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
  printf "%-8s %-14s %12s %10s %10s %12s %10s %10s\n" \
    "DB" "Operation" "QPS" "p50(ms)" "p95(ms)" "p99(ms)" "Memory" "Keys"
  printf "%-8s %-14s %12s %10s %10s %12s %10s %10s\n" \
    "--------" "--------------" "------------" "--------" "--------" "------------" "--------" "--------"
  jq -r '.results[] | [
    .db, .op,
    (.throughput_ops_sec|floor|tostring),
    (.p50_ms|tostring),
    (.p95_ms|tostring),
    (.p99_ms|tostring),
    (if .memory_used then .memory_used else "-" end),
    (if .key_count > 0 then .key_count|tostring else "-" end)
  ] | @tsv' "$RESULTS_DIR/summary.json" | \
  while IFS=$'\t' read -r db op qps p50 p95 p99 mem keys; do
    printf "%-8s %-14s %12s %10s %10s %12s %10s %10s\n" \
      "$db" "$op" "$qps" "$p50" "$p95" "$p99" "$mem" "$keys"
  done
  echo ""
}

# --- Main ---
check_deps
build_binary
start_stack
wait_for_redis
wait_for_valkey

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
log "Grafana:    http://localhost:3001"
log "Prometheus: http://localhost:9091"
