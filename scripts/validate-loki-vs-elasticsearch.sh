#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/.." && pwd)
TMP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/validate-loki-elasticsearch.XXXXXX")

cleanup() {
  rm -rf "$TMP_DIR"
}

trap cleanup EXIT

log() {
  echo "[validate-loki-elasticsearch] $*"
}

log "Checking experiment runner syntax"
bash -n "$REPO_ROOT/experiments/observability/loki-vs-elasticsearch/run.sh"

log "Checking experiment runner help output"
"$REPO_ROOT/experiments/observability/loki-vs-elasticsearch/run.sh" --help >/dev/null

log "Checking Docker Compose config"
docker compose -f "$REPO_ROOT/deployments/docker-compose/logs/docker-compose.yml" config --quiet

log "Running loadgen-logs tests"
(
  cd "$REPO_ROOT/benchmarks/loadgen-logs"
  GOCACHE="$TMP_DIR/go-build" go vet ./...
  GOCACHE="$TMP_DIR/go-build" go test ./...
)

log "Validation completed"
