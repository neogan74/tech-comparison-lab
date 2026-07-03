#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/.." && pwd)
COMPOSE_FILE="$REPO_ROOT/deployments/docker-compose/gitops/docker-compose.yml"
EXPERIMENT_DIR="$REPO_ROOT/experiments/platform/argocd-vs-fluxcd"

log() { echo "[smoke-argocd-vs-fluxcd] $*"; }

cleanup() {
  log "Stopping Gitea stack"
  docker compose -f "$COMPOSE_FILE" down -v --remove-orphans >/dev/null 2>&1 || true
}

trap cleanup EXIT

SMOKE_COUNT="${SMOKE_COUNT:-3}"
SMOKE_BULK_SIZE="${SMOKE_BULK_SIZE:-3}"

log "Starting end-to-end smoke run (count=${SMOKE_COUNT}, bulk=${SMOKE_BULK_SIZE})"
(
  cd "$EXPERIMENT_DIR"
  SMOKE_COUNT="$SMOKE_COUNT" \
  SMOKE_BULK_SIZE="$SMOKE_BULK_SIZE" \
    ./run.sh --clean --smoke-only
)

log "Smoke run completed"
