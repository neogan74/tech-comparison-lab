#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/.." && pwd)
TMP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/validate-kafka-vs-pulsar.XXXXXX")

cleanup() {
  rm -rf "$TMP_DIR"
}

trap cleanup EXIT

log() {
  echo "[validate-kafka-vs-pulsar] $*"
}

log "Checking experiment runner syntax"
bash -n "$REPO_ROOT/experiments/messaging/kafka-vs-pulsar/run.sh"

log "Checking experiment runner help output"
"$REPO_ROOT/experiments/messaging/kafka-vs-pulsar/run.sh" --help >/dev/null

log "Checking Docker Compose config"
docker compose -f "$REPO_ROOT/deployments/docker-compose/kafka-vs-pulsar/docker-compose.yml" config --quiet

log "Running loadgen-msg tests"
(
  cd "$REPO_ROOT/benchmarks/loadgen-msg"
  GOCACHE="$TMP_DIR/go-build" go test ./...
)

log "Validation completed"
