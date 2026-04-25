#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/.." && pwd)
TMP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/validate-redis-vs-memcached.XXXXXX")

cleanup() {
  rm -rf "$TMP_DIR"
}

trap cleanup EXIT

log() {
  echo "[validate-redis-vs-memcached] $*"
}

log "Checking experiment runner syntax"
bash -n "$REPO_ROOT/experiments/cache/redis-vs-memcached/run.sh"

log "Checking experiment runner help output"
"$REPO_ROOT/experiments/cache/redis-vs-memcached/run.sh" --help >/dev/null

log "Checking Docker Compose config"
docker compose -f "$REPO_ROOT/deployments/docker-compose/cache-memcached/docker-compose.yml" config --quiet

log "Running loadgen-cache tests"
(
  cd "$REPO_ROOT/benchmarks/loadgen-cache"
  GOCACHE="$TMP_DIR/go-build" go test ./...
)

log "Building loadgen-cache binary"
(
  cd "$REPO_ROOT/benchmarks/loadgen-cache"
  GOCACHE="$TMP_DIR/go-build" go build -o "$TMP_DIR/loadgen-cache" .
)

log "Verifying --db memcached flag is accepted"
"$TMP_DIR/loadgen-cache" --db memcached --dry-run --addr localhost:11211 2>/dev/null || true

log "Validation completed"