#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/.." && pwd)
TMP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/validate-mysql-pg.XXXXXX")

cleanup() {
  rm -rf "$TMP_DIR"
}

trap cleanup EXIT

log() {
  echo "[validate-mysql-pg] $*"
}

log "Checking experiment runner syntax"
bash -n "$REPO_ROOT/experiments/databases/mysql-vs-postgres/run.sh"

log "Checking experiment runner help output"
"$REPO_ROOT/experiments/databases/mysql-vs-postgres/run.sh" --help >/dev/null

log "Checking Docker Compose config"
docker compose -f "$REPO_ROOT/deployments/docker-compose/mysql-vs-postgres/docker-compose.yml" config --quiet

log "Running loadgen-db tests"
(
  cd "$REPO_ROOT/benchmarks/loadgen-db"
  GOCACHE="$TMP_DIR/go-build" go test ./...
)

log "Validation completed"