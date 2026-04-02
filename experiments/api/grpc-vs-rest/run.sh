#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
API_DIR="$REPO_ROOT/apps/bench-api"
BENCH_DIR="$REPO_ROOT/benchmarks/loadgen-http"
RESULTS_DIR="$(dirname "$0")/results"

API_BIN="$API_DIR/bench-api"
BENCH_BIN="$BENCH_DIR/loadgen-http"

REST_ADDR="http://localhost:8080"
GRPC_ADDR="localhost:50051"

COUNT=${COUNT:-100000}
WORKERS=${WORKERS:-50}
SMOKE_COUNT=1000
SMOKE=${SMOKE_ONLY:-0}

# ── colours ───────────────────────────────────────────────────────────────────
GREEN='\033[0;32m'; YELLOW='\033[1;33m'; NC='\033[0m'
info()  { echo -e "${GREEN}[run]${NC} $*"; }
warn()  { echo -e "${YELLOW}[warn]${NC} $*"; }

# ── build ─────────────────────────────────────────────────────────────────────
info "Building bench-api server..."
(cd "$API_DIR" && go build -o bench-api .)

info "Building loadgen-http..."
(cd "$BENCH_DIR" && go build -o loadgen-http .)

# ── start server ──────────────────────────────────────────────────────────────
stop_server() {
  if [[ -n "${SERVER_PID:-}" ]]; then
    info "Stopping bench-api (pid $SERVER_PID)..."
    kill "$SERVER_PID" 2>/dev/null || true
  fi
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

mkdir -p "$RESULTS_DIR"

# ── smoke test ────────────────────────────────────────────────────────────────
info "Smoke test (${SMOKE_COUNT} requests per op, both protocols)..."

"$BENCH_BIN" --proto rest --op all --count "$SMOKE_COUNT" --workers 10 --addr "$REST_ADDR" \
  --out "$RESULTS_DIR/smoke-rest.json"
"$BENCH_BIN" --proto grpc --op all --count "$SMOKE_COUNT" --workers 10 --addr "$GRPC_ADDR" \
  --out "$RESULTS_DIR/smoke-grpc.json"

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
if command -v jq &>/dev/null && [[ -f "$RESULTS_DIR/rest.json" && -f "$RESULTS_DIR/grpc.json" ]]; then
  echo ""
  echo "════════════════════════════════════════════════════════════════════════"
  echo " gRPC vs REST — side-by-side (RPS, p50/p99 ms)"
  echo "════════════════════════════════════════════════════════════════════════"
  printf "%-20s  %8s %8s %8s %8s %8s %8s\n" "operation" "REST RPS" "gRPC RPS" "REST p50" "gRPC p50" "REST p99" "gRPC p99"
  printf "%-20s  %8s %8s %8s %8s %8s %8s\n" "─────────────────────" "────────" "────────" "────────" "────────" "────────" "────────"

  ops=$(jq -r '.[].op' "$RESULTS_DIR/rest.json")
  for op in $ops; do
    r_rps=$(jq -r --arg op "$op" '.[] | select(.op==$op) | .throughput_rps' "$RESULTS_DIR/rest.json" | xargs printf "%.0f")
    g_rps=$(jq -r --arg op "$op" '.[] | select(.op==$op) | .throughput_rps' "$RESULTS_DIR/grpc.json" | xargs printf "%.0f")
    r_p50=$(jq -r --arg op "$op" '.[] | select(.op==$op) | .p50_ms' "$RESULTS_DIR/rest.json" | xargs printf "%.2f")
    g_p50=$(jq -r --arg op "$op" '.[] | select(.op==$op) | .p50_ms' "$RESULTS_DIR/grpc.json" | xargs printf "%.2f")
    r_p99=$(jq -r --arg op "$op" '.[] | select(.op==$op) | .p99_ms' "$RESULTS_DIR/rest.json" | xargs printf "%.2f")
    g_p99=$(jq -r --arg op "$op" '.[] | select(.op==$op) | .p99_ms' "$RESULTS_DIR/grpc.json" | xargs printf "%.2f")
    printf "%-20s  %8s %8s %8s %8s %8s %8s\n" "$op" "$r_rps" "$g_rps" "$r_p50" "$g_p50" "$r_p99" "$g_p99"
  done
  echo "════════════════════════════════════════════════════════════════════════"
fi

# ── save combined summary ─────────────────────────────────────────────────────
if command -v jq &>/dev/null; then
  jq -s '{rest: .[0], grpc: .[1]}' \
    "$RESULTS_DIR/rest.json" "$RESULTS_DIR/grpc.json" \
    > "$RESULTS_DIR/summary.json"
  info "Summary saved to $RESULTS_DIR/summary.json"
fi

info "Done."
