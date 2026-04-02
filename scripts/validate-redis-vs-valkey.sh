#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/.." && pwd)
TMP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/validate-redis-valkey.XXXXXX")

cleanup() {
  rm -rf "$TMP_DIR"
}

trap cleanup EXIT

log() {
  echo "[validate-redis-valkey] $*"
}

log "Checking experiment runner syntax"
bash -n "$REPO_ROOT/experiments/cache/redis-vs-valkey/run.sh"

log "Checking experiment runner help output"
"$REPO_ROOT/experiments/cache/redis-vs-valkey/run.sh" --help >/dev/null

log "Checking Docker Compose config"
docker compose -f "$REPO_ROOT/deployments/docker-compose/cache/docker-compose.yml" config --quiet

log "Running loadgen-cache tests"
(
  cd "$REPO_ROOT/benchmarks/loadgen-cache"
  GOCACHE="$TMP_DIR/go-build" go test ./...
)

log "Validation completed"
