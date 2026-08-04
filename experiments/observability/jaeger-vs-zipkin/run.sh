#!/usr/bin/env bash
# Experiment: Jaeger vs Zipkin — span ingest + trace query benchmark
# Usage: ./run.sh [--clean] [--smoke-only]
#
# Environment overrides:
#   COUNT           (default: 200000   — total spans ingested)
#   SERVICES        (default: 50       — distinct synthetic services)
#   BATCH_SIZE      (default: 500      — spans per ingest request)
#   WORKERS         (default: 4)
#   QUERY_ITER      (default: 20)
#   SMOKE_COUNT     (default: 5000)
#   SMOKE_SERVICES  (default: 10)
#   SMOKE_ITER      (default: 3)
#   SKIP_BUILD=true

set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/../../.." && pwd)
COMPOSE_DIR="$REPO_ROOT/deployments/docker-compose/tracing"
LOADGEN_DIR="$REPO_ROOT/benchmarks/loadgen-tracing"
RESULTS_DIR="$SCRIPT_DIR/results"
BINARY="$LOADGEN_DIR/bin/loadgen-tracing"

COUNT="${COUNT:-200000}"
SERVICES="${SERVICES:-50}"
BATCH_SIZE="${BATCH_SIZE:-500}"
WORKERS="${WORKERS:-4}"
QUERY_ITER="${QUERY_ITER:-20}"
SMOKE_COUNT="${SMOKE_COUNT:-5000}"
SMOKE_SERVICES="${SMOKE_SERVICES:-10}"
SMOKE_ITER="${SMOKE_ITER:-3}"
SKIP_BUILD="${SKIP_BUILD:-false}"

JAEGER_INGEST="http://localhost:9411"
JAEGER_QUERY="http://localhost:16686"
ZIPKIN_ADDR="http://localhost:9412"

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
  COUNT
  SERVICES
  BATCH_SIZE
  WORKERS
  QUERY_ITER
  SMOKE_COUNT
  SMOKE_SERVICES
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
  for cmd in docker jq curl; do
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
    log "Building loadgen-tracing..."
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
  log "Starting Docker Compose stack (tracing)..."
  if [ "$1" = "clean" ]; then
    docker compose -f "$COMPOSE_DIR/docker-compose.yml" down -v --remove-orphans 2>/dev/null || true
  fi
  docker compose -f "$COMPOSE_DIR/docker-compose.yml" up -d
}

wait_for_jaeger() {
  log "Waiting for Jaeger (query 16686)..."
  local max=24 i=0
  until curl -sf "$JAEGER_QUERY/api/services" >/dev/null 2>&1; do
    i=$((i+1))
    [ $i -ge $max ] && { echo "error: Jaeger not ready after 120s" >&2; exit 1; }
    sleep 5
  done
  log "Jaeger ready."
}

wait_for_zipkin() {
  log "Waiting for Zipkin (port 9412)..."
  local max=24 i=0
  until curl -sf "$ZIPKIN_ADDR/health" >/dev/null 2>&1; do
    i=$((i+1))
    [ $i -ge $max ] && { echo "error: Zipkin not ready after 120s" >&2; exit 1; }
    sleep 5
  done
  log "Zipkin ready."
}

wait_for_stack() {
  wait_for_jaeger
  wait_for_zipkin
}

smoke_test() {
  log "--- Smoke test (${SMOKE_COUNT} spans, ${SMOKE_SERVICES} services, ${SMOKE_ITER} query iters) ---"
  "$BINARY" --db jaeger --op all --count "$SMOKE_COUNT" --services "$SMOKE_SERVICES" \
    --batch 200 --workers 2 --query-iter "$SMOKE_ITER" \
    --addr "$JAEGER_INGEST" --query-addr "$JAEGER_QUERY"
  "$BINARY" --db zipkin --op all --count "$SMOKE_COUNT" --services "$SMOKE_SERVICES" \
    --batch 200 --workers 2 --query-iter "$SMOKE_ITER" \
    --addr "$ZIPKIN_ADDR"
  log "Smoke test passed."
}

run_full() {
  log "--- Full benchmark: COUNT=$COUNT SERVICES=$SERVICES ---"
  mkdir -p "$RESULTS_DIR"

  log "Running Jaeger benchmark..."
  "$BINARY" \
    --db         jaeger \
    --op         all \
    --count      "$COUNT" \
    --services   "$SERVICES" \
    --batch      "$BATCH_SIZE" \
    --workers    "$WORKERS" \
    --query-iter "$QUERY_ITER" \
    --addr       "$JAEGER_INGEST" \
    --query-addr "$JAEGER_QUERY" \
    --out        "$RESULTS_DIR/jaeger.json"

  log "Running Zipkin benchmark..."
  "$BINARY" \
    --db         zipkin \
    --op         all \
    --count      "$COUNT" \
    --services   "$SERVICES" \
    --batch      "$BATCH_SIZE" \
    --workers    "$WORKERS" \
    --query-iter "$QUERY_ITER" \
    --addr       "$ZIPKIN_ADDR" \
    --out        "$RESULTS_DIR/zipkin.json"
}

merge_results() {
  log "Merging results..."
  jq -s '{
    schema_version: "results-summary/v1",
    experiment: {
      id: "jaeger-vs-zipkin",
      name: "Jaeger vs Zipkin",
      category: "observability",
      path: "experiments/observability/jaeger-vs-zipkin"
    },
    run_id: .[0].run_id,
    timestamp: .[0].timestamp,
    mode: "full",
    config: {
      count: '"$COUNT"',
      services: '"$SERVICES"',
      batch_size: '"$BATCH_SIZE"',
      workers: '"$WORKERS"',
      query_iter: '"$QUERY_ITER"'
    },
    sources: [
      {name: "jaeger", file: "results/jaeger.json"},
      {name: "zipkin", file: "results/zipkin.json"}
    ],
    results: [.[].results[]]
  }' "$RESULTS_DIR/jaeger.json" "$RESULTS_DIR/zipkin.json" \
    > "$RESULTS_DIR/summary.json"
}

verify_results() {
  for path in \
    "$RESULTS_DIR/jaeger.json" \
    "$RESULTS_DIR/zipkin.json" \
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
  printf "%-10s %-18s %10s %10s %10s %10s\n" \
    "Backend" "Operation" "count" "p50(ms)" "p95(ms)" "p99(ms)"
  printf "%-10s %-18s %10s %10s %10s %10s\n" \
    "----------" "------------------" "----------" "--------" "--------" "--------"
  jq -r '.results[] | [
    .db, .op, (.count|tostring),
    (.p50_ms|tostring), (.p95_ms|tostring), (.p99_ms|tostring)
  ] | @tsv' "$RESULTS_DIR/summary.json" | \
  while IFS=$'\t' read -r db op cnt p50 p95 p99; do
    printf "%-10s %-18s %10s %10s %10s %10s\n" \
      "$db" "$op" "$cnt" "$p50" "$p95" "$p99"
  done
  echo ""
}

# --- Main ---
check_deps
build_binary
start_stack "$([ "$CLEAN" = true ] && echo clean || echo keep)"
wait_for_stack

if [ "$SMOKE_ONLY" = true ]; then
  smoke_test
  log "Smoke-only mode: done."
  exit 0
fi

smoke_test

# Restart with fresh volumes so the full run starts from empty backends.
log "Restarting stack with clean volumes before the full run..."
start_stack clean
wait_for_stack

run_full
merge_results
verify_results
print_table

log "Done. Results: $RESULTS_DIR/summary.json"
log "Jaeger UI:  $JAEGER_QUERY"
log "Zipkin UI:  $ZIPKIN_ADDR/zipkin"
