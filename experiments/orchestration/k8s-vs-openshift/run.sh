#!/usr/bin/env bash
# Experiment: Kubernetes vs OpenShift
# Compares API latency, deploy/scale speed, and control-plane overhead.
#
# Requirements:
#   - kind       (https://kind.sigs.k8s.io)
#   - kubectl
#   - go >= 1.22
#   - jq         (for side-by-side table, optional)
#
# For OpenShift comparison (optional):
#   - OpenShift Local (CRC) running, OR any OCP cluster reachable via kubeconfig
#   - Set OCP_CONTEXT to the kubeconfig context name, e.g.:
#       OCP_CONTEXT=crc-admin ./run.sh
#
# Tuning:
#   COUNT=200      API latency iterations (default: 1000)
#   ROUNDS=3       Deploy/scale rounds   (default: 3)
#   REPLICAS=20    Max replicas          (default: 20)
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
BENCH_DIR="$REPO_ROOT/benchmarks/loadgen-k8s"
RESULTS_DIR="$(dirname "$0")/results"
BENCH_BIN="$BENCH_DIR/loadgen-k8s"

K8S_CLUSTER="bench-k8s"
K8S_CONTEXT="kind-${K8S_CLUSTER}"
OCP_CONTEXT="${OCP_CONTEXT:-}"

COUNT="${COUNT:-1000}"
ROUNDS="${ROUNDS:-3}"
REPLICAS="${REPLICAS:-20}"
SMOKE="${SMOKE_ONLY:-0}"

GREEN='\033[0;32m'; YELLOW='\033[1;33m'; NC='\033[0m'
info()  { echo -e "${GREEN}[run]${NC} $*"; }
warn()  { echo -e "${YELLOW}[warn]${NC} $*"; }

# ── prerequisites ─────────────────────────────────────────────────────────────
for cmd in kind kubectl go; do
  if ! command -v "$cmd" &>/dev/null; then
    echo "ERROR: $cmd not found. Please install it." >&2; exit 1
  fi
done

# ── build ─────────────────────────────────────────────────────────────────────
info "Building loadgen-k8s..."
(cd "$BENCH_DIR" && go mod tidy && go build -o loadgen-k8s .)

# ── kind cluster ──────────────────────────────────────────────────────────────
if ! kind get clusters 2>/dev/null | grep -q "^${K8S_CLUSTER}$"; then
  info "Creating kind cluster '${K8S_CLUSTER}'..."
  kind create cluster --name "$K8S_CLUSTER" --config - <<'EOF'
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
  - role: control-plane
  - role: worker
EOF
else
  info "Kind cluster '${K8S_CLUSTER}' already exists."
fi

mkdir -p "$RESULTS_DIR"

# ── helper: run benchmark against a context ───────────────────────────────────
run_bench() {
  local label="$1" ctx="$2" out="$3"
  local extra_args=""
  if [[ "$SMOKE" == "1" ]]; then
    extra_args="--count=200 --rounds=1 --replicas=5"
  else
    extra_args="--count=${COUNT} --rounds=${ROUNDS} --replicas=${REPLICAS}"
  fi
  info "Benchmarking ${label} (context: ${ctx})..."
  "$BENCH_BIN" \
    --context "$ctx" \
    --op all \
    $extra_args \
    --out "$out"
}

# ── k8s benchmark ─────────────────────────────────────────────────────────────
run_bench "Kubernetes (kind)" "$K8S_CONTEXT" "$RESULTS_DIR/k8s.json"

# ── openshift benchmark (optional) ────────────────────────────────────────────
if [[ -n "$OCP_CONTEXT" ]]; then
  # Grant anyuid SCC if oc is available (needed for the bench namespace)
  if command -v oc &>/dev/null; then
    info "Granting anyuid SCC to bench namespace on OpenShift..."
    oc --context "$OCP_CONTEXT" adm policy add-scc-to-user anyuid \
      system:serviceaccount:bench-k8s:default 2>/dev/null || true
  fi
  run_bench "OpenShift" "$OCP_CONTEXT" "$RESULTS_DIR/ocp.json"
else
  warn "OCP_CONTEXT not set — skipping OpenShift benchmark."
  warn "To compare: OCP_CONTEXT=crc-admin ./run.sh"
fi

# ── side-by-side table ────────────────────────────────────────────────────────
if command -v jq &>/dev/null && [[ -f "$RESULTS_DIR/k8s.json" && -f "$RESULTS_DIR/ocp.json" ]]; then
  echo ""
  echo "════════════════════════════════════════════════════════════════════════════════"
  echo " Kubernetes vs OpenShift — side-by-side"
  echo "════════════════════════════════════════════════════════════════════════════════"

  k8s_type=$(jq -r '.cluster_type' "$RESULTS_DIR/k8s.json")
  ocp_type=$(jq -r '.cluster_type'  "$RESULTS_DIR/ocp.json")
  printf "%-35s  %12s  %12s\n" "Operation" "$k8s_type" "$ocp_type"
  printf "%-35s  %12s  %12s\n" "───────────────────────────────────" "────────────" "────────────"

  # Latency ops: show p50_ms
  latency_ops=$(jq -r '.results[] | select(.p50_ms > 0) | .op' "$RESULTS_DIR/k8s.json" 2>/dev/null | sort -u)
  for op in $latency_ops; do
    k8s_p50=$(jq -r --arg op "$op" '.results[] | select(.op==$op) | .p50_ms' "$RESULTS_DIR/k8s.json" 2>/dev/null | head -1 | xargs printf "%.2f ms")
    ocp_p50=$(jq -r --arg op "$op" '.results[] | select(.op==$op) | .p50_ms' "$RESULTS_DIR/ocp.json"  2>/dev/null | head -1 | xargs printf "%.2f ms")
    printf "%-35s  %12s  %12s\n" "$op (p50)" "$k8s_p50" "$ocp_p50"
  done

  # Scalar ops: show value + unit
  scalar_ops=$(jq -r '.results[] | select(.p50_ms == null or .p50_ms == 0) | .op' "$RESULTS_DIR/k8s.json" 2>/dev/null | sort -u)
  for op in $scalar_ops; do
    k8s_val=$(jq -r --arg op "$op" '.results[] | select(.op==$op) | "\(.value) \(.unit)"' "$RESULTS_DIR/k8s.json" 2>/dev/null | head -1)
    ocp_val=$(jq -r  --arg op "$op" '.results[] | select(.op==$op) | "\(.value) \(.unit)"' "$RESULTS_DIR/ocp.json"  2>/dev/null | head -1)
    printf "%-35s  %12s  %12s\n" "$op" "$k8s_val" "$ocp_val"
  done
  echo "════════════════════════════════════════════════════════════════════════════════"

  jq -s '{k8s: .[0], ocp: .[1]}' "$RESULTS_DIR/k8s.json" "$RESULTS_DIR/ocp.json" \
    > "$RESULTS_DIR/summary.json"
  info "Summary saved to $RESULTS_DIR/summary.json"
elif [[ -f "$RESULTS_DIR/k8s.json" ]]; then
  info "Single-cluster run complete. Set OCP_CONTEXT to enable side-by-side comparison."
fi

# ── cleanup ───────────────────────────────────────────────────────────────────
if [[ "${KEEP_CLUSTER:-0}" != "1" ]]; then
  info "Deleting kind cluster '${K8S_CLUSTER}'..."
  kind delete cluster --name "$K8S_CLUSTER"
else
  warn "KEEP_CLUSTER=1 — cluster '${K8S_CLUSTER}' left running."
fi

info "Done."
