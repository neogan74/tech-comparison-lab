#!/usr/bin/env bash
# Experiment: Kubernetes vs Docker Swarm
# Usage: ./run.sh [--clean] [--smoke-only] [--keep-environment]

set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/../../.." && pwd)
BENCH_DIR="$REPO_ROOT/benchmarks/loadgen-scheduler"
BENCH_BIN="$BENCH_DIR/bin/loadgen-scheduler"
RESULTS_DIR="$SCRIPT_DIR/results"

K8S_CLUSTER="${K8S_CLUSTER:-bench-swarm}"
K8S_CONTEXT="kind-${K8S_CLUSTER}"
DOCKER_HOST_URI="${DOCKER_HOST_URI:-}"
ROUNDS="${ROUNDS:-5}"
REPLICAS="${REPLICAS:-20}"
SMOKE_ROUNDS="${SMOKE_ROUNDS:-1}"
SMOKE_REPLICAS="${SMOKE_REPLICAS:-3}"
OP_TIMEOUT="${OP_TIMEOUT:-3m}"
SKIP_BUILD="${SKIP_BUILD:-false}"

CLEAN=false
SMOKE_ONLY=false
KEEP_ENVIRONMENT="${KEEP_ENVIRONMENT:-0}"
STARTED_SWARM=false

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
  --clean             recreate the kind cluster and remove the benchmark Swarm service
  --smoke-only        use one round and three replicas by default
  --keep-environment  leave the kind cluster and script-created Swarm active

Environment overrides:
  ROUNDS             full-run rounds (default: 5)
  REPLICAS           full-run scale target (default: 20)
  SMOKE_ROUNDS       smoke rounds (default: 1)
  SMOKE_REPLICAS     smoke scale target (default: 3)
  OP_TIMEOUT         timeout per operation (default: 3m)
  K8S_CLUSTER        kind cluster name (default: bench-swarm)
  DOCKER_HOST_URI    Docker Engine API host (auto-detected from current context)
  SKIP_BUILD=true    use benchmarks/loadgen-scheduler/bin/loadgen-scheduler
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
  for cmd in docker kind kubectl jq; do
    if ! have_cmd "$cmd"; then
      echo "error: '$cmd' not found." >&2
      exit 1
    fi
  done
  if ! docker info >/dev/null 2>&1; then
    echo "error: Docker is not running." >&2
    exit 1
  fi
  if [ -z "$DOCKER_HOST_URI" ]; then
    DOCKER_HOST_URI=$(docker context inspect --format '{{.Endpoints.docker.Host}}' 2>/dev/null || true)
  fi
  if [ -z "$DOCKER_HOST_URI" ]; then
    DOCKER_HOST_URI="unix:///var/run/docker.sock"
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
    warn "Environment left running (kind=${K8S_CLUSTER}, Swarm active)."
    return
  fi
  kind delete cluster --name "$K8S_CLUSTER" >/dev/null 2>&1 || true
  docker service rm scheduler-bench >/dev/null 2>&1 || true
  if [ "$STARTED_SWARM" = true ]; then
    docker swarm leave --force >/dev/null 2>&1 || true
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

ensure_swarm() {
  local state
  state=$(docker info --format '{{.Swarm.LocalNodeState}}')
  if [ "$state" = "active" ]; then
    info "Using existing Docker Swarm manager"
    return
  fi
  if [ "$state" != "inactive" ]; then
    echo "error: Docker Swarm state is '$state', expected active or inactive" >&2
    exit 1
  fi
  info "Initializing single-node Docker Swarm..."
  docker swarm init >/dev/null
  STARTED_SWARM=true
}

run_benchmark() {
  local platform="$1" output="$2"
  info "Benchmarking $platform..."
  "$BENCH_BIN" \
    --platform "$platform" \
    --context "$K8S_CONTEXT" \
    --docker-host "$DOCKER_HOST_URI" \
    --rounds "$(selected_rounds)" \
    --replicas "$(selected_replicas)" \
    --timeout "$OP_TIMEOUT" \
    --out "$output"
}

render_summary() {
  local mode="full"
  if [ "$SMOKE_ONLY" = true ]; then mode="smoke"; fi
  jq -s --arg mode "$mode" --argjson rounds "$(selected_rounds)" --argjson replicas "$(selected_replicas)" '
    .[0] as $kubernetes | .[1] as $swarm | {
      schema_version: "results-summary/v1",
      experiment: {
        id: "kubernetes-vs-docker-swarm",
        name: "Kubernetes vs Docker Swarm",
        category: "orchestration",
        path: "experiments/orchestration/kubernetes-vs-docker-swarm"
      },
      run_id: ("kubernetes-vs-docker-swarm-" + ($kubernetes.timestamp | tostring)),
      timestamp: $kubernetes.timestamp,
      mode: $mode,
      config: {rounds: $rounds, replicas: $replicas},
      sources: [
        {name: "kubernetes", file: "results/kubernetes.json"},
        {name: "swarm", file: "results/swarm.json"}
      ],
      results: (
        [$kubernetes.results[] | . + {platform: "kubernetes"}] +
        [$swarm.results[] | . + {platform: "swarm"}]
      )
    }
  ' "$RESULTS_DIR/kubernetes.json" "$RESULTS_DIR/swarm.json" > "$RESULTS_DIR/summary.json"
  info "Summary saved to $RESULTS_DIR/summary.json"
}

mkdir -p "$RESULTS_DIR"
check_deps
build_binary
trap cleanup EXIT
ensure_kind
ensure_workload_image
ensure_swarm
if [ "$CLEAN" = true ]; then docker service rm scheduler-bench >/dev/null 2>&1 || true; fi
run_benchmark kubernetes "$RESULTS_DIR/kubernetes.json"
run_benchmark swarm "$RESULTS_DIR/swarm.json"
render_summary

for result in kubernetes.json swarm.json summary.json; do
  [ -s "$RESULTS_DIR/$result" ] || { echo "error: missing result $result" >&2; exit 1; }
done
info "Done. Results: $RESULTS_DIR"
