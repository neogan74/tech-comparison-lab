#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/.." && pwd)
TMP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/validate-scylla-cassandra.XXXXXX")
trap 'rm -rf "$TMP_DIR"' EXIT

echo "[validate-scylla-cassandra] Checking runner syntax and help"
bash -n "$REPO_ROOT/experiments/databases/scylladb-vs-cassandra/run.sh"
"$REPO_ROOT/experiments/databases/scylladb-vs-cassandra/run.sh" --help >/dev/null

echo "[validate-scylla-cassandra] Checking Docker Compose config"
docker compose -f "$REPO_ROOT/deployments/docker-compose/scylladb-vs-cassandra/docker-compose.yml" config --quiet

echo "[validate-scylla-cassandra] Running loadgen-db tests"
(
  cd "$REPO_ROOT/benchmarks/loadgen-db"
  GOCACHE="$TMP_DIR/go-build" go test ./...
)
