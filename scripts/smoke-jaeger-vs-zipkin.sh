#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/.." && pwd)
COMPOSE_FILE="$REPO_ROOT/deployments/docker-compose/tracing/docker-compose.yml"
EXPERIMENT_DIR="$REPO_ROOT/experiments/observability/jaeger-vs-zipkin"

log() {
  echo "[smoke-jaeger-zipkin] $*"
}

cleanup() {
  log "Stopping benchmark stack"
  docker compose -f "$COMPOSE_FILE" down -v --remove-orphans >/dev/null 2>&1 || true
}

trap cleanup EXIT

SMOKE_COUNT="${SMOKE_COUNT:-5000}"
SMOKE_SERVICES="${SMOKE_SERVICES:-10}"
SMOKE_ITER="${SMOKE_ITER:-3}"

log "Starting end-to-end smoke run"
(
  cd "$EXPERIMENT_DIR"
  SMOKE_COUNT="$SMOKE_COUNT" SMOKE_SERVICES="$SMOKE_SERVICES" SMOKE_ITER="$SMOKE_ITER" ./run.sh --clean --smoke-only
)

log "Smoke run completed"
