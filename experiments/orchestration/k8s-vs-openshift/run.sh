#!/usr/bin/env bash
# Experiment: Kubernetes vs OpenShift
# Usage: ./run.sh [--clean] [--smoke-only] [--keep-cluster]
#
# Environment overrides:
#   COUNT           (default: 1000)
#   ROUNDS          (default: 3)
#   REPLICAS        (default: 20)
#   OCP_CONTEXT     (optional OpenShift kubeconfig context)
#   SMOKE_COUNT     (default: 200)
#   SMOKE_ROUNDS    (default: 1)
#   SMOKE_REPLICAS  (default: 5)
#   SKIP_BUILD=true

set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/../../.." && pwd)
BENCH_DIR="$REPO_ROOT/benchmarks/loadgen-k8s"
RESULTS_DIR="$SCRIPT_DIR/results"
BENCH_BIN="$BENCH_DIR/bin/loadgen-k8s"

K8S_CLUSTER="${K8S_CLUSTER:-bench-k8s}"
K8S_CONTEXT="kind-${K8S_CLUSTER}"
OCP_CONTEXT="${OCP_CONTEXT:-}"

COUNT="${COUNT:-1000}"
ROUNDS="${ROUNDS:-3}"
REPLICAS="${REPLICAS:-20}"
SMOKE_COUNT="${SMOKE_COUNT:-200}"
SMOKE_ROUNDS="${SMOKE_ROUNDS:-1}"
SMOKE_REPLICAS="${SMOKE_REPLICAS:-5}"
SKIP_BUILD="${SKIP_BUILD:-false}"

CLEAN=false
SMOKE_ONLY=false
KEEP_CLUSTER="${KEEP_CLUSTER:-0}"

GREEN='\033[0;32m'; YELLOW='\033[1;33m'; NC='\033[0m'
info()  { echo -e "${GREEN}[run]${NC} $*"; }
warn()  { echo -e "${YELLOW}[warn]${NC} $*"; }
have_cmd() { command -v "$1" >/dev/null 2>&1; }

for arg in "$@"; do
  case "$arg" in
    --clean) CLEAN=true ;;
    --smoke-only) SMOKE_ONLY=true ;;
    --keep-cluster) KEEP_CLUSTER=1 ;;
    -h|--help)
      cat <<'EOF'
Usage: ./run.sh [--clean] [--smoke-only] [--keep-cluster]

Options:
  --clean         delete the existing kind cluster before the run
  --smoke-only    run a smaller kind benchmark
  --keep-cluster  leave the kind cluster running after the run

Environment overrides:
  COUNT
  ROUNDS
  REPLICAS
  OCP_CONTEXT
  SMOKE_COUNT
  SMOKE_ROUNDS
  SMOKE_REPLICAS
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

check_deps() {
  for cmd in kind kubectl; do
    if ! have_cmd "$cmd"; then
      echo "error: '$cmd' not found." >&2
      exit 1
    fi
  done
  if ! docker info >/dev/null 2>&1; then
    echo "error: Docker not running." >&2
    exit 1
  fi
}

build_binary() {
  if [ "$SKIP_BUILD" = true ]; then
    if [ ! -x "$BENCH_BIN" ]; then
      echo "error: SKIP_BUILD=true but binary is missing: $BENCH_BIN" >&2
      exit 1
    fi
    info "Using existing binary: $BENCH_BIN"
    return
  fi

  if have_cmd go; then
    info "Building loadgen-k8s..."
    mkdir -p "$BENCH_DIR/bin"
    (cd "$BENCH_DIR" && go build -o "$BENCH_BIN" .)
    info "Binary: $BENCH_BIN"
    return
  fi

  if [ -x "$BENCH_BIN" ]; then
    info "Go not found; using existing binary: $BENCH_BIN"
    return
  fi

  echo "error: 'go' not found and no prebuilt binary is available at $BENCH_BIN" >&2
  exit 1
}

cleanup_kind() {
  if [ "$KEEP_CLUSTER" = "1" ]; then
    warn "KEEP_CLUSTER=1 — cluster '${K8S_CLUSTER}' left running."
    return
  fi
  info "Deleting kind cluster '${K8S_CLUSTER}'..."
  kind delete cluster --name "$K8S_CLUSTER" >/dev/null 2>&1 || true
}

cleanup_on_exit() {
  cleanup_kind
}

ensure_kind_cluster() {
  if [ "$CLEAN" = true ]; then
    info "Removing existing kind cluster '${K8S_CLUSTER}'..."
    kind delete cluster --name "$K8S_CLUSTER" >/dev/null 2>&1 || true
  fi

  if kind get clusters 2>/dev/null | grep -q "^${K8S_CLUSTER}$"; then
    info "Kind cluster '${K8S_CLUSTER}' already exists."
    return
  fi

  info "Creating kind cluster '${K8S_CLUSTER}'..."
  kind create cluster --name "$K8S_CLUSTER" --config - <<'EOF'
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
  - role: control-plane
  - role: worker
EOF
}

run_bench() {
  local label="$1" ctx="$2" out="$3"
  local bench_count bench_rounds bench_replicas
  bench_count=$(selected_count)
  bench_rounds=$(selected_rounds)
  bench_replicas=$(selected_replicas)

  info "Benchmarking ${label} (context: ${ctx})..."
  "$BENCH_BIN" \
    --context "$ctx" \
    --op all \
    --count "$bench_count" \
    --rounds "$bench_rounds" \
    --replicas "$bench_replicas" \
    --out "$out"
}

selected_count() {
  if [ "$SMOKE_ONLY" = true ]; then
    echo "$SMOKE_COUNT"
  else
    echo "$COUNT"
  fi
}

selected_rounds() {
  if [ "$SMOKE_ONLY" = true ]; then
    echo "$SMOKE_ROUNDS"
  else
    echo "$ROUNDS"
  fi
}

selected_replicas() {
  if [ "$SMOKE_ONLY" = true ]; then
    echo "$SMOKE_REPLICAS"
  else
    echo "$REPLICAS"
  fi
}

render_summary() {
  if [ ! -s "$RESULTS_DIR/k8s.json" ] || [ ! -s "$RESULTS_DIR/ocp.json" ]; then
    return
  fi

  if ! have_cmd jq; then
    warn "jq not found — skipping side-by-side summary generation."
    return
  fi

  echo ""
  echo "════════════════════════════════════════════════════════════════════════════════"
  echo " Kubernetes vs OpenShift — side-by-side"
  echo "════════════════════════════════════════════════════════════════════════════════"

  local k8s_type ocp_type latency_ops scalar_ops op k8s_p50 ocp_p50 k8s_val ocp_val
  k8s_type=$(jq -r '.cluster_type' "$RESULTS_DIR/k8s.json")
  ocp_type=$(jq -r '.cluster_type' "$RESULTS_DIR/ocp.json")
  printf "%-35s  %12s  %12s\n" "Operation" "$k8s_type" "$ocp_type"
  printf "%-35s  %12s  %12s\n" "───────────────────────────────────" "────────────" "────────────"

  latency_ops=$(jq -r '.results[] | select(.p50_ms > 0) | .op' "$RESULTS_DIR/k8s.json" | sort -u)
  for op in $latency_ops; do
    k8s_p50=$(jq -r --arg op "$op" '.results[] | select(.op==$op) | .p50_ms' "$RESULTS_DIR/k8s.json" | head -1 | xargs printf "%.2f ms")
    ocp_p50=$(jq -r --arg op "$op" '.results[] | select(.op==$op) | .p50_ms' "$RESULTS_DIR/ocp.json" | head -1 | xargs printf "%.2f ms")
    printf "%-35s  %12s  %12s\n" "$op (p50)" "$k8s_p50" "$ocp_p50"
  done

  scalar_ops=$(jq -r '.results[] | select(.p50_ms == null or .p50_ms == 0) | .op' "$RESULTS_DIR/k8s.json" | sort -u)
  for op in $scalar_ops; do
    k8s_val=$(jq -r --arg op "$op" '.results[] | select(.op==$op) | "\(.value) \(.unit)"' "$RESULTS_DIR/k8s.json" | head -1)
    ocp_val=$(jq -r --arg op "$op" '.results[] | select(.op==$op) | "\(.value) \(.unit)"' "$RESULTS_DIR/ocp.json" | head -1)
    printf "%-35s  %12s  %12s\n" "$op" "$k8s_val" "$ocp_val"
  done
  echo "════════════════════════════════════════════════════════════════════════════════"

  jq -s '
    .[0] as $k8s
    | .[1] as $ocp
    | {
        schema_version: "results-summary/v1",
        experiment: {
          id: "k8s-vs-openshift",
          name: "Kubernetes vs OpenShift",
          category: "orchestration",
          path: "experiments/orchestration/k8s-vs-openshift"
        },
        run_id: ("k8s-vs-openshift-" + ($k8s.timestamp | tostring)),
        timestamp: $k8s.timestamp,
        mode: "full",
        config: {
          count: '"$(selected_count)"',
          rounds: '"$(selected_rounds)"',
          replicas: '"$(selected_replicas)"'
        },
        sources: [
          {name: "k8s", file: "results/k8s.json"},
          {name: "ocp", file: "results/ocp.json"}
        ],
        results: (
          [ $k8s.results[] | . + {cluster: $k8s.cluster_type} ] +
          [ $ocp.results[] | . + {cluster: $ocp.cluster_type} ]
        ),
        clusters: {
          k8s: $k8s,
          ocp: $ocp
        }
      }
  ' "$RESULTS_DIR/k8s.json" "$RESULTS_DIR/ocp.json" > "$RESULTS_DIR/summary.json"
  info "Summary saved to $RESULTS_DIR/summary.json"
}

verify_results() {
  if [ ! -s "$RESULTS_DIR/k8s.json" ]; then
    echo "error: expected results file is missing or empty: $RESULTS_DIR/k8s.json" >&2
    exit 1
  fi

  if [ -n "$OCP_CONTEXT" ] && [ ! -s "$RESULTS_DIR/ocp.json" ]; then
    echo "error: expected results file is missing or empty: $RESULTS_DIR/ocp.json" >&2
    exit 1
  fi

  if [ -n "$OCP_CONTEXT" ] && have_cmd jq && [ ! -s "$RESULTS_DIR/summary.json" ]; then
    echo "error: expected summary file is missing or empty: $RESULTS_DIR/summary.json" >&2
    exit 1
  fi
}

check_deps
build_binary
trap cleanup_on_exit EXIT
ensure_kind_cluster
mkdir -p "$RESULTS_DIR"

run_bench "Kubernetes (kind)" "$K8S_CONTEXT" "$RESULTS_DIR/k8s.json"

if [ -n "$OCP_CONTEXT" ]; then
  if have_cmd oc; then
    info "Granting anyuid SCC to bench namespace on OpenShift..."
    oc --context "$OCP_CONTEXT" adm policy add-scc-to-user anyuid \
      system:serviceaccount:bench-k8s:default >/dev/null 2>&1 || true
  fi
  run_bench "OpenShift" "$OCP_CONTEXT" "$RESULTS_DIR/ocp.json"
else
  warn "OCP_CONTEXT not set — skipping OpenShift benchmark."
  warn "To compare: OCP_CONTEXT=crc-admin ./run.sh"
fi

render_summary
verify_results

if [ "$SMOKE_ONLY" = true ]; then
  info "Smoke-only mode: done."
else
  info "Done. Results: $RESULTS_DIR"
fi
