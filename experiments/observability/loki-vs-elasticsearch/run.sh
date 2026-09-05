#!/usr/bin/env bash
# Experiment: Loki vs Elasticsearch — log ingest + log query benchmark
# Usage: ./run.sh [--clean] [--smoke-only]
#
# Environment overrides:
#   COUNT           (default: 500000  — total log entries written)
#   SERVICES        (default: 50      — distinct synthetic services)
#   BATCH_SIZE      (default: 2000    — entries per ingest request)
#   WORKERS         (default: 4)
#   QUERY_ITER      (default: 20)
#   WINDOW          (default: 300     — seconds entries are spread over / looked back)
#   LIMIT           (default: 100     — page size for line-returning queries)
#   SMOKE_COUNT     (default: 5000)
#   SMOKE_SERVICES  (default: 10)
#   SMOKE_ITER      (default: 3)
#   SKIP_BUILD=true

set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/../../.." && pwd)
COMPOSE_DIR="$REPO_ROOT/deployments/docker-compose/logs"
LOADGEN_DIR="$REPO_ROOT/benchmarks/loadgen-logs"
RESULTS_DIR="$SCRIPT_DIR/results"
BINARY="$LOADGEN_DIR/bin/loadgen-logs"

COUNT="${COUNT:-500000}"
SERVICES="${SERVICES:-50}"
BATCH_SIZE="${BATCH_SIZE:-2000}"
WORKERS="${WORKERS:-4}"
QUERY_ITER="${QUERY_ITER:-20}"
WINDOW="${WINDOW:-300}"
LIMIT="${LIMIT:-100}"
SMOKE_COUNT="${SMOKE_COUNT:-5000}"
SMOKE_SERVICES="${SMOKE_SERVICES:-10}"
SMOKE_ITER="${SMOKE_ITER:-3}"
SKIP_BUILD="${SKIP_BUILD:-false}"

LOKI_ADDR="http://localhost:3100"
ES_ADDR="http://localhost:9200"

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
  WINDOW
  LIMIT
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
    log "Building loadgen-logs..."
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
  log "Starting Docker Compose stack (logs)..."
  if [ "$1" = "clean" ]; then
    docker compose -f "$COMPOSE_DIR/docker-compose.yml" down -v --remove-orphans 2>/dev/null || true
  fi
  docker compose -f "$COMPOSE_DIR/docker-compose.yml" up -d
}

wait_for_loki() {
  log "Waiting for Loki (port 3100)..."
  local max=24 i=0
  until curl -sf "$LOKI_ADDR/ready" >/dev/null 2>&1; do
    i=$((i+1))
    [ $i -ge $max ] && { echo "error: Loki not ready after 120s" >&2; exit 1; }
    sleep 5
  done
  log "Loki ready."
}

wait_for_es() {
  log "Waiting for Elasticsearch (port 9200)..."
  local max=24 i=0
  until curl -sf "$ES_ADDR/_cluster/health?wait_for_status=yellow&timeout=1s" >/dev/null 2>&1; do
    i=$((i+1))
    [ $i -ge $max ] && { echo "error: Elasticsearch not ready after 120s" >&2; exit 1; }
    sleep 5
  done
  log "Elasticsearch ready."
}

wait_for_stack() {
  wait_for_loki
  wait_for_es
}

smoke_test() {
  log "--- Smoke test (${SMOKE_COUNT} entries, ${SMOKE_SERVICES} services, ${SMOKE_ITER} query iters) ---"
  "$BINARY" --db loki --op all --count "$SMOKE_COUNT" --services "$SMOKE_SERVICES" \
    --batch 500 --workers 2 --query-iter "$SMOKE_ITER" --window "$WINDOW" --limit "$LIMIT" \
    --addr "$LOKI_ADDR"
  "$BINARY" --db elasticsearch --op all --count "$SMOKE_COUNT" --services "$SMOKE_SERVICES" \
    --batch 500 --workers 2 --query-iter "$SMOKE_ITER" --window "$WINDOW" --limit "$LIMIT" \
    --addr "$ES_ADDR"
  log "Smoke test passed."
}

run_full() {
  log "--- Full benchmark: COUNT=$COUNT SERVICES=$SERVICES ---"
  mkdir -p "$RESULTS_DIR"

  log "Running Loki benchmark..."
  "$BINARY" \
    --db         loki \
    --op         all \
    --count      "$COUNT" \
    --services   "$SERVICES" \
    --batch      "$BATCH_SIZE" \
    --workers    "$WORKERS" \
    --query-iter "$QUERY_ITER" \
    --window     "$WINDOW" \
    --limit      "$LIMIT" \
    --addr       "$LOKI_ADDR" \
    --out        "$RESULTS_DIR/loki.json"

  log "Running Elasticsearch benchmark..."
  "$BINARY" \
    --db         elasticsearch \
    --op         all \
    --count      "$COUNT" \
    --services   "$SERVICES" \
    --batch      "$BATCH_SIZE" \
    --workers    "$WORKERS" \
    --query-iter "$QUERY_ITER" \
    --window     "$WINDOW" \
    --limit      "$LIMIT" \
    --addr       "$ES_ADDR" \
    --out        "$RESULTS_DIR/elasticsearch.json"
}

merge_results() {
  log "Merging results..."
  jq -s '{
    schema_version: "results-summary/v1",
    experiment: {
      id: "loki-vs-elasticsearch",
      name: "Loki vs Elasticsearch",
      category: "observability",
      path: "experiments/observability/loki-vs-elasticsearch"
    },
    run_id: .[0].run_id,
    timestamp: .[0].timestamp,
    mode: "full",
    config: {
      count: '"$COUNT"',
      services: '"$SERVICES"',
      batch_size: '"$BATCH_SIZE"',
      workers: '"$WORKERS"',
      query_iter: '"$QUERY_ITER"',
      window: '"$WINDOW"',
      limit: '"$LIMIT"'
    },
    sources: [
      {name: "loki", file: "results/loki.json"},
      {name: "elasticsearch", file: "results/elasticsearch.json"}
    ],
    results: [.[].results[]]
  }' "$RESULTS_DIR/loki.json" "$RESULTS_DIR/elasticsearch.json" \
    > "$RESULTS_DIR/summary.json"
}

verify_results() {
  for path in \
    "$RESULTS_DIR/loki.json" \
    "$RESULTS_DIR/elasticsearch.json" \
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
  printf "%-14s %-18s %10s %10s %10s %10s\n" \
    "Backend" "Operation" "count" "p50(ms)" "p95(ms)" "p99(ms)"
  printf "%-14s %-18s %10s %10s %10s %10s\n" \
    "--------------" "------------------" "----------" "--------" "--------" "--------"
  jq -r '.results[] | [
    .db, .op, (.count|tostring),
    (.p50_ms|tostring), (.p95_ms|tostring), (.p99_ms|tostring)
  ] | @tsv' "$RESULTS_DIR/summary.json" | \
  while IFS=$'\t' read -r db op cnt p50 p95 p99; do
    printf "%-14s %-18s %10s %10s %10s %10s\n" \
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
log "Loki:          $LOKI_ADDR"
log "Elasticsearch: $ES_ADDR"
