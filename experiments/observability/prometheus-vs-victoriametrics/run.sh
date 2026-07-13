#!/usr/bin/env bash
# Experiment: Prometheus vs VictoriaMetrics — time series ingest + PromQL benchmark
# Usage: ./run.sh [--clean] [--smoke-only]
#
# Environment overrides:
#   COUNT           (default: 10000000  — 10M samples)
#   SERIES          (default: 10000     — unique time series / cardinality)
#   INTERVAL        (default: 15        — seconds between samples of the same series)
#   BATCH_SIZE      (default: 5000      — samples per remote_write request)
#   WORKERS         (default: 4)
#   QUERY_ITER      (default: 5)
#   SMOKE_COUNT     (default: 50000)
#   SMOKE_SERIES    (default: 500)
#   SMOKE_ITER      (default: 2)
#   SKIP_BUILD=true

set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/../../.." && pwd)
COMPOSE_DIR="$REPO_ROOT/deployments/docker-compose/observability"
LOADGEN_DIR="$REPO_ROOT/benchmarks/loadgen-observability"
RESULTS_DIR="$SCRIPT_DIR/results"
BINARY="$LOADGEN_DIR/bin/loadgen-observability"

COUNT="${COUNT:-10000000}"
SERIES="${SERIES:-10000}"
INTERVAL="${INTERVAL:-15}"
BATCH_SIZE="${BATCH_SIZE:-5000}"
WORKERS="${WORKERS:-4}"
QUERY_ITER="${QUERY_ITER:-5}"
SMOKE_COUNT="${SMOKE_COUNT:-50000}"
SMOKE_SERIES="${SMOKE_SERIES:-500}"
SMOKE_ITER="${SMOKE_ITER:-2}"
SKIP_BUILD="${SKIP_BUILD:-false}"

PROM_ADDR="http://localhost:9090"
VM_ADDR="http://localhost:8428"

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
  SERIES
  INTERVAL
  BATCH_SIZE
  WORKERS
  QUERY_ITER
  SMOKE_COUNT
  SMOKE_SERIES
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
    log "Building loadgen-observability..."
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
  log "Starting Docker Compose stack (observability)..."
  if [ "$1" = "clean" ]; then
    docker compose -f "$COMPOSE_DIR/docker-compose.yml" down -v --remove-orphans 2>/dev/null || true
  fi
  docker compose -f "$COMPOSE_DIR/docker-compose.yml" up -d
}

wait_for_prometheus() {
  log "Waiting for Prometheus (port 9090)..."
  local max=24 i=0
  until curl -sf "$PROM_ADDR/-/ready" >/dev/null 2>&1; do
    i=$((i+1))
    [ $i -ge $max ] && { echo "error: Prometheus not ready after 120s" >&2; exit 1; }
    sleep 5
  done
  log "Prometheus ready."
}

wait_for_victoriametrics() {
  log "Waiting for VictoriaMetrics (port 8428)..."
  local max=24 i=0
  until curl -sf "$VM_ADDR/health" >/dev/null 2>&1; do
    i=$((i+1))
    [ $i -ge $max ] && { echo "error: VictoriaMetrics not ready after 120s" >&2; exit 1; }
    sleep 5
  done
  log "VictoriaMetrics ready."
}

wait_for_stack() {
  wait_for_prometheus
  wait_for_victoriametrics
}

smoke_test() {
  log "--- Smoke test (${SMOKE_COUNT} samples, ${SMOKE_SERIES} series, ${SMOKE_ITER} query iters) ---"
  "$BINARY" --db prometheus      --op all --count "$SMOKE_COUNT" --series "$SMOKE_SERIES" \
    --interval "$INTERVAL" --batch 1000 --workers 2 --query-iter "$SMOKE_ITER" --addr "$PROM_ADDR"
  "$BINARY" --db victoriametrics --op all --count "$SMOKE_COUNT" --series "$SMOKE_SERIES" \
    --interval "$INTERVAL" --batch 1000 --workers 2 --query-iter "$SMOKE_ITER" --addr "$VM_ADDR"
  log "Smoke test passed."
}

run_full() {
  log "--- Full benchmark: COUNT=$COUNT SERIES=$SERIES ---"
  mkdir -p "$RESULTS_DIR"

  log "Running Prometheus benchmark..."
  "$BINARY" \
    --db         prometheus \
    --op         all \
    --count      "$COUNT" \
    --series     "$SERIES" \
    --interval   "$INTERVAL" \
    --batch      "$BATCH_SIZE" \
    --workers    "$WORKERS" \
    --query-iter "$QUERY_ITER" \
    --addr       "$PROM_ADDR" \
    --out        "$RESULTS_DIR/prometheus.json"

  log "Running VictoriaMetrics benchmark..."
  "$BINARY" \
    --db         victoriametrics \
    --op         all \
    --count      "$COUNT" \
    --series     "$SERIES" \
    --interval   "$INTERVAL" \
    --batch      "$BATCH_SIZE" \
    --workers    "$WORKERS" \
    --query-iter "$QUERY_ITER" \
    --addr       "$VM_ADDR" \
    --out        "$RESULTS_DIR/victoriametrics.json"
}

merge_results() {
  log "Merging results..."
  jq -s '{
    schema_version: "results-summary/v1",
    experiment: {
      id: "prometheus-vs-victoriametrics",
      name: "Prometheus vs VictoriaMetrics",
      category: "observability",
      path: "experiments/observability/prometheus-vs-victoriametrics"
    },
    run_id: .[0].run_id,
    timestamp: .[0].timestamp,
    mode: "full",
    config: {
      count: '"$COUNT"',
      series: '"$SERIES"',
      interval_sec: '"$INTERVAL"',
      batch_size: '"$BATCH_SIZE"',
      workers: '"$WORKERS"',
      query_iter: '"$QUERY_ITER"'
    },
    sources: [
      {name: "prometheus", file: "results/prometheus.json"},
      {name: "victoriametrics", file: "results/victoriametrics.json"}
    ],
    results: [.[].results[]]
  }' "$RESULTS_DIR/prometheus.json" "$RESULTS_DIR/victoriametrics.json" \
    > "$RESULTS_DIR/summary.json"
}

verify_results() {
  for path in \
    "$RESULTS_DIR/prometheus.json" \
    "$RESULTS_DIR/victoriametrics.json" \
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
  printf "%-16s %-18s %10s %10s %10s %12s %10s\n" \
    "DB" "Operation" "count" "p50(ms)" "p95(ms)" "p99(ms)" "Storage"
  printf "%-16s %-18s %10s %10s %10s %12s %10s\n" \
    "----------------" "------------------" "----------" "--------" "--------" "------------" "--------"
  jq -r '.results[] | [
    .db, .op, (.count|tostring),
    (.p50_ms|tostring), (.p95_ms|tostring), (.p99_ms|tostring),
    (if .storage_bytes > 0 then (.storage_bytes/1e6|.*10|floor/10|tostring)+"MB" else "-" end)
  ] | @tsv' "$RESULTS_DIR/summary.json" | \
  while IFS=$'\t' read -r db op cnt p50 p95 p99 storage; do
    printf "%-16s %-18s %10s %10s %10s %12s %10s\n" \
      "$db" "$op" "$cnt" "$p50" "$p95" "$p99" "$storage"
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

# Restart with fresh volumes so the full run's storage-size measurement
# isn't inflated by smoke-test data.
log "Restarting stack with clean volumes before the full run..."
start_stack clean
wait_for_stack

run_full
merge_results
verify_results
print_table

log "Done. Results: $RESULTS_DIR/summary.json"
log "Prometheus:      $PROM_ADDR"
log "VictoriaMetrics:  $VM_ADDR (VMUI: $VM_ADDR/vmui)"
log "Meta-Prometheus:  http://localhost:9098"
log "Grafana:          http://localhost:3006"
