#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
API_DIR="$REPO_ROOT/apps/bench-api"
BENCH_DIR="$REPO_ROOT/benchmarks/loadgen-http"
COMPOSE_DIR="$REPO_ROOT/deployments/docker-compose/kong-vs-tyk"
RESULTS_DIR="$SCRIPT_DIR/results"

API_BIN="$API_DIR/bench-api"
BENCH_BIN="$BENCH_DIR/loadgen-http"

KONG_ADDR="http://localhost:8000"
TYK_ADDR="http://localhost:8090"

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
    warn "KEEP_STACK=1 — Kong + Tyk stack left running."
    return
  fi
  info "Stopping Kong + Tyk stack..."
  docker compose -f "$COMPOSE_DIR/docker-compose.yml" down --remove-orphans >/dev/null 2>&1 || true
}

start_stack() {
  if [[ "$CLEAN" == true ]]; then
    info "Removing existing gateway stack..."
    docker compose -f "$COMPOSE_DIR/docker-compose.yml" down --remove-orphans >/dev/null 2>&1 || true
  fi
  info "Starting Kong + Tyk stack..."
  docker compose -f "$COMPOSE_DIR/docker-compose.yml" up -d
}

# ── build binaries ─────────────────────────────────────────────────────────────
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

# ── start backend + gateways ────────────────────────────────────────────────────
stop_server() {
  if [[ -n "${SERVER_PID:-}" ]]; then
    info "Stopping bench-api (pid $SERVER_PID)..."
    kill "$SERVER_PID" 2>/dev/null || true
  fi
  cleanup_stack
}
trap stop_server EXIT

info "Starting bench-api backend (REST :8080)..."
"$API_BIN" &
SERVER_PID=$!

# wait until the backend responds directly
for i in $(seq 1 20); do
  if curl -sf "http://localhost:8080/echo?msg=ping" >/dev/null 2>&1; then
    break
  fi
  sleep 0.5
  if [[ $i -eq 20 ]]; then
    echo "ERROR: bench-api did not start in time" >&2
    exit 1
  fi
done
info "bench-api backend is up"

start_stack

# wait until each gateway proxies to the backend
wait_gateway() {
  local name="$1" addr="$2"
  for i in $(seq 1 40); do
    if curl -sf "$addr/echo?msg=ping" >/dev/null 2>&1; then
      info "$name is proxying ($addr)"
      return 0
    fi
    sleep 0.5
    if [[ $i -eq 40 ]]; then
      echo "ERROR: $name did not become ready at $addr" >&2
      docker compose -f "$COMPOSE_DIR/docker-compose.yml" ps || true
      exit 1
    fi
  done
}

wait_gateway "Kong" "$KONG_ADDR"
wait_gateway "Tyk" "$TYK_ADDR"

mkdir -p "$RESULTS_DIR"

# ── smoke test ────────────────────────────────────────────────────────────────
info "Smoke test (${SMOKE_COUNT} requests per op, both gateways)..."

"$BENCH_BIN" --proto rest --op all --count "$SMOKE_COUNT" --workers 10 --addr "$KONG_ADDR" \
  --out "$RESULTS_DIR/smoke-kong.json"
"$BENCH_BIN" --proto rest --op all --count "$SMOKE_COUNT" --workers 10 --addr "$TYK_ADDR" \
  --out "$RESULTS_DIR/smoke-tyk.json"

[[ -s "$RESULTS_DIR/smoke-kong.json" ]] || { echo "ERROR: smoke-kong.json missing" >&2; exit 1; }
[[ -s "$RESULTS_DIR/smoke-tyk.json" ]]  || { echo "ERROR: smoke-tyk.json missing" >&2; exit 1; }

info "Smoke test passed"

if [[ "$SMOKE" == "1" || "${1:-}" == "--smoke-only" ]]; then
  info "Smoke-only mode — done."
  exit 0
fi

# ── full benchmark ─────────────────────────────────────────────────────────────
info "Full benchmark: COUNT=$COUNT WORKERS=$WORKERS"

info "Kong benchmark..."
"$BENCH_BIN" --proto rest --op all \
  --count "$COUNT" --workers "$WORKERS" \
  --addr "$KONG_ADDR" \
  --out "$RESULTS_DIR/kong.json"

info "Tyk benchmark..."
"$BENCH_BIN" --proto rest --op all \
  --count "$COUNT" --workers "$WORKERS" \
  --addr "$TYK_ADDR" \
  --out "$RESULTS_DIR/tyk.json"

# ── side-by-side summary ──────────────────────────────────────────────────────
if [[ -f "$RESULTS_DIR/kong.json" && -f "$RESULTS_DIR/tyk.json" ]]; then
  echo ""
  echo "════════════════════════════════════════════════════════════════════════"
  echo " Kong vs Tyk — side-by-side (RPS, p50/p99 ms)"
  echo "════════════════════════════════════════════════════════════════════════"
  printf "%-20s  %8s %8s %8s %8s %8s %8s\n" "operation" "KONG RPS" "TYK RPS" "KONG p50" "TYK p50" "KONG p99" "TYK p99"
  printf "%-20s  %8s %8s %8s %8s %8s %8s\n" "─────────────────────" "────────" "────────" "────────" "────────" "────────" "────────"

  ops=$(jq -r '.results[].op' "$RESULTS_DIR/kong.json")
  for op in $ops; do
    k_rps=$(jq -r --arg op "$op" '.results[] | select(.op==$op) | .throughput_rps' "$RESULTS_DIR/kong.json" | xargs printf "%.0f")
    t_rps=$(jq -r --arg op "$op" '.results[] | select(.op==$op) | .throughput_rps' "$RESULTS_DIR/tyk.json" | xargs printf "%.0f")
    k_p50=$(jq -r --arg op "$op" '.results[] | select(.op==$op) | .p50_ms' "$RESULTS_DIR/kong.json" | xargs printf "%.2f")
    t_p50=$(jq -r --arg op "$op" '.results[] | select(.op==$op) | .p50_ms' "$RESULTS_DIR/tyk.json" | xargs printf "%.2f")
    k_p99=$(jq -r --arg op "$op" '.results[] | select(.op==$op) | .p99_ms' "$RESULTS_DIR/kong.json" | xargs printf "%.2f")
    t_p99=$(jq -r --arg op "$op" '.results[] | select(.op==$op) | .p99_ms' "$RESULTS_DIR/tyk.json" | xargs printf "%.2f")
    printf "%-20s  %8s %8s %8s %8s %8s %8s\n" "$op" "$k_rps" "$t_rps" "$k_p50" "$t_p50" "$k_p99" "$t_p99"
  done
  echo "════════════════════════════════════════════════════════════════════════"
fi

# ── save combined summary ─────────────────────────────────────────────────────
jq -s '{
  schema_version: "results-summary/v1",
  experiment: {
    id: "kong-vs-tyk",
    name: "Kong vs Tyk",
    category: "api",
    path: "experiments/api/kong-vs-tyk"
  },
  run_id: .[0].run_id,
  timestamp: .[0].timestamp,
  mode: "full",
  config: {
    count: '"$COUNT"',
    workers: '"$WORKERS"'
  },
  sources: [
    {name: "kong", file: "results/kong.json"},
    {name: "tyk",  file: "results/tyk.json"}
  ],
  results: [.[].results[]]
}' \
  "$RESULTS_DIR/kong.json" "$RESULTS_DIR/tyk.json" \
  > "$RESULTS_DIR/summary.json"
[[ -s "$RESULTS_DIR/summary.json" ]] || { echo "ERROR: summary.json missing" >&2; exit 1; }
info "Summary saved to $RESULTS_DIR/summary.json"

info "Kong proxy: http://localhost:8000  (admin: http://localhost:8001)"
info "Tyk proxy:  http://localhost:8090"
info "Done."
