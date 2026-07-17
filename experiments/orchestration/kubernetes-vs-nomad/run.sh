#!/usr/bin/env bash
# Experiment: Kubernetes vs Nomad
# Usage: ./run.sh [--clean] [--smoke-only] [--keep-environment]

set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/../../.." && pwd)
BENCH_DIR="$REPO_ROOT/benchmarks/loadgen-scheduler"
BENCH_BIN="$BENCH_DIR/bin/loadgen-scheduler"
RESULTS_DIR="$SCRIPT_DIR/results"

K8S_CLUSTER="${K8S_CLUSTER:-bench-scheduler}"
K8S_CONTEXT="kind-${K8S_CLUSTER}"
NOMAD_ADDRESS="${NOMAD_ADDRESS:-http://127.0.0.1:4646}"
ROUNDS="${ROUNDS:-5}"
REPLICAS="${REPLICAS:-20}"
SMOKE_ROUNDS="${SMOKE_ROUNDS:-1}"
SMOKE_REPLICAS="${SMOKE_REPLICAS:-3}"
OP_TIMEOUT="${OP_TIMEOUT:-3m}"
SKIP_BUILD="${SKIP_BUILD:-false}"
NOMAD_USE_SUDO="${NOMAD_USE_SUDO:-0}"

CLEAN=false
SMOKE_ONLY=false
KEEP_ENVIRONMENT="${KEEP_ENVIRONMENT:-0}"
NOMAD_PID=""
NOMAD_DATA_DIR=""
STARTED_NOMAD=false

GREEN='\033[0;32m'; YELLOW='\033[1;33m'; NC='\033[0m'
info() { echo -e "${GREEN}[run]${NC} $*"; }
warn() { echo -e "${YELLOW}[warn]${NC} $*"; }
have_cmd() { command -v "$1" >/dev/null 2>&1; }

for arg in "$@"; do
  case "$arg" in
    --clean) CLEAN=true ;;
    --smoke-only) SMOKE_ONLY=true ;;
    --keep-environment) KEEP_ENVIRONMENT=1 ;;
    -h|--help)
      cat <<'EOF'
Usage: ./run.sh [--clean] [--smoke-only] [--keep-environment]

Options:
  --clean             recreate the kind cluster before the run
  --smoke-only        use one round and three replicas by default
  --keep-environment  leave the kind cluster and locally-started Nomad agent running

Environment overrides:
  ROUNDS             full-run rounds (default: 5)
  REPLICAS           full-run scale target (default: 20)
  SMOKE_ROUNDS       smoke rounds (default: 1)
  SMOKE_REPLICAS     smoke scale target (default: 3)
  OP_TIMEOUT         timeout per operation (default: 3m)
  K8S_CLUSTER        kind cluster name (default: bench-scheduler)
  NOMAD_ADDRESS      Nomad API address (default: http://127.0.0.1:4646)
  SKIP_BUILD=true    use benchmarks/loadgen-scheduler/bin/loadgen-scheduler
  NOMAD_USE_SUDO=1   start and stop the Nomad agent through sudo
EOF
      exit 0
      ;;
    *)
      echo "error: unknown argument '$arg'" >&2
      exit 1
      ;;
  esac
done

selected_rounds() {
  if [ "$SMOKE_ONLY" = true ]; then echo "$SMOKE_ROUNDS"; else echo "$ROUNDS"; fi
}

selected_replicas() {
  if [ "$SMOKE_ONLY" = true ]; then echo "$SMOKE_REPLICAS"; else echo "$REPLICAS"; fi
}

check_deps() {
  for cmd in docker kind kubectl nomad curl jq; do
    if ! have_cmd "$cmd"; then
      echo "error: '$cmd' not found." >&2
      exit 1
    fi
  done
  if ! docker info >/dev/null 2>&1; then
    echo "error: Docker is not running." >&2
    exit 1
  fi
}

build_binary() {
  if [ "$SKIP_BUILD" = true ]; then
    [ -x "$BENCH_BIN" ] || { echo "error: missing benchmark binary: $BENCH_BIN" >&2; exit 1; }
    return
  fi
  if have_cmd go; then
    info "Building loadgen-scheduler..."
    mkdir -p "$BENCH_DIR/bin"
    (cd "$BENCH_DIR" && go build -o "$BENCH_BIN" .)
    return
  fi
  [ -x "$BENCH_BIN" ] || { echo "error: 'go' not found and no prebuilt binary exists." >&2; exit 1; }
  warn "Go not found; using existing binary."
}

cleanup() {
  if [ "$KEEP_ENVIRONMENT" = "1" ]; then
    warn "Environment left running (kind=${K8S_CLUSTER}, Nomad PID=${NOMAD_PID:-external})."
    return
  fi
  kind delete cluster --name "$K8S_CLUSTER" >/dev/null 2>&1 || true
  if [ "$STARTED_NOMAD" = true ] && [ -n "$NOMAD_PID" ]; then
    if [ "$NOMAD_USE_SUDO" = "1" ]; then
      sudo kill "$NOMAD_PID" >/dev/null 2>&1 || true
    else
      kill "$NOMAD_PID" >/dev/null 2>&1 || true
    fi
    wait "$NOMAD_PID" >/dev/null 2>&1 || true
  fi
  if [ -n "$NOMAD_DATA_DIR" ]; then
    if [ "$NOMAD_USE_SUDO" = "1" ]; then
      sudo rm -rf "$NOMAD_DATA_DIR"
    else
      rm -rf "$NOMAD_DATA_DIR"
    fi
  fi
}

ensure_kind() {
  if [ "$CLEAN" = true ]; then
    kind delete cluster --name "$K8S_CLUSTER" >/dev/null 2>&1 || true
  fi
  if ! kind get clusters 2>/dev/null | grep -q "^${K8S_CLUSTER}$"; then
    info "Creating kind cluster '${K8S_CLUSTER}'..."
    kind create cluster --name "$K8S_CLUSTER" --config - <<'EOF'
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
  - role: control-plane
  - role: worker
EOF
  fi
  kubectl --context "$K8S_CONTEXT" wait --for=condition=Ready nodes --all --timeout=120s >/dev/null
}

ensure_workload_image() {
  local image="registry.k8s.io/pause:3.10"
  if ! docker image inspect "$image" >/dev/null 2>&1; then
    info "Pulling shared workload image..."
    docker pull "$image" >/dev/null
  fi
  info "Loading shared workload image into kind..."
  kind load docker-image "$image" --name "$K8S_CLUSTER" >/dev/null
}

nomad_ready() {
  curl --fail --silent "$NOMAD_ADDRESS/v1/status/leader" | grep -Eq '".+"'
}

nomad_process_running() {
  if [ "$NOMAD_USE_SUDO" = "1" ]; then
    sudo kill -0 "$NOMAD_PID" >/dev/null 2>&1
  else
    kill -0 "$NOMAD_PID" >/dev/null 2>&1
  fi
}

ensure_nomad() {
  if nomad_ready; then
    info "Using Nomad at $NOMAD_ADDRESS"
    return
  fi
  if [ "$NOMAD_ADDRESS" != "http://127.0.0.1:4646" ] && [ "$NOMAD_ADDRESS" != "http://localhost:4646" ]; then
    echo "error: external Nomad is unavailable at $NOMAD_ADDRESS" >&2
    exit 1
  fi
  NOMAD_DATA_DIR=$(mktemp -d "${TMPDIR:-/tmp}/kubernetes-vs-nomad.XXXXXX")
  cat >"$NOMAD_DATA_DIR/nomad.hcl" <<'EOF'
client {
  options = {
    "driver.raw_exec.enable" = "1"
  }
}
EOF
  info "Starting single-node Nomad dev agent..."
  if [ "$NOMAD_USE_SUDO" = "1" ]; then
    sudo -n nomad agent -dev -bind=127.0.0.1 -data-dir="$NOMAD_DATA_DIR/data" -config="$NOMAD_DATA_DIR/nomad.hcl" >"$RESULTS_DIR/nomad.log" 2>&1 &
  else
    nomad agent -dev -bind=127.0.0.1 -data-dir="$NOMAD_DATA_DIR/data" -config="$NOMAD_DATA_DIR/nomad.hcl" >"$RESULTS_DIR/nomad.log" 2>&1 &
  fi
  NOMAD_PID=$!
  STARTED_NOMAD=true
  for _ in $(seq 1 60); do
    if nomad_ready; then return; fi
    if ! nomad_process_running; then
      echo "error: Nomad agent exited; see $RESULTS_DIR/nomad.log" >&2
      exit 1
    fi
    sleep 1
  done
  echo "error: Nomad did not become ready" >&2
  exit 1
}

run_benchmark() {
  local platform="$1" output="$2"
  info "Benchmarking $platform..."
  "$BENCH_BIN" \
    --platform "$platform" \
    --context "$K8S_CONTEXT" \
    --nomad-address "$NOMAD_ADDRESS" \
    --rounds "$(selected_rounds)" \
    --replicas "$(selected_replicas)" \
    --timeout "$OP_TIMEOUT" \
    --out "$output"
}

render_summary() {
  local mode="full"
  if [ "$SMOKE_ONLY" = true ]; then mode="smoke"; fi
  jq -s --arg mode "$mode" --argjson rounds "$(selected_rounds)" --argjson replicas "$(selected_replicas)" '
    .[0] as $kubernetes | .[1] as $nomad | {
      schema_version: "results-summary/v1",
      experiment: {
        id: "kubernetes-vs-nomad",
        name: "Kubernetes vs Nomad",
        category: "orchestration",
        path: "experiments/orchestration/kubernetes-vs-nomad"
      },
      run_id: ("kubernetes-vs-nomad-" + ($kubernetes.timestamp | tostring)),
      timestamp: $kubernetes.timestamp,
      mode: $mode,
      config: {rounds: $rounds, replicas: $replicas},
      sources: [
        {name: "kubernetes", file: "results/kubernetes.json"},
        {name: "nomad", file: "results/nomad.json"}
      ],
      results: (
        [$kubernetes.results[] | . + {platform: "kubernetes"}] +
        [$nomad.results[] | . + {platform: "nomad"}]
      )
    }
  ' "$RESULTS_DIR/kubernetes.json" "$RESULTS_DIR/nomad.json" > "$RESULTS_DIR/summary.json"
  info "Summary saved to $RESULTS_DIR/summary.json"
}

mkdir -p "$RESULTS_DIR"
check_deps
build_binary
trap cleanup EXIT
ensure_kind
ensure_workload_image
ensure_nomad
run_benchmark kubernetes "$RESULTS_DIR/kubernetes.json"
run_benchmark nomad "$RESULTS_DIR/nomad.json"
render_summary

for result in kubernetes.json nomad.json summary.json; do
  [ -s "$RESULTS_DIR/$result" ] || { echo "error: missing result $result" >&2; exit 1; }
done
info "Done. Results: $RESULTS_DIR"
