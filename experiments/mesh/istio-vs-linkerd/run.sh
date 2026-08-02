#!/usr/bin/env bash
# Experiment: Istio vs Linkerd — service mesh injection, footprint & data-plane
# Usage: ./run.sh [--clean] [--smoke-only] [--keep-cluster]
#
# Environment overrides:
#   REPLICAS        (default: 3    — echo replicas for the inject benchmark)
#   ROUNDS          (default: 3    — inject benchmark rounds)
#   COUNT           (default: 5000 — data-plane requests)
#   WORKERS         (default: 25   — data-plane concurrency)
#   SMOKE_REPLICAS  (default: 1)
#   SMOKE_ROUNDS    (default: 1)
#   SMOKE_COUNT     (default: 200)
#   SMOKE_WORKERS   (default: 5)
#   ISTIO_VERSION   (default: 1.24.2)
#   SKIP_BUILD=true

set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/../../.." && pwd)
BENCH_DIR="$REPO_ROOT/benchmarks/loadgen-mesh"
BENCH_BIN="$BENCH_DIR/bin/loadgen-mesh"
RESULTS_DIR="$SCRIPT_DIR/results"

K8S_CLUSTER="${K8S_CLUSTER:-bench-mesh}"
K8S_CONTEXT="kind-${K8S_CLUSTER}"

REPLICAS="${REPLICAS:-3}"
ROUNDS="${ROUNDS:-3}"
COUNT="${COUNT:-5000}"
WORKERS="${WORKERS:-25}"
SMOKE_REPLICAS="${SMOKE_REPLICAS:-1}"
SMOKE_ROUNDS="${SMOKE_ROUNDS:-1}"
SMOKE_COUNT="${SMOKE_COUNT:-200}"
SMOKE_WORKERS="${SMOKE_WORKERS:-5}"
ISTIO_VERSION="${ISTIO_VERSION:-1.24.2}"
GATEWAY_API_VERSION="${GATEWAY_API_VERSION:-1.2.1}"
SKIP_BUILD="${SKIP_BUILD:-false}"

BASELINE_NS="bench-baseline"
PF_MESHED_PORT=18080
PF_BASE_PORT=18081

CLEAN=false
SMOKE_ONLY=false
KEEP_CLUSTER="${KEEP_CLUSTER:-0}"
PF_PID=""

GREEN='\033[0;32m'; YELLOW='\033[1;33m'; RED='\033[0;31m'; NC='\033[0m'
log()  { echo -e "${GREEN}[run]${NC} $*"; }
warn() { echo -e "${YELLOW}[warn]${NC} $*"; }
die()  { echo -e "${RED}[error]${NC} $*" >&2; exit 1; }
have_cmd() { command -v "$1" >/dev/null 2>&1; }

for arg in "$@"; do
  case "$arg" in
    --clean)        CLEAN=true ;;
    --smoke-only)   SMOKE_ONLY=true ;;
    --keep-cluster) KEEP_CLUSTER=1 ;;
    -h|--help)
      cat <<'EOF'
Usage: ./run.sh [--clean] [--smoke-only] [--keep-cluster]

Options:
  --clean          delete any existing kind cluster before the run
  --smoke-only     run a fast smoke pass (small sizes)
  --keep-cluster   leave the kind cluster running after the run

Environment overrides:
  REPLICAS ROUNDS COUNT WORKERS
  SMOKE_REPLICAS SMOKE_ROUNDS SMOKE_COUNT SMOKE_WORKERS
  ISTIO_VERSION        Istio release (default: 1.24.2)
  GATEWAY_API_VERSION  Gateway API CRD release for Linkerd (default: 1.2.1)
  SKIP_BUILD=true      skip go build if the binary already exists
EOF
      exit 0
      ;;
    *) die "unknown argument '$arg'" ;;
  esac
done

if [ "$SMOKE_ONLY" = true ]; then
  REPLICAS="$SMOKE_REPLICAS"; ROUNDS="$SMOKE_ROUNDS"
  COUNT="$SMOKE_COUNT"; WORKERS="$SMOKE_WORKERS"
fi

# ---------------------------------------------------------------------------
check_deps() {
  for cmd in docker kind kubectl istioctl linkerd jq; do
    have_cmd "$cmd" || die "'$cmd' not found — install it before running this experiment."
  done
  docker info >/dev/null 2>&1 || die "Docker daemon is not running."
}

build_binary() {
  if [ "$SKIP_BUILD" = true ]; then
    [ -x "$BENCH_BIN" ] || die "SKIP_BUILD=true but binary is missing: $BENCH_BIN"
    log "Using existing binary: $BENCH_BIN"; return
  fi
  if have_cmd go; then
    log "Building loadgen-mesh..."
    mkdir -p "$BENCH_DIR/bin"
    (cd "$BENCH_DIR" && go build -o "$BENCH_BIN" .)
    log "Binary: $BENCH_BIN"; return
  fi
  [ -x "$BENCH_BIN" ] && { log "Go not found; using existing binary."; return; }
  die "'go' not found and no prebuilt binary at $BENCH_BIN"
}

# ---------------------------------------------------------------------------
ensure_kind_cluster() {
  if [ "$CLEAN" = true ]; then
    log "Removing existing kind cluster '${K8S_CLUSTER}'..."
    kind delete cluster --name "$K8S_CLUSTER" 2>/dev/null || true
  fi
  if kind get clusters 2>/dev/null | grep -q "^${K8S_CLUSTER}$"; then
    log "Kind cluster '${K8S_CLUSTER}' already exists."; return
  fi
  log "Creating kind cluster '${K8S_CLUSTER}'..."
  kind create cluster --name "$K8S_CLUSTER" --config - <<'KINDEOF'
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
  - role: control-plane
  - role: worker
KINDEOF
}

install_istio() {
  log "Installing Istio (minimal profile)..."
  istioctl install --context "$K8S_CONTEXT" --set profile=minimal -y
  kubectl --context "$K8S_CONTEXT" -n istio-system rollout status deploy/istiod --timeout=300s
  log "Istio ready."
}

install_linkerd() {
  log "Installing Gateway API CRDs (Linkerd prerequisite)..."
  kubectl --context "$K8S_CONTEXT" apply --server-side -f \
    "https://github.com/kubernetes-sigs/gateway-api/releases/download/v${GATEWAY_API_VERSION}/standard-install.yaml"

  log "Installing Linkerd (CRDs + control plane)..."
  linkerd install --crds | kubectl --context "$K8S_CONTEXT" apply -f -
  linkerd install | kubectl --context "$K8S_CONTEXT" apply -f -
  kubectl --context "$K8S_CONTEXT" -n linkerd rollout status deploy --timeout=300s
  log "Linkerd ready."
}

ensure_baseline() {
  log "Deploying baseline echo (no sidecar) in ${BASELINE_NS}..."
  kubectl --context "$K8S_CONTEXT" create namespace "$BASELINE_NS" \
    --dry-run=client -o yaml | kubectl --context "$K8S_CONTEXT" apply -f - >/dev/null
  kubectl --context "$K8S_CONTEXT" -n "$BASELINE_NS" apply -f - <<'YAMLEOF' >/dev/null
apiVersion: apps/v1
kind: Deployment
metadata:
  name: echo
  labels: { app: echo }
spec:
  replicas: 1
  selector: { matchLabels: { app: echo } }
  template:
    metadata: { labels: { app: echo } }
    spec:
      containers:
        - name: echo
          image: nginx:1.27-alpine
          ports: [{ containerPort: 80 }]
---
apiVersion: v1
kind: Service
metadata:
  name: echo
spec:
  selector: { app: echo }
  ports: [{ port: 80, targetPort: 80 }]
YAMLEOF
  kubectl --context "$K8S_CONTEXT" -n "$BASELINE_NS" rollout status deploy/echo --timeout=180s
}

start_pf() {
  local ns="$1" localport="$2"
  kubectl --context "$K8S_CONTEXT" -n "$ns" port-forward svc/echo "${localport}:80" >/dev/null 2>&1 &
  PF_PID=$!
  sleep 2
}
stop_pf() {
  [ -n "$PF_PID" ] && kill "$PF_PID" 2>/dev/null || true
  PF_PID=""
}

# Run inject + footprint + data-plane (meshed & baseline) for one mesh,
# accumulating partial JSON files, then merge into results/<mesh>.json.
run_mesh() {
  local mesh="$1" ns="bench-$1"
  local tmp; tmp=$(mktemp -d "${TMPDIR:-/tmp}/mesh-${mesh}.XXXXXX")

  log "[$mesh] inject benchmark (replicas=$REPLICAS rounds=$ROUNDS)..."
  "$BENCH_BIN" --context "$K8S_CONTEXT" --mesh "$mesh" --namespace "$ns" \
    --op inject --replicas "$REPLICAS" --rounds "$ROUNDS" --out "$tmp/inject.json"

  log "[$mesh] footprint benchmark..."
  "$BENCH_BIN" --context "$K8S_CONTEXT" --mesh "$mesh" --namespace "$ns" \
    --op footprint --out "$tmp/footprint.json"

  log "[$mesh] data-plane (meshed) benchmark..."
  start_pf "$ns" "$PF_MESHED_PORT"
  "$BENCH_BIN" --mesh "$mesh" --op data-plane --label meshed \
    --addr "http://localhost:${PF_MESHED_PORT}" --count "$COUNT" --workers "$WORKERS" \
    --out "$tmp/dp-meshed.json"
  stop_pf

  log "[$mesh] data-plane (baseline) benchmark..."
  start_pf "$BASELINE_NS" "$PF_BASE_PORT"
  "$BENCH_BIN" --mesh "$mesh" --op data-plane --label baseline \
    --addr "http://localhost:${PF_BASE_PORT}" --count "$COUNT" --workers "$WORKERS" \
    --out "$tmp/dp-baseline.json"
  stop_pf

  mkdir -p "$RESULTS_DIR"
  jq -s '{
    mesh: .[0].mesh,
    timestamp: .[0].timestamp,
    results: [.[].results[]]
  }' "$tmp/inject.json" "$tmp/footprint.json" "$tmp/dp-meshed.json" "$tmp/dp-baseline.json" \
    > "$RESULTS_DIR/${mesh}.json"
  rm -rf "$tmp"
  log "[$mesh] results → $RESULTS_DIR/${mesh}.json"
}

merge_results() {
  log "Merging results..."
  jq -s '{
    schema_version: "results-summary/v1",
    experiment: {
      id: "istio-vs-linkerd",
      name: "Istio vs Linkerd",
      category: "mesh",
      path: "experiments/mesh/istio-vs-linkerd"
    },
    run_id: ("istio-vs-linkerd-" + (.[0].timestamp | tostring)),
    timestamp: .[0].timestamp,
    mode: "'"$([ "$SMOKE_ONLY" = true ] && echo smoke || echo full)"'",
    config: {
      replicas: '"$REPLICAS"',
      rounds: '"$ROUNDS"',
      count: '"$COUNT"',
      workers: '"$WORKERS"'
    },
    sources: [
      {name: "istio",   file: "results/istio.json"},
      {name: "linkerd", file: "results/linkerd.json"}
    ],
    results: [.[].results[]]
  }' "$RESULTS_DIR/istio.json" "$RESULTS_DIR/linkerd.json" \
    > "$RESULTS_DIR/summary.json"
}

verify_results() {
  for f in "$RESULTS_DIR/istio.json" "$RESULTS_DIR/linkerd.json" "$RESULTS_DIR/summary.json"; do
    [ -s "$f" ] || die "expected results file is missing or empty: $f"
  done
}

print_table() {
  log "--- Side-by-side comparison ---"
  echo ""
  printf "%-8s %-32s %10s %10s %10s %12s\n" "Mesh" "Operation" "p50(ms)" "p95(ms)" "p99(ms)" "Value"
  printf "%-8s %-32s %10s %10s %10s %12s\n" "----" "--------------------------------" "--------" "--------" "--------" "------------"
  jq -r '.results[] | [
    .mesh, .op,
    (.p50_ms // 0 | tostring), (.p95_ms // 0 | tostring), (.p99_ms // 0 | tostring),
    (if .value != null then (.value|tostring) + " " + (.unit // "") else "-" end)
  ] | @tsv' "$RESULTS_DIR/summary.json" | \
  while IFS=$'\t' read -r mesh op p50 p95 p99 val; do
    printf "%-8s %-32s %10s %10s %10s %12s\n" "$mesh" "$op" "$p50" "$p95" "$p99" "$val"
  done
  echo ""
}

cleanup() {
  stop_pf
  if [ "$KEEP_CLUSTER" = "1" ]; then
    warn "KEEP_CLUSTER=1 — kind cluster '${K8S_CLUSTER}' left running."
    return
  fi
  log "Deleting kind cluster '${K8S_CLUSTER}'..."
  kind delete cluster --name "$K8S_CLUSTER" 2>/dev/null || true
}
trap cleanup EXIT

# --- Main ---
check_deps
build_binary
ensure_kind_cluster
install_istio
install_linkerd
ensure_baseline

run_mesh istio
run_mesh linkerd

merge_results
verify_results
print_table

log "Done. Results: $RESULTS_DIR/summary.json"
