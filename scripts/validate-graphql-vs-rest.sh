#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/.." && pwd)
TMP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/validate-graphql-rest.XXXXXX")

cleanup() {
  rm -rf "$TMP_DIR"
}

trap cleanup EXIT

log() {
  echo "[validate-graphql-rest] $*"
}

log "Checking experiment runner syntax"
bash -n "$REPO_ROOT/experiments/api/graphql-vs-rest/run.sh"

log "Checking experiment runner help output"
"$REPO_ROOT/experiments/api/graphql-vs-rest/run.sh" --help >/dev/null

log "Checking Docker Compose config"
docker compose -f "$REPO_ROOT/deployments/docker-compose/api/docker-compose.yml" config --quiet

log "Running bench-api tests"
(
  cd "$REPO_ROOT/apps/bench-api"
  GOCACHE="$TMP_DIR/go-build-api" go test ./...
)

log "Running loadgen-http tests"
(
  cd "$REPO_ROOT/benchmarks/loadgen-http"
  GOCACHE="$TMP_DIR/go-build-loadgen" go test ./...
)

log "Validation completed"