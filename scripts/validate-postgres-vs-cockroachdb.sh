#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/.." && pwd)
TMP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/validate-pg-crdb.XXXXXX")

cleanup() {
  rm -rf "$TMP_DIR"
}

trap cleanup EXIT

log() {
  echo "[validate-pg-crdb] $*"
}

log "Checking experiment runner syntax"
bash -n "$REPO_ROOT/experiments/databases/postgres-vs-cockroachdb/run.sh"

log "Checking experiment runner help output"
"$REPO_ROOT/experiments/databases/postgres-vs-cockroachdb/run.sh" --help >/dev/null

log "Checking Docker Compose config"
docker compose -f "$REPO_ROOT/deployments/docker-compose/postgres-vs-cockroachdb/docker-compose.yml" config --quiet

log "Running loadgen-db tests"
(
  cd "$REPO_ROOT/benchmarks/loadgen-db"
  GOCACHE="$TMP_DIR/go-build" go test ./...
)

log "Validation completed"