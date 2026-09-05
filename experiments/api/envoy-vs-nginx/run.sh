#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
API_DIR="$REPO_ROOT/apps/bench-api"
BENCH_DIR="$REPO_ROOT/benchmarks/loadgen-http"
COMPOSE_DIR="$REPO_ROOT/deployments/docker-compose/edge"
RESULTS_DIR="$SCRIPT_DIR/results"

API_BIN="$API_DIR/bench-api"
BENCH_BIN="$BENCH_DIR/loadgen-http"

NGINX_ADDR="http://localhost:8093"
ENVOY_ADDR="http://localhost:8094"
ENVOY_ADMIN="http://localhost:8095"

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
    warn "KEEP_STACK=1 — Envoy + NGINX stack left running."
    return
  fi
  info "Stopping Envoy + NGINX stack..."
  docker compose -f "$COMPOSE_DIR/docker-compose.yml" down --remove-orphans >/dev/null 2>&1 || true
}

start_stack() {
  if [[ "$CLEAN" == true ]]; then
    info "Removing existing edge stack..."
    docker compose -f "$COMPOSE_DIR/docker-compose.yml" down --remove-orphans >/dev/null 2>&1 || true
  fi
  info "Starting Envoy + NGINX stack..."
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

# ── start backend + proxies ─────────────────────────────────────────────────────
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

# wait until each proxy forwards to the backend
wait_proxy() {
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

wait_proxy "NGINX" "$NGINX_ADDR"
wait_proxy "Envoy" "$ENVOY_ADDR"

mkdir -p "$RESULTS_DIR"

# ── smoke test ────────────────────────────────────────────────────────────────
info "Smoke test (${SMOKE_COUNT} requests per op, both proxies)..."

"$BENCH_BIN" --proto rest --op all --count "$SMOKE_COUNT" --workers 10 --addr "$NGINX_ADDR" \
  --out "$RESULTS_DIR/smoke-nginx.json"
"$BENCH_BIN" --proto rest --op all --count "$SMOKE_COUNT" --workers 10 --addr "$ENVOY_ADDR" \
  --out "$RESULTS_DIR/smoke-envoy.json"

[[ -s "$RESULTS_DIR/smoke-nginx.json" ]] || { echo "ERROR: smoke-nginx.json missing" >&2; exit 1; }
[[ -s "$RESULTS_DIR/smoke-envoy.json" ]] || { echo "ERROR: smoke-envoy.json missing" >&2; exit 1; }

info "Smoke test passed"

if [[ "$SMOKE" == "1" || "${1:-}" == "--smoke-only" ]]; then
  info "Smoke-only mode — done."
  exit 0
fi

# ── full benchmark ─────────────────────────────────────────────────────────────
info "Full benchmark: COUNT=$COUNT WORKERS=$WORKERS"

info "NGINX benchmark..."
"$BENCH_BIN" --proto rest --op all \
  --count "$COUNT" --workers "$WORKERS" \
  --addr "$NGINX_ADDR" \
  --out "$RESULTS_DIR/nginx.json"

info "Envoy benchmark..."
"$BENCH_BIN" --proto rest --op all \
  --count "$COUNT" --workers "$WORKERS" \
  --addr "$ENVOY_ADDR" \
  --out "$RESULTS_DIR/envoy.json"

# ── side-by-side summary ──────────────────────────────────────────────────────
if [[ -f "$RESULTS_DIR/nginx.json" && -f "$RESULTS_DIR/envoy.json" ]]; then
  echo ""
  echo "════════════════════════════════════════════════════════════════════════"
  echo " Envoy vs NGINX — side-by-side (RPS, p50/p99 ms)"
  echo "════════════════════════════════════════════════════════════════════════"
  printf "%-20s  %8s %8s %8s %8s %8s %8s\n" "operation" "NGX RPS" "ENV RPS" "NGX p50" "ENV p50" "NGX p99" "ENV p99"
  printf "%-20s  %8s %8s %8s %8s %8s %8s\n" "─────────────────────" "────────" "────────" "────────" "────────" "────────" "────────"

  ops=$(jq -r '.results[].op' "$RESULTS_DIR/nginx.json")
  for op in $ops; do
    n_rps=$(jq -r --arg op "$op" '.results[] | select(.op==$op) | .throughput_rps' "$RESULTS_DIR/nginx.json" | xargs printf "%.0f")
    e_rps=$(jq -r --arg op "$op" '.results[] | select(.op==$op) | .throughput_rps' "$RESULTS_DIR/envoy.json" | xargs printf "%.0f")
    n_p50=$(jq -r --arg op "$op" '.results[] | select(.op==$op) | .p50_ms' "$RESULTS_DIR/nginx.json" | xargs printf "%.2f")
    e_p50=$(jq -r --arg op "$op" '.results[] | select(.op==$op) | .p50_ms' "$RESULTS_DIR/envoy.json" | xargs printf "%.2f")
    n_p99=$(jq -r --arg op "$op" '.results[] | select(.op==$op) | .p99_ms' "$RESULTS_DIR/nginx.json" | xargs printf "%.2f")
    e_p99=$(jq -r --arg op "$op" '.results[] | select(.op==$op) | .p99_ms' "$RESULTS_DIR/envoy.json" | xargs printf "%.2f")
    printf "%-20s  %8s %8s %8s %8s %8s %8s\n" "$op" "$n_rps" "$e_rps" "$n_p50" "$e_p50" "$n_p99" "$e_p99"
  done
  echo "════════════════════════════════════════════════════════════════════════"
fi

# ── save combined summary ─────────────────────────────────────────────────────
jq -s '{
  schema_version: "results-summary/v1",
  experiment: {
    id: "envoy-vs-nginx",
    name: "Envoy vs NGINX",
    category: "api",
    path: "experiments/api/envoy-vs-nginx"
  },
  run_id: .[0].run_id,
  timestamp: .[0].timestamp,
  mode: "full",
  config: {
    count: '"$COUNT"',
    workers: '"$WORKERS"'
  },
  sources: [
    {name: "nginx", file: "results/nginx.json"},
    {name: "envoy", file: "results/envoy.json"}
  ],
  results: [.[].results[]]
}' \
  "$RESULTS_DIR/nginx.json" "$RESULTS_DIR/envoy.json" \
  > "$RESULTS_DIR/summary.json"
[[ -s "$RESULTS_DIR/summary.json" ]] || { echo "ERROR: summary.json missing" >&2; exit 1; }
info "Summary saved to $RESULTS_DIR/summary.json"

info "NGINX proxy: $NGINX_ADDR"
info "Envoy proxy: $ENVOY_ADDR  (admin: $ENVOY_ADMIN, metrics: $ENVOY_ADMIN/stats/prometheus)"
info "Done."
