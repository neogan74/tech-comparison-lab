#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/.." && pwd)
TMP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/validate-istio-linkerd.XXXXXX")

trap 'rm -rf "$TMP_DIR"' EXIT

log() { echo "[validate-istio-vs-linkerd] $*"; }

log "Checking experiment runner syntax"
bash -n "$REPO_ROOT/experiments/mesh/istio-vs-linkerd/run.sh"

log "Checking experiment runner help output"
"$REPO_ROOT/experiments/mesh/istio-vs-linkerd/run.sh" --help >/dev/null

log "Running loadgen-mesh unit tests"
(
  cd "$REPO_ROOT/benchmarks/loadgen-mesh"
  GOCACHE="$TMP_DIR/go-build" go vet ./...
  GOCACHE="$TMP_DIR/go-build" go test ./...
)

log "Building loadgen-mesh binary"
(
  cd "$REPO_ROOT/benchmarks/loadgen-mesh"
  GOCACHE="$TMP_DIR/go-build" go build -o "$TMP_DIR/loadgen-mesh" .
)

log "Checking binary flag parsing"
mesh_out=$("$TMP_DIR/loadgen-mesh" --mesh bogus 2>&1 || true)
echo "$mesh_out" | grep -q "must be istio or linkerd" \
  || { echo "ERROR: --mesh validation missing" >&2; exit 1; }

log "Validation completed"
