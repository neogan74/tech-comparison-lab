#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/.." && pwd)
COMPOSE_FILE="$REPO_ROOT/deployments/docker-compose/mongodb-vs-cassandra/docker-compose.yml"
EXPERIMENT_DIR="$REPO_ROOT/experiments/databases/mongodb-vs-cassandra"

cleanup() {
  docker compose -f "$COMPOSE_FILE" down -v --remove-orphans >/dev/null 2>&1 || true
}
trap cleanup EXIT

echo "[smoke-mongo-cassandra] Starting end-to-end smoke run"
(
  cd "$EXPERIMENT_DIR"
  SMOKE_COUNT="${SMOKE_COUNT:-500}" ./run.sh --clean --smoke-only
)
