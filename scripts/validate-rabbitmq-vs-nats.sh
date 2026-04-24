#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/.." && pwd)
TMP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/validate-rabbitmq-vs-nats.XXXXXX")

cleanup() {
  rm -rf "$TMP_DIR"
}

trap cleanup EXIT

log() {
  echo "[validate-rabbitmq-vs-nats] $*"
}

log "Checking experiment runner syntax"
bash -n "$REPO_ROOT/experiments/messaging/rabbitmq-vs-nats/run.sh"

log "Checking experiment runner help output"
"$REPO_ROOT/experiments/messaging/rabbitmq-vs-nats/run.sh" --help >/dev/null

log "Checking Docker Compose config"
docker compose -f "$REPO_ROOT/deployments/docker-compose/rabbitmq-vs-nats/docker-compose.yml" config --quiet

log "Running loadgen-msg tests"
(
  cd "$REPO_ROOT/benchmarks/loadgen-msg"
  GOCACHE="$TMP_DIR/go-build" go test ./...
)

log "Validation completed"
