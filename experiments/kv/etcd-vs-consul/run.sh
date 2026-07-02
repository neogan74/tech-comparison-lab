#!/usr/bin/env bash
# Experiment: etcd vs Consul KV store benchmark
# Usage: ./run.sh [--clean] [--smoke-only]
#
# Environment overrides:
#   COUNT              (default: 10000)
#   WORKERS            (default: 8)
#   WATCH_COUNT        (default: 100)
#   ELECTION_COUNT     (default: 20)
#   SMOKE_COUNT        (default: 200)
#   SKIP_BUILD=true

set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/../../.." && pwd)
COMPOSE_DIR="$REPO_ROOT/deployments/docker-compose/kv"
LOADGEN_DIR="$REPO_ROOT/benchmarks/loadgen-kv"
RESULTS_DIR="$SCRIPT_DIR/results"
BINARY="$LOADGEN_DIR/bin/loadgen-kv"

COUNT="${COUNT:-10000}"
WORKERS="${WORKERS:-8}"
WATCH_COUNT="${WATCH_COUNT:-100}"
ELECTION_COUNT="${ELECTION_COUNT:-20}"
SMOKE_COUNT="${SMOKE_COUNT:-200}"
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
  COUNT          total KV operations per run (default: 10000)
  WORKERS        concurrent goroutines for write/read/mixed (default: 8)
  WATCH_COUNT    watch latency measurements (default: 100)
  ELECTION_COUNT leader election measurements (default: 20)
  SMOKE_COUNT    reduced count for smoke test (default: 200)
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
  if ! docker info &>/dev/null; then
    echo "error: Docker daemon is not running." >&2; exit 1
  fi
}

build_binary() {
  if [ "$SKIP_BUILD" = true ]; then
    if [ ! -x "$BINARY" ]; then
      echo "error: SKIP_BUILD=true but binary is missing: $BINARY" >&2; exit 1
    fi
    log "Using existing binary: $BINARY"
    return
  fi

  if have_cmd go; then
    log "Building loadgen-kv..."
    mkdir -p "$LOADGEN_DIR/bin"
    (cd "$LOADGEN_DIR" && go build -o "$BINARY" .)
    log "Binary: $BINARY"
    return
  fi

  if [ -x "$BINARY" ]; then
    log "Go not found; using existing binary: $BINARY"
    return
  fi

  echo "error: 'go' not found and no prebuilt binary at $BINARY" >&2
  exit 1
}

start_stack() {
  log "Starting Docker Compose stack (kv)..."
  if [ "$CLEAN" = true ]; then
    log "  --clean: removing existing volumes"
    docker compose -f "$COMPOSE_DIR/docker-compose.yml" down -v --remove-orphans 2>/dev/null || true
  fi
  docker compose -f "$COMPOSE_DIR/docker-compose.yml" up -d etcd consul consul-exporter prometheus
}

wait_for_etcd() {
  log "Waiting for etcd (port 2379)..."
  local max=30 i=0
  until curl -sf http://localhost:2379/health >/dev/null 2>&1; do
    i=$((i+1))
    [ $i -ge $max ] && { echo "error: etcd not ready after ${max}s" >&2; exit 1; }
    sleep 2
  done
  log "etcd ready."
}

wait_for_consul() {
  log "Waiting for Consul (port 8500)..."
  local max=30 i=0
  until curl -sf http://localhost:8500/v1/status/leader 2>/dev/null | grep -q '"'; do
    i=$((i+1))
    [ $i -ge $max ] && { echo "error: Consul not ready after ${max}s" >&2; exit 1; }
    sleep 2
  done
  log "Consul ready."
}

smoke_test() {
  log "--- Smoke test (count=${SMOKE_COUNT}) ---"
  "$BINARY" --db etcd   --op all --count "$SMOKE_COUNT" --watch-count 10 --election-count 5 \
    --workers 4 --addr localhost:2379
  "$BINARY" --db consul --op all --count "$SMOKE_COUNT" --watch-count 10 --election-count 5 \
    --workers 4 --addr localhost:8500
  log "Smoke test passed."
}

run_full() {
  log "--- Full benchmark: COUNT=$COUNT ---"
  mkdir -p "$RESULTS_DIR"

  log "Running etcd benchmark..."
  "$BINARY" \
    --db             etcd \
    --op             all \
    --count          "$COUNT" \
    --workers        "$WORKERS" \
    --watch-count    "$WATCH_COUNT" \
    --election-count "$ELECTION_COUNT" \
    --addr           localhost:2379 \
    --out            "$RESULTS_DIR/etcd.json"

  log "Running Consul benchmark..."
  "$BINARY" \
    --db             consul \
    --op             all \
    --count          "$COUNT" \
    --workers        "$WORKERS" \
    --watch-count    "$WATCH_COUNT" \
    --election-count "$ELECTION_COUNT" \
    --addr           localhost:8500 \
    --out            "$RESULTS_DIR/consul.json"
}

merge_results() {
  log "Merging results..."
  jq -s '{
    schema_version: "results-summary/v1",
    experiment: {
      id: "etcd-vs-consul",
      name: "etcd vs Consul",
      category: "kv",
      path: "experiments/kv/etcd-vs-consul"
    },
    run_id: .[0].run_id,
    timestamp: .[0].timestamp,
    mode: "full",
    config: {
      count: '"$COUNT"',
      workers: '"$WORKERS"',
      watch_count: '"$WATCH_COUNT"',
      election_count: '"$ELECTION_COUNT"'
    },
    sources: [
      {name: "etcd",   file: "results/etcd.json"},
      {name: "consul", file: "results/consul.json"}
    ],
    results: [.[].results[]]
  }' "$RESULTS_DIR/etcd.json" "$RESULTS_DIR/consul.json" \
    > "$RESULTS_DIR/summary.json"
}

verify_results() {
  for path in \
    "$RESULTS_DIR/etcd.json" \
    "$RESULTS_DIR/consul.json" \
    "$RESULTS_DIR/summary.json"; do
    if [ ! -s "$path" ]; then
      echo "error: expected results file is missing or empty: $path" >&2; exit 1
    fi
  done
}

print_table() {
  log "--- Side-by-side comparison ---"
  echo ""
  printf "%-8s %-10s %12s %10s %10s %12s\n" \
    "DB" "Operation" "QPS" "p50(ms)" "p95(ms)" "p99(ms)"
  printf "%-8s %-10s %12s %10s %10s %12s\n" \
    "--------" "----------" "------------" "--------" "--------" "------------"
  jq -r '.results[] | [
    .db, .op,
    (.throughput_ops_sec|floor|tostring),
    (.p50_ms|tostring),
    (.p95_ms|tostring),
    (.p99_ms|tostring)
  ] | @tsv' "$RESULTS_DIR/summary.json" | \
  while IFS=$'\t' read -r db op qps p50 p95 p99; do
    printf "%-8s %-10s %12s %10s %10s %12s\n" \
      "$db" "$op" "$qps" "$p50" "$p95" "$p99"
  done
  echo ""
}

# --- Main ---
check_deps
build_binary
start_stack
wait_for_etcd
wait_for_consul

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
log "Consul UI:  http://localhost:8500/ui"
log "Prometheus: http://localhost:9095"
log "Grafana:    http://localhost:3005"
