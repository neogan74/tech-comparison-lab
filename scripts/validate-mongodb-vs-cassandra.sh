#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/.." && pwd)
TMP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/validate-mongo-cassandra.XXXXXX")
trap 'rm -rf "$TMP_DIR"' EXIT

echo "[validate-mongo-cassandra] Checking runner syntax and help"
bash -n "$REPO_ROOT/experiments/databases/mongodb-vs-cassandra/run.sh"
"$REPO_ROOT/experiments/databases/mongodb-vs-cassandra/run.sh" --help >/dev/null

echo "[validate-mongo-cassandra] Checking Docker Compose config"
docker compose -f "$REPO_ROOT/deployments/docker-compose/mongodb-vs-cassandra/docker-compose.yml" config --quiet

echo "[validate-mongo-cassandra] Running loadgen-db tests"
(
  cd "$REPO_ROOT/benchmarks/loadgen-db"
  GOCACHE="$TMP_DIR/go-build" go test ./...
)
