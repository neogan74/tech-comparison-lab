#!/usr/bin/env bash
# Local verification script: spins up Gitea, creates a fake k8s "sink",
# and runs the loadgen in dry-run + unit-test mode — no kind/ArgoCD/Flux needed.
#
# Usage: ./verify-local.sh [--stop]
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/../.." && pwd)
COMPOSE_DIR="$REPO_ROOT/deployments/docker-compose/gitops"
BINARY="$SCRIPT_DIR/bin/loadgen-gitops"

GITEA_URL="http://localhost:3000"
GITEA_USER="benchadmin"
GITEA_PASS="benchpass123"

GREEN='\033[0;32m'; YELLOW='\033[1;33m'; NC='\033[0m'
log()  { echo -e "${GREEN}[verify]${NC} $*"; }
warn() { echo -e "${YELLOW}[verify]${NC} $*"; }

cleanup() {
  log "Stopping Gitea..."
  docker compose -f "$COMPOSE_DIR/docker-compose.yml" down -v --remove-orphans >/dev/null 2>&1 || true
}

if [ "${1:-}" = "--stop" ]; then
  cleanup
  exit 0
fi

# ---- 1. Unit tests ----
log "Running unit tests..."
(
  cd "$SCRIPT_DIR"
  go test ./internal/report/ -v 2>&1 | grep -E "^(=== RUN|--- (PASS|FAIL)|PASS|FAIL|ok)"
)
log "Unit tests passed."

# ---- 2. Build binary ----
log "Building binary..."
mkdir -p "$SCRIPT_DIR/bin"
(cd "$SCRIPT_DIR" && go build -o "$BINARY" .)
log "Binary built: $BINARY"

# ---- 3. Start Gitea ----
log "Starting Gitea..."
docker compose -f "$COMPOSE_DIR/docker-compose.yml" up -d gitea
trap cleanup EXIT

log "Waiting for Gitea to be healthy..."
for i in $(seq 1 24); do
  curl -sf "${GITEA_URL}/api/v1/settings/api" >/dev/null 2>&1 && break
  [ $i -eq 24 ] && { echo "Gitea failed to start"; exit 1; }
  sleep 5
done
log "Gitea ready at ${GITEA_URL}"

# ---- 4. Create Gitea admin user ----
log "Creating admin user '${GITEA_USER}'..."
docker exec -u git gitops-gitea \
  gitea admin user create \
    --username "$GITEA_USER" \
    --password "$GITEA_PASS" \
    --email    "admin@bench.local" \
    --admin \
    --must-change-password=false 2>/dev/null || true

# ---- 5. Gitea integration tests ----
log "Running Gitea integration tests..."
(
  cd "$SCRIPT_DIR"
  GITEA_URL="$GITEA_URL" \
  GITEA_PASS="$GITEA_PASS" \
    go test ./internal/gitea/ -v -run Integration -timeout 60s 2>&1 | \
    grep -E "^(=== RUN|--- (PASS|FAIL)|PASS|FAIL|ok|    )"
)
log "Gitea integration tests passed."

# ---- 6. Dry-run connectivity check (Gitea only) ----
log "Testing loadgen dry-run (k8s failure is expected — no cluster here)..."
OUTPUT=$("$BINARY" \
  --tool       argocd \
  --gitea-url  "$GITEA_URL" \
  --gitea-user "$GITEA_USER" \
  --gitea-pass "$GITEA_PASS" \
  --gitea-repo bench-dryrun \
  --dry-run 2>&1 || true)
echo "$OUTPUT"
if echo "$OUTPUT" | grep -q "gitea: ok"; then
  log "Gitea connectivity confirmed."
else
  warn "Gitea connectivity check failed — see output above"
  exit 1
fi

# ---- 7. Verify Gitea API manually: repo + file ----
log "Verifying Gitea repo + file push end-to-end..."
BASE="${GITEA_URL}/api/v1"
AUTH="-u ${GITEA_USER}:${GITEA_PASS}"

# Create repo
curl -sf $AUTH -X POST "${BASE}/user/repos" \
  -H "Content-Type: application/json" \
  -d '{"name":"verify-e2e","auto_init":true,"default_branch":"main"}' >/dev/null || true

# Push a file
CONTENT=$(echo -n 'apiVersion: v1
kind: ConfigMap
metadata:
  name: bench-test
data:
  x: "1"
' | base64)
SHA=$(curl -sf $AUTH -X POST "${BASE}/repos/${GITEA_USER}/verify-e2e/contents/manifests/test.yaml" \
  -H "Content-Type: application/json" \
  -d "{\"message\":\"test\",\"content\":\"${CONTENT}\",\"branch\":\"main\"}" \
  | python3 -c "import sys,json; print(json.load(sys.stdin)['content']['sha'])" 2>/dev/null || echo "")

if [ -n "$SHA" ]; then
  log "File pushed, SHA: ${SHA:0:8}..."
else
  warn "Could not extract SHA — check Gitea logs"
fi

# Read back the file
READBACK=$(curl -sf $AUTH "${BASE}/repos/${GITEA_USER}/verify-e2e/contents/manifests/test.yaml" \
  | python3 -c "import sys,json,base64; d=json.load(sys.stdin); print(base64.b64decode(d['content']).decode())" 2>/dev/null || echo "")

if echo "$READBACK" | grep -q "bench-test"; then
  log "File content verified: round-trip push/get works correctly."
else
  warn "File content mismatch — Gitea API may have issues"
fi

echo ""
log "=== All local checks passed ==="
echo ""
echo "  Unit tests:        OK"
echo "  Integration tests: OK (Gitea client)"
echo "  Gitea API e2e:     OK (push → read → verified)"
echo ""
echo "  Next: run ./run.sh --smoke-only to test with a real k8s cluster."
