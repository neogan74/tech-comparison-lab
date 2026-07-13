#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/.." && pwd)
COMPOSE_FILE="$REPO_ROOT/deployments/docker-compose/observability/docker-compose.yml"
EXPERIMENT_DIR="$REPO_ROOT/experiments/observability/prometheus-vs-victoriametrics"

log() {
  echo "[smoke-prom-vm] $*"
}

cleanup() {
  log "Stopping benchmark stack"
  docker compose -f "$COMPOSE_FILE" down -v --remove-orphans >/dev/null 2>&1 || true
}

trap cleanup EXIT

SMOKE_COUNT="${SMOKE_COUNT:-20000}"
SMOKE_SERIES="${SMOKE_SERIES:-200}"
SMOKE_ITER="${SMOKE_ITER:-1}"

log "Starting end-to-end smoke run"
(
  cd "$EXPERIMENT_DIR"
  SMOKE_COUNT="$SMOKE_COUNT" SMOKE_SERIES="$SMOKE_SERIES" SMOKE_ITER="$SMOKE_ITER" ./run.sh --clean --smoke-only
)

log "Smoke run completed"
