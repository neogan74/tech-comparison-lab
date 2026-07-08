#!/usr/bin/env bash
# Experiment: Argo CD vs Flux CD — GitOps sync latency benchmark
# Usage: ./run.sh [--clean] [--smoke-only] [--keep-cluster]
#
# Environment overrides:
#   COUNT             (default: 20)
#   BULK_SIZE         (default: 10)
#   SMOKE_COUNT       (default: 3)
#   SMOKE_BULK_SIZE   (default: 3)
#   SYNC_TIMEOUT      (default: 120s)
#   GITEA_PASS        (default: benchpass123)
#   ARGOCD_VERSION    (default: v2.13.3)
#   FLUX_VERSION      (default: v2.4.0)
#   SKIP_BUILD=true

set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/../../.." && pwd)
BENCH_DIR="$REPO_ROOT/benchmarks/loadgen-gitops"
COMPOSE_DIR="$REPO_ROOT/deployments/docker-compose/gitops"
RESULTS_DIR="$SCRIPT_DIR/results"
BENCH_BIN="$BENCH_DIR/bin/loadgen-gitops"

K8S_CLUSTER="${K8S_CLUSTER:-bench-gitops}"
K8S_CONTEXT="kind-${K8S_CLUSTER}"

COUNT="${COUNT:-20}"
BULK_SIZE="${BULK_SIZE:-10}"
SMOKE_COUNT="${SMOKE_COUNT:-3}"
SMOKE_BULK_SIZE="${SMOKE_BULK_SIZE:-3}"
SYNC_TIMEOUT="${SYNC_TIMEOUT:-120s}"
GITEA_PASS="${GITEA_PASS:-benchpass123}"
GITEA_USER="benchadmin"
GITEA_URL="http://localhost:3000"
ARGOCD_VERSION="${ARGOCD_VERSION:-v2.13.3}"
FLUX_VERSION="${FLUX_VERSION:-v2.4.0}"
SKIP_BUILD="${SKIP_BUILD:-false}"

CLEAN=false
SMOKE_ONLY=false
KEEP_CLUSTER="${KEEP_CLUSTER:-0}"

GREEN='\033[0;32m'; YELLOW='\033[1;33m'; RED='\033[0;31m'; NC='\033[0m'
log()  { echo -e "${GREEN}[run]${NC} $*"; }
warn() { echo -e "${YELLOW}[warn]${NC} $*"; }
die()  { echo -e "${RED}[error]${NC} $*" >&2; exit 1; }

have_cmd() { command -v "$1" >/dev/null 2>&1; }

selected_count()     { [ "$SMOKE_ONLY" = true ] && echo "$SMOKE_COUNT"     || echo "$COUNT"; }
selected_bulk_size() { [ "$SMOKE_ONLY" = true ] && echo "$SMOKE_BULK_SIZE" || echo "$BULK_SIZE"; }

for arg in "$@"; do
  case "$arg" in
    --clean)       CLEAN=true ;;
    --smoke-only)  SMOKE_ONLY=true ;;
    --keep-cluster) KEEP_CLUSTER=1 ;;
    -h|--help)
      cat <<'EOF'
Usage: ./run.sh [--clean] [--smoke-only] [--keep-cluster]

Options:
  --clean          remove existing kind cluster and Gitea volumes before run
  --smoke-only     run a fast smoke pass (SMOKE_COUNT iterations each tool)
  --keep-cluster   leave the kind cluster running after the run

Environment overrides:
  COUNT             full benchmark iterations (default: 20)
  BULK_SIZE         resources in bulk test (default: 10)
  SMOKE_COUNT       smoke iterations per tool (default: 3)
  SMOKE_BULK_SIZE   smoke bulk size (default: 3)
  SYNC_TIMEOUT      per-iteration timeout (default: 120s)
  GITEA_PASS        Gitea admin password (default: benchpass123)
  ARGOCD_VERSION    Argo CD release tag (default: v2.13.3)
  FLUX_VERSION      Flux CD release tag (default: v2.4.0)
  SKIP_BUILD=true   skip Go build if binary already exists
EOF
      exit 0
      ;;
    *) die "unknown argument '$arg'" ;;
  esac
done

# ---------------------------------------------------------------------------
# Dependency check
# ---------------------------------------------------------------------------
check_deps() {
  for cmd in docker kind kubectl jq flux; do
    if ! have_cmd "$cmd"; then
      die "'$cmd' not found — please install it before running this experiment."
    fi
  done
  if ! docker info >/dev/null 2>&1; then
    die "Docker daemon is not running."
  fi
  if ! docker compose version >/dev/null 2>&1; then
    die "Docker Compose v2 is required."
  fi
}

# ---------------------------------------------------------------------------
# Build the Go benchmark binary
# ---------------------------------------------------------------------------
build_binary() {
  if [ "$SKIP_BUILD" = true ]; then
    [ -x "$BENCH_BIN" ] || die "SKIP_BUILD=true but binary is missing: $BENCH_BIN"
    log "Using existing binary: $BENCH_BIN"
    return
  fi
  if have_cmd go; then
    log "Building loadgen-gitops..."
    mkdir -p "$BENCH_DIR/bin"
    (cd "$BENCH_DIR" && go build -o "$BENCH_BIN" .)
    log "Binary: $BENCH_BIN"
    return
  fi
  [ -x "$BENCH_BIN" ] && { log "Go not found; using existing binary."; return; }
  die "'go' not found and no prebuilt binary at $BENCH_BIN"
}

# ---------------------------------------------------------------------------
# Gitea stack
# ---------------------------------------------------------------------------
start_gitea() {
  log "Starting Gitea stack..."
  if [ "$CLEAN" = true ]; then
    log "  --clean: removing Gitea volumes"
    docker compose -f "$COMPOSE_DIR/docker-compose.yml" down -v --remove-orphans 2>/dev/null || true
  fi
  docker compose -f "$COMPOSE_DIR/docker-compose.yml" up -d gitea
}

wait_for_gitea() {
  log "Waiting for Gitea to be healthy..."
  local max=60 i=0
  until curl -sf "${GITEA_URL}/api/v1/settings/api" >/dev/null 2>&1; do
    i=$((i+1))
    [ $i -ge $max ] && die "Gitea did not become healthy after ${max}×5s"
    sleep 5
  done
  log "Gitea ready."
}

setup_gitea_user() {
  log "Creating Gitea admin user '${GITEA_USER}'..."
  docker exec gitops-gitea \
    gitea admin user create \
      --username "$GITEA_USER" \
      --password "$GITEA_PASS" \
      --email    "admin@bench.local" \
      --admin    \
      --must-change-password=false 2>/dev/null || true
  log "Gitea user ready."
}

# ---------------------------------------------------------------------------
# kind cluster
# ---------------------------------------------------------------------------
ensure_kind_cluster() {
  if [ "$CLEAN" = true ]; then
    log "Removing existing kind cluster '${K8S_CLUSTER}'..."
    kind delete cluster --name "$K8S_CLUSTER" 2>/dev/null || true
  fi

  if kind get clusters 2>/dev/null | grep -q "^${K8S_CLUSTER}$"; then
    log "Kind cluster '${K8S_CLUSTER}' already exists."
    return
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

cleanup_kind() {
  if [ "$KEEP_CLUSTER" = "1" ]; then
    warn "KEEP_CLUSTER=1 — kind cluster '${K8S_CLUSTER}' left running."
    return
  fi
  log "Deleting kind cluster '${K8S_CLUSTER}'..."
  kind delete cluster --name "$K8S_CLUSTER" 2>/dev/null || true
}

cleanup_gitea() {
  log "Stopping Gitea stack..."
  docker compose -f "$COMPOSE_DIR/docker-compose.yml" down -v --remove-orphans 2>/dev/null || true
}

# Attach Gitea to the kind Docker network so controllers inside the cluster
# can reach it by container hostname.
connect_gitea_to_kind_network() {
  log "Connecting Gitea container to kind network..."
  docker network connect kind gitops-gitea 2>/dev/null || true
}

# Return the IP address of the Gitea container on the kind network.
gitea_kind_ip() {
  docker inspect gitops-gitea \
    --format '{{.NetworkSettings.Networks.kind.IPAddress}}' 2>/dev/null
}

# ---------------------------------------------------------------------------
# Argo CD
# ---------------------------------------------------------------------------
install_argocd() {
  log "Installing Argo CD ${ARGOCD_VERSION}..."
  kubectl --context "$K8S_CONTEXT" create namespace argocd --dry-run=client -o yaml \
    | kubectl --context "$K8S_CONTEXT" apply -f -
  kubectl --context "$K8S_CONTEXT" apply -n argocd -f \
    "https://raw.githubusercontent.com/argoproj/argo-cd/${ARGOCD_VERSION}/manifests/install.yaml"

  # Lower the reconciliation interval to 10 s for a more responsive benchmark.
  kubectl --context "$K8S_CONTEXT" -n argocd patch configmap argocd-cm \
    --patch '{"data":{"timeout.reconciliation":"10s"}}' 2>/dev/null || true

  log "Waiting for Argo CD deployments to be ready (up to 5 min)..."
  for deploy in argocd-server argocd-repo-server argocd-application-controller; do
    kubectl --context "$K8S_CONTEXT" -n argocd rollout status \
      deployment/"$deploy" --timeout=300s 2>/dev/null || \
    kubectl --context "$K8S_CONTEXT" -n argocd rollout status \
      statefulset/"$deploy" --timeout=300s 2>/dev/null || true
  done
  # application-controller is a StatefulSet in newer versions
  kubectl --context "$K8S_CONTEXT" -n argocd wait pod \
    --for=condition=Ready --selector=app.kubernetes.io/component=application-controller \
    --timeout=300s 2>/dev/null || true
  log "Argo CD ready."
}

create_argocd_app() {
  local gitea_ip="$1"
  local repo_name="$2"
  local dest_ns="$3"

  log "Creating ArgoCD Application (repo=${repo_name}, dest=${dest_ns})..."
  kubectl --context "$K8S_CONTEXT" create namespace "$dest_ns" --dry-run=client -o yaml \
    | kubectl --context "$K8S_CONTEXT" apply -f -

  kubectl --context "$K8S_CONTEXT" apply -f - <<EOF
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: bench-${repo_name}
  namespace: argocd
spec:
  project: default
  source:
    repoURL: http://${gitea_ip}:3000/${GITEA_USER}/${repo_name}.git
    targetRevision: HEAD
    path: manifests
    directory:
      recurse: false
  destination:
    server: https://kubernetes.default.svc
    namespace: ${dest_ns}
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
    syncOptions:
      - CreateNamespace=true
EOF
  log "ArgoCD Application 'bench-${repo_name}' created."
}

uninstall_argocd() {
  log "Uninstalling Argo CD..."
  kubectl --context "$K8S_CONTEXT" delete namespace argocd --ignore-not-found --timeout=120s 2>/dev/null || true
  kubectl --context "$K8S_CONTEXT" delete crd \
    applications.argoproj.io applicationsets.argoproj.io appprojects.argoproj.io \
    --ignore-not-found 2>/dev/null || true
}

# ---------------------------------------------------------------------------
# Flux CD
# ---------------------------------------------------------------------------
install_fluxcd() {
  log "Installing Flux CD ${FLUX_VERSION}..."
  flux install \
    --namespace=flux-system \
    --context="$K8S_CONTEXT" \
    --components=source-controller,kustomize-controller \
    --timeout=5m

  # Grant kustomize-controller cluster-admin so it can manage resources in
  # any target namespace during the benchmark.
  kubectl --context "$K8S_CONTEXT" apply -f - <<'EOF'
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: flux-bench-admin
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: cluster-admin
subjects:
  - kind: ServiceAccount
    name: kustomize-controller
    namespace: flux-system
EOF
  log "Flux CD ready."
}

create_flux_source() {
  local gitea_ip="$1"
  local repo_name="$2"
  local dest_ns="$3"

  log "Creating Flux GitRepository + Kustomization (repo=${repo_name}, dest=${dest_ns})..."
  kubectl --context "$K8S_CONTEXT" create namespace "$dest_ns" --dry-run=client -o yaml \
    | kubectl --context "$K8S_CONTEXT" apply -f -

  flux create source git "$repo_name" \
    --context="$K8S_CONTEXT" \
    --url="http://${gitea_ip}:3000/${GITEA_USER}/${repo_name}" \
    --branch=main \
    --interval=10s \
    --namespace=flux-system \
    --timeout=2m

  flux create kustomization "bench-${repo_name}" \
    --context="$K8S_CONTEXT" \
    --source="GitRepository/${repo_name}" \
    --path="./manifests" \
    --prune=true \
    --interval=10s \
    --target-namespace="$dest_ns" \
    --namespace=flux-system \
    --timeout=2m

  log "Flux source and kustomization for '${repo_name}' created."
}

uninstall_fluxcd() {
  log "Uninstalling Flux CD..."
  flux uninstall \
    --namespace=flux-system \
    --context="$K8S_CONTEXT" \
    --silent 2>/dev/null || true
  kubectl --context "$K8S_CONTEXT" delete clusterrolebinding flux-bench-admin \
    --ignore-not-found 2>/dev/null || true
}

# ---------------------------------------------------------------------------
# Benchmark runner
# ---------------------------------------------------------------------------
run_bench() {
  local tool="$1"
  local repo_name="$2"
  local dest_ns="$3"
  local out="$4"
  local cnt bulk

  cnt=$(selected_count)
  bulk=$(selected_bulk_size)

  log "Running benchmark: tool=${tool} count=${cnt} bulk=${bulk}"
  "$BENCH_BIN" \
    --tool         "$tool" \
    --op           all \
    --count        "$cnt" \
    --bulk-size    "$bulk" \
    --context      "$K8S_CONTEXT" \
    --namespace    "$dest_ns" \
    --gitea-url    "$GITEA_URL" \
    --gitea-user   "$GITEA_USER" \
    --gitea-pass   "$GITEA_PASS" \
    --gitea-repo   "$repo_name" \
    --sync-timeout "$SYNC_TIMEOUT" \
    --out          "$out"
}

# ---------------------------------------------------------------------------
# Results
# ---------------------------------------------------------------------------
merge_results() {
  log "Merging results..."
  jq -s '{
    schema_version: "results-summary/v1",
    experiment: {
      id: "argocd-vs-fluxcd",
      name: "Argo CD vs Flux CD",
      category: "platform",
      path: "experiments/platform/argocd-vs-fluxcd"
    },
    run_id: ("argocd-vs-fluxcd-" + (.[0].timestamp | tostring)),
    timestamp: .[0].timestamp,
    mode: (if env.SMOKE_ONLY == "true" then "smoke" else "full" end),
    config: {
      count: '"$(selected_count)"',
      bulk_size: '"$(selected_bulk_size)"',
      sync_timeout: "'"$SYNC_TIMEOUT"'"
    },
    sources: [
      {name: "argocd", file: "results/argocd.json"},
      {name: "flux",   file: "results/flux.json"}
    ],
    results: [.[].results[]]
  }' "$RESULTS_DIR/argocd.json" "$RESULTS_DIR/flux.json" \
    > "$RESULTS_DIR/summary.json"
}

verify_results() {
  for f in "$RESULTS_DIR/argocd.json" "$RESULTS_DIR/flux.json" "$RESULTS_DIR/summary.json"; do
    [ -s "$f" ] || die "expected results file is missing or empty: $f"
  done
}

print_table() {
  log "--- Side-by-side comparison ---"
  echo ""
  printf "%-10s %-15s %10s %10s %10s %12s\n" \
    "Tool" "Operation" "p50(ms)" "p95(ms)" "p99(ms)" "total(ms)"
  printf "%-10s %-15s %10s %10s %10s %12s\n" \
    "----------" "---------------" "----------" "----------" "----------" "------------"
  jq -r '.results[] | [
    .tool, .op,
    (if .p50_ms? and .p50_ms > 0 then (.p50_ms|floor|tostring) else "-" end),
    (if .p95_ms? and .p95_ms > 0 then (.p95_ms|floor|tostring) else "-" end),
    (if .p99_ms? and .p99_ms > 0 then (.p99_ms|floor|tostring) else "-" end),
    (.total_ms|tostring)
  ] | @tsv' "$RESULTS_DIR/summary.json" | \
  while IFS=$'\t' read -r tool op p50 p95 p99 total; do
    printf "%-10s %-15s %10s %10s %10s %12s\n" \
      "$tool" "$op" "$p50" "$p95" "$p99" "$total"
  done
  echo ""
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
check_deps
build_binary
mkdir -p "$RESULTS_DIR"

start_gitea
wait_for_gitea
setup_gitea_user

ensure_kind_cluster
connect_gitea_to_kind_network
GITEA_KIND_IP=$(gitea_kind_ip)
[ -z "$GITEA_KIND_IP" ] && die "Could not detect Gitea IP on kind network"
log "Gitea reachable inside kind at: ${GITEA_KIND_IP}:3000"

trap 'cleanup_kind; cleanup_gitea' EXIT

# ---- Argo CD benchmark ----
log "=== Argo CD benchmark ==="
install_argocd
create_argocd_app "$GITEA_KIND_IP" "bench-argocd" "bench-argocd"

if [ "$SMOKE_ONLY" = true ]; then
  run_bench "argocd" "bench-argocd" "bench-argocd" "$RESULTS_DIR/argocd.json"
  log "Smoke-only: skipping full ArgoCD run."
else
  run_bench "argocd" "bench-argocd" "bench-argocd" "$RESULTS_DIR/argocd.json"
fi
uninstall_argocd

# ---- Flux CD benchmark ----
log "=== Flux CD benchmark ==="
install_fluxcd
create_flux_source "$GITEA_KIND_IP" "bench-flux" "bench-flux"

if [ "$SMOKE_ONLY" = true ]; then
  run_bench "flux" "bench-flux" "bench-flux" "$RESULTS_DIR/flux.json"
  log "Smoke-only: skipping full Flux run."
else
  run_bench "flux" "bench-flux" "bench-flux" "$RESULTS_DIR/flux.json"
fi
uninstall_fluxcd

# ---- Aggregate ----
merge_results
verify_results
print_table

log "Done. Results: $RESULTS_DIR/summary.json"
log "Gitea UI: ${GITEA_URL}"
