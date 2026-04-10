#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/.." && pwd)
EXPERIMENT_DIR="$REPO_ROOT/experiments/orchestration/k8s-vs-openshift"

log() {
  echo "[smoke-k8s-openshift] $*"
}

cleanup() {
  if [[ "${KEEP_CLUSTER:-0}" != "1" ]]; then
    log "Cleaning up kind cluster..."
    kind delete cluster --name bench-k8s 2>/dev/null || true
  fi
}

trap cleanup EXIT

log "Starting smoke test (kind only, no OpenShift)"
(
  cd "$EXPERIMENT_DIR"
  SMOKE_ONLY=1 ./run.sh
)

log "Smoke test completed"
