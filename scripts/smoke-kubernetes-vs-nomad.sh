#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/.." && pwd)
EXPERIMENT_DIR="$REPO_ROOT/experiments/orchestration/kubernetes-vs-nomad"

log() { echo "[smoke-kubernetes-vs-nomad] $*"; }

log "Starting Kubernetes vs Nomad smoke benchmark"
(
  cd "$EXPERIMENT_DIR"
  ./run.sh --smoke-only
)
log "Smoke benchmark completed"
