#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/.." && pwd)
EXPERIMENT_DIR="$REPO_ROOT/experiments/orchestration/kubernetes-vs-docker-swarm"

log() { echo "[smoke-kubernetes-vs-docker-swarm] $*"; }

log "Starting Kubernetes vs Docker Swarm smoke benchmark"
(
  cd "$EXPERIMENT_DIR"
  ./run.sh --smoke-only
)
log "Smoke benchmark completed"
