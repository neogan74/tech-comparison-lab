#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/.." && pwd)
TMP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/validate-kong-tyk.XXXXXX")

cleanup() {
  rm -rf "$TMP_DIR"
}

trap cleanup EXIT

log() {
  echo "[validate-kong-tyk] $*"
}

log "Checking experiment runner syntax"
bash -n "$REPO_ROOT/experiments/api/kong-vs-tyk/run.sh"

log "Checking experiment runner help output"
"$REPO_ROOT/experiments/api/kong-vs-tyk/run.sh" --help >/dev/null

log "Checking Docker Compose config"
docker compose -f "$REPO_ROOT/deployments/docker-compose/kong-vs-tyk/docker-compose.yml" config --quiet

log "Checking Tyk API definition is valid JSON"
jq empty "$REPO_ROOT/deployments/docker-compose/kong-vs-tyk/tyk/apps/bench-api.json"
jq empty "$REPO_ROOT/deployments/docker-compose/kong-vs-tyk/tyk/tyk.conf"

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
