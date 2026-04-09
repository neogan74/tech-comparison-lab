#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
API_DIR="$REPO_ROOT/apps/bench-api"
BENCH_DIR="$REPO_ROOT/benchmarks/loadgen-http"
COMPOSE_DIR="$REPO_ROOT/deployments/docker-compose/api"
RESULTS_DIR="$SCRIPT_DIR/results"

API_BIN="$API_DIR/bench-api"
BENCH_BIN="$BENCH_DIR/loadgen-http"

REST_ADDR="http://localhost:8080"
GRPC_ADDR="localhost:50051"

COUNT=${COUNT:-100000}
WORKERS=${WORKERS:-50}
SMOKE_COUNT=${SMOKE_COUNT:-1000}
SMOKE=${SMOKE_ONLY:-0}
SKIP_BUILD=${SKIP_BUILD:-0}
KEEP_STACK=${KEEP_STACK:-0}
CLEAN=false

# ── colours ───────────────────────────────────────────────────────────────────
GREEN='\033[0;32m'; YELLOW='\033[1;33m'; NC='\033[0m'
info()  { echo -e "${GREEN}[run]${NC} $*"; }
warn()  { echo -e "${YELLOW}[warn]${NC} $*"; }

check_deps() {
  for cmd in curl jq docker; do
    command -v "$cmd" >/dev/null 2>&1 || { echo "ERROR: '$cmd' not found" >&2; exit 1; }
  done
  docker compose version >/dev/null 2>&1 || { echo "ERROR: Docker Compose v2 is required" >&2; exit 1; }
  docker info >/dev/null 2>&1 || { echo "ERROR: Docker daemon is not running" >&2; exit 1; }
}

usage() {
  cat <<'EOF'
Usage: ./run.sh [--clean] [--smoke-only] [--keep-stack]

Environment overrides:
  COUNT
  WORKERS
  SMOKE_COUNT
  SMOKE_ONLY=1
  SKIP_BUILD=1
  KEEP_STACK=1
EOF
}

# ── build ─────────────────────────────────────────────────────────────────────
for arg in "$@"; do
  case "$arg" in
    --clean) CLEAN=true ;;
    --smoke-only) ;;
    --keep-stack) KEEP_STACK=1 ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "ERROR: unknown argument '$arg'" >&2
      exit 1
      ;;
  esac
done

check_deps

cleanup_stack() {
  if [[ "$KEEP_STACK" == "1" ]]; then
    warn "KEEP_STACK=1 — API observability stack left running."
    return
  fi
  info "Stopping Prometheus + Grafana stack..."
  docker compose -f "$COMPOSE_DIR/docker-compose.yml" down --remove-orphans >/dev/null 2>&1 || true
}

start_stack() {
  if [[ "$CLEAN" == true ]]; then
    info "Removing existing API observability stack..."
    docker compose -f "$COMPOSE_DIR/docker-compose.yml" down --remove-orphans >/dev/null 2>&1 || true
  fi
  info "Starting Prometheus + Grafana stack..."
  docker compose -f "$COMPOSE_DIR/docker-compose.yml" up -d
}

start_stack

if [[ "$SKIP_BUILD" != "1" ]]; then
  info "Building bench-api server..."
  (cd "$API_DIR" && go build -o bench-api .)

  info "Building loadgen-http..."
  (cd "$BENCH_DIR" && go build -o loadgen-http .)
else
  [[ -x "$API_BIN" ]] || { echo "ERROR: missing bench-api binary: $API_BIN" >&2; exit 1; }
  [[ -x "$BENCH_BIN" ]] || { echo "ERROR: missing loadgen-http binary: $BENCH_BIN" >&2; exit 1; }
  info "Using existing binaries"
fi

# ── start server ──────────────────────────────────────────────────────────────
stop_server() {
  if [[ -n "${SERVER_PID:-}" ]]; then
    info "Stopping bench-api (pid $SERVER_PID)..."
    kill "$SERVER_PID" 2>/dev/null || true
  fi
  cleanup_stack
}
trap stop_server EXIT

info "Starting bench-api (REST :8080, gRPC :50051)..."
"$API_BIN" &
SERVER_PID=$!

# wait until REST responds
for i in $(seq 1 20); do
  if curl -sf "$REST_ADDR/echo?msg=ping" >/dev/null 2>&1; then
    break
  fi
  sleep 0.5
  if [[ $i -eq 20 ]]; then
    echo "ERROR: bench-api did not start in time" >&2
    exit 1
  fi
done
info "bench-api is up"

for i in $(seq 1 20); do
  if curl -sf "http://localhost:8080/metrics" >/dev/null 2>&1; then
    break
  fi
  sleep 0.5
  if [[ $i -eq 20 ]]; then
    echo "ERROR: bench-api metrics endpoint did not start in time" >&2
    exit 1
  fi
done
info "bench-api metrics endpoint is up"

mkdir -p "$RESULTS_DIR"

# ── smoke test ────────────────────────────────────────────────────────────────
info "Smoke test (${SMOKE_COUNT} requests per op, both protocols)..."

"$BENCH_BIN" --proto rest --op all --count "$SMOKE_COUNT" --workers 10 --addr "$REST_ADDR" \
  --out "$RESULTS_DIR/smoke-rest.json"
"$BENCH_BIN" --proto grpc --op all --count "$SMOKE_COUNT" --workers 10 --addr "$GRPC_ADDR" \
  --out "$RESULTS_DIR/smoke-grpc.json"

[[ -s "$RESULTS_DIR/smoke-rest.json" ]] || { echo "ERROR: smoke-rest.json missing" >&2; exit 1; }
[[ -s "$RESULTS_DIR/smoke-grpc.json" ]] || { echo "ERROR: smoke-grpc.json missing" >&2; exit 1; }

info "Smoke test passed"

if [[ "$SMOKE" == "1" || "${1:-}" == "--smoke-only" ]]; then
  info "Smoke-only mode — done."
  exit 0
fi

# ── full benchmark ─────────────────────────────────────────────────────────────
info "Full benchmark: COUNT=$COUNT WORKERS=$WORKERS"

info "REST benchmark..."
"$BENCH_BIN" --proto rest --op all \
  --count "$COUNT" --workers "$WORKERS" \
  --addr "$REST_ADDR" \
  --out "$RESULTS_DIR/rest.json"

info "gRPC benchmark..."
"$BENCH_BIN" --proto grpc --op all \
  --count "$COUNT" --workers "$WORKERS" \
  --addr "$GRPC_ADDR" \
  --out "$RESULTS_DIR/grpc.json"

# ── side-by-side summary ──────────────────────────────────────────────────────
if [[ -f "$RESULTS_DIR/rest.json" && -f "$RESULTS_DIR/grpc.json" ]]; then
  echo ""
  echo "════════════════════════════════════════════════════════════════════════"
  echo " gRPC vs REST — side-by-side (RPS, p50/p99 ms)"
  echo "════════════════════════════════════════════════════════════════════════"
  printf "%-20s  %8s %8s %8s %8s %8s %8s\n" "operation" "REST RPS" "gRPC RPS" "REST p50" "gRPC p50" "REST p99" "gRPC p99"
  printf "%-20s  %8s %8s %8s %8s %8s %8s\n" "─────────────────────" "────────" "────────" "────────" "────────" "────────" "────────"

  ops=$(jq -r '.results[].op' "$RESULTS_DIR/rest.json")
  for op in $ops; do
    r_rps=$(jq -r --arg op "$op" '.results[] | select(.op==$op) | .throughput_rps' "$RESULTS_DIR/rest.json" | xargs printf "%.0f")
    g_rps=$(jq -r --arg op "$op" '.results[] | select(.op==$op) | .throughput_rps' "$RESULTS_DIR/grpc.json" | xargs printf "%.0f")
    r_p50=$(jq -r --arg op "$op" '.results[] | select(.op==$op) | .p50_ms' "$RESULTS_DIR/rest.json" | xargs printf "%.2f")
    g_p50=$(jq -r --arg op "$op" '.results[] | select(.op==$op) | .p50_ms' "$RESULTS_DIR/grpc.json" | xargs printf "%.2f")
    r_p99=$(jq -r --arg op "$op" '.results[] | select(.op==$op) | .p99_ms' "$RESULTS_DIR/rest.json" | xargs printf "%.2f")
    g_p99=$(jq -r --arg op "$op" '.results[] | select(.op==$op) | .p99_ms' "$RESULTS_DIR/grpc.json" | xargs printf "%.2f")
    printf "%-20s  %8s %8s %8s %8s %8s %8s\n" "$op" "$r_rps" "$g_rps" "$r_p50" "$g_p50" "$r_p99" "$g_p99"
  done
  echo "════════════════════════════════════════════════════════════════════════"
fi

# ── save combined summary ─────────────────────────────────────────────────────
jq -s '{
  schema_version: "results-summary/v1",
  experiment: {
    id: "grpc-vs-rest",
    name: "gRPC vs REST",
    category: "api",
    path: "experiments/api/grpc-vs-rest"
  },
  run_id: .[0].run_id,
  timestamp: .[0].timestamp,
  mode: "full",
  config: {
    count: '"$COUNT"',
    workers: '"$WORKERS"'
  },
  sources: [
    {name: "rest", file: "results/rest.json"},
    {name: "grpc", file: "results/grpc.json"}
  ],
  results: [.[].results[]]
}' \
  "$RESULTS_DIR/rest.json" "$RESULTS_DIR/grpc.json" \
  > "$RESULTS_DIR/summary.json"
[[ -s "$RESULTS_DIR/summary.json" ]] || { echo "ERROR: summary.json missing" >&2; exit 1; }
info "Summary saved to $RESULTS_DIR/summary.json"

info "Prometheus: http://localhost:9096"
info "Grafana:    http://localhost:3004"
info "Done."
