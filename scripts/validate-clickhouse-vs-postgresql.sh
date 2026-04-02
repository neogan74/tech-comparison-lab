#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/.." && pwd)
TMP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/validate-ch-pg.XXXXXX")

cleanup() {
  rm -rf "$TMP_DIR"
}

trap cleanup EXIT

log() {
  echo "[validate-ch-pg] $*"
}

log "Checking experiment runner syntax"
bash -n "$REPO_ROOT/experiments/analytics/clickhouse-vs-postgresql/run.sh"

log "Checking experiment runner help output"
"$REPO_ROOT/experiments/analytics/clickhouse-vs-postgresql/run.sh" --help >/dev/null

log "Checking Docker Compose config"
docker compose -f "$REPO_ROOT/deployments/docker-compose/analytics/docker-compose.yml" config --quiet

log "Running loadgen-analytics tests"
(
  cd "$REPO_ROOT/benchmarks/loadgen-analytics"
  GOCACHE="$TMP_DIR/go-build" go test ./...
)

log "Validation completed"
