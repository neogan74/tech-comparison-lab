#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/.." && pwd)
EXPERIMENT_DIR="$REPO_ROOT/experiments/orchestration/k8s-vs-openshift"

log() {
  echo "[smoke-k8s-ocp] $*"
}

SMOKE_COUNT="${SMOKE_COUNT:-100}"
SMOKE_ROUNDS="${SMOKE_ROUNDS:-1}"
SMOKE_REPLICAS="${SMOKE_REPLICAS:-3}"

log "Starting kind-only smoke run"
(
  cd "$EXPERIMENT_DIR"
  SMOKE_COUNT="$SMOKE_COUNT" \
  SMOKE_ROUNDS="$SMOKE_ROUNDS" \
  SMOKE_REPLICAS="$SMOKE_REPLICAS" \
  ./run.sh --clean --smoke-only
)

log "Smoke run completed"
