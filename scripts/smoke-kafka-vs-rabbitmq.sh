#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/.." && pwd)
COMPOSE_FILE="$REPO_ROOT/deployments/docker-compose/messaging/docker-compose.yml"
EXPERIMENT_DIR="$REPO_ROOT/experiments/messaging/kafka-vs-rabbitmq"

log() {
  echo "[smoke-kafka-rabbit] $*"
}

cleanup() {
  log "Stopping benchmark stack"
  docker compose -f "$COMPOSE_FILE" down -v --remove-orphans >/dev/null 2>&1 || true
}

trap cleanup EXIT

SMOKE_COUNT="${SMOKE_COUNT:-5000}"

log "Starting end-to-end smoke run"
(
  cd "$EXPERIMENT_DIR"
  SMOKE_COUNT="$SMOKE_COUNT" ./run.sh --clean --smoke-only
)

log "Smoke run completed"
