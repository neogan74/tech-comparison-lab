#!/usr/bin/env bash
# Experiment #8: Kafka vs RabbitMQ messaging benchmark
# Usage: ./run.sh [--clean] [--smoke-only]
#
# Environment overrides:
#   MSG_COUNT       (default: 1000000)
#   BATCH_SIZE      (default: 1000)
#   CONSUMERS       (default: 3)
#   PARTITIONS      (default: 3)

set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/../../.." && pwd)
COMPOSE_DIR="$REPO_ROOT/deployments/docker-compose/messaging"
LOADGEN_DIR="$REPO_ROOT/benchmarks/loadgen-msg"
RESULTS_DIR="$SCRIPT_DIR/results"
BINARY="$LOADGEN_DIR/bin/loadgen-msg"

MSG_COUNT="${MSG_COUNT:-1000000}"
BATCH_SIZE="${BATCH_SIZE:-1000}"
CONSUMERS="${CONSUMERS:-3}"
PARTITIONS="${PARTITIONS:-3}"
SMOKE_COUNT="${SMOKE_COUNT:-10000}"
SKIP_BUILD="${SKIP_BUILD:-false}"

KAFKA_ADDR="localhost:9093"
RABBIT_ADDR="amqp://bench:benchpass@localhost:5672/"

CLEAN=false
SMOKE_ONLY=false
for arg in "$@"; do
  case $arg in
    --clean)      CLEAN=true ;;
    --smoke-only) SMOKE_ONLY=true ;;
    -h|--help)
      cat <<'EOF'
Usage: ./run.sh [--clean] [--smoke-only]

Options:
  --clean       remove existing volumes before starting the stack
  --smoke-only  run the smoke benchmark only

Environment overrides:
  MSG_COUNT
  BATCH_SIZE
  CONSUMERS
  PARTITIONS
  SMOKE_COUNT
  SKIP_BUILD=true
EOF
      exit 0
      ;;
    *)
      echo "error: unknown argument '$arg'" >&2
      exit 1
      ;;
  esac
done

log() { echo "[$(date '+%H:%M:%S')] $*"; }
have_cmd() { command -v "$1" >/dev/null 2>&1; }

check_deps() {
  for cmd in docker jq; do
    if ! command -v "$cmd" &>/dev/null; then
      echo "error: '$cmd' not found." >&2; exit 1
    fi
  done
  docker compose version &>/dev/null || { echo "error: Docker Compose v2 is required." >&2; exit 1; }
  docker info &>/dev/null || { echo "error: Docker not running." >&2; exit 1; }
}

build_binary() {
  if [ "$SKIP_BUILD" = true ]; then
    if [ ! -x "$BINARY" ]; then
      echo "error: SKIP_BUILD=true but binary is missing: $BINARY" >&2
      exit 1
    fi
    log "Using existing binary: $BINARY"
    return
  fi

  if have_cmd go; then
    log "Building loadgen-msg..."
    mkdir -p "$LOADGEN_DIR/bin"
    (cd "$LOADGEN_DIR" && go build -o "$BINARY" .)
    log "Binary: $BINARY"
    return
  fi

  if [ -x "$BINARY" ]; then
    log "Go not found; using existing binary: $BINARY"
    return
  fi

  echo "error: 'go' not found and no prebuilt binary is available at $BINARY" >&2
  exit 1
}

start_stack() {
  log "Starting Docker Compose stack (messaging)..."
  if [ "$CLEAN" = true ]; then
    docker compose -f "$COMPOSE_DIR/docker-compose.yml" down -v --remove-orphans 2>/dev/null || true
  fi
  docker compose -f "$COMPOSE_DIR/docker-compose.yml" up -d
}

wait_for_kafka() {
  log "Waiting for Kafka (port 9093)..."
  local max=30 i=0
  until docker compose -f "$COMPOSE_DIR/docker-compose.yml" exec -T kafka \
      /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 --list &>/dev/null; do
    i=$((i+1))
    [ $i -ge $max ] && { echo "error: Kafka not ready after 300s" >&2; exit 1; }
    sleep 10
  done
  log "Kafka ready."
}

wait_for_rabbitmq() {
  log "Waiting for RabbitMQ..."
  local max=60 i=0 cid status
  until cid=$(docker compose -f "$COMPOSE_DIR/docker-compose.yml" ps -q rabbitmq 2>/dev/null) \
        && [ -n "$cid" ] \
        && status=$(docker inspect --format='{{.State.Health.Status}}' "$cid" 2>/dev/null) \
        && [ "$status" = "healthy" ]; do
    i=$((i+1))
    [ $i -ge $max ] && { echo "error: RabbitMQ not ready after 300s" >&2; exit 1; }
    sleep 5
  done
  log "RabbitMQ ready."
}

smoke_test() {
  log "--- Smoke test (${SMOKE_COUNT} messages each) ---"
  "$BINARY" --db kafka    --op all --count "$SMOKE_COUNT" --batch 100 \
    --consumers "$CONSUMERS" --partitions "$PARTITIONS" --clean \
    --addr "$KAFKA_ADDR"
  "$BINARY" --db rabbitmq --op all --count "$SMOKE_COUNT" --batch 100 \
    --consumers "$CONSUMERS" --clean \
    --addr "$RABBIT_ADDR"
  log "Smoke test passed."
}

run_full() {
  log "--- Full benchmark: MSG_COUNT=$MSG_COUNT ---"
  mkdir -p "$RESULTS_DIR"

  log "Running Kafka benchmark..."
  "$BINARY" \
    --db         kafka \
    --op         all \
    --count      "$MSG_COUNT" \
    --batch      "$BATCH_SIZE" \
    --consumers  "$CONSUMERS" \
    --partitions "$PARTITIONS" \
    --clean \
    --addr       "$KAFKA_ADDR" \
    --out        "$RESULTS_DIR/kafka.json"

  log "Running RabbitMQ benchmark..."
  "$BINARY" \
    --db         rabbitmq \
    --op         all \
    --count      "$MSG_COUNT" \
    --batch      "$BATCH_SIZE" \
    --consumers  "$CONSUMERS" \
    --clean \
    --addr       "$RABBIT_ADDR" \
    --out        "$RESULTS_DIR/rabbitmq.json"
}

merge_results() {
  log "Merging results..."
  jq -s '{
    schema_version: "results-summary/v1",
    experiment: {
      id: "kafka-vs-rabbitmq",
      name: "Kafka vs RabbitMQ",
      category: "messaging",
      path: "experiments/messaging/kafka-vs-rabbitmq"
    },
    run_id: .[0].run_id,
    timestamp: .[0].timestamp,
    mode: "full",
    config: {
      msg_count: '"$MSG_COUNT"',
      batch_size: '"$BATCH_SIZE"',
      consumers: '"$CONSUMERS"',
      partitions: '"$PARTITIONS"'
    },
    sources: [
      {name: "kafka", file: "results/kafka.json"},
      {name: "rabbitmq", file: "results/rabbitmq.json"}
    ],
    results: [.[].results[]]
  }' "$RESULTS_DIR/kafka.json" "$RESULTS_DIR/rabbitmq.json" \
    > "$RESULTS_DIR/summary.json"
}

verify_results() {
  for path in \
    "$RESULTS_DIR/kafka.json" \
    "$RESULTS_DIR/rabbitmq.json" \
    "$RESULTS_DIR/summary.json"; do
    if [ ! -s "$path" ]; then
      echo "error: expected results file is missing or empty: $path" >&2
      exit 1
    fi
  done
}

print_table() {
  log "--- Side-by-side comparison ---"
  echo ""
  printf "%-12s %-10s %12s %10s %10s %12s\n" \
    "DB" "Operation" "msg/s" "p50(ms)" "p95(ms)" "p99(ms)"
  printf "%-12s %-10s %12s %10s %10s %12s\n" \
    "------------" "----------" "------------" "--------" "--------" "------------"
  jq -r '.results[] | [
    .db, .op,
    (.throughput_msg_sec|floor|tostring),
    (.p50_ms|tostring),
    (.p95_ms|tostring),
    (.p99_ms|tostring)
  ] | @tsv' "$RESULTS_DIR/summary.json" | \
  while IFS=$'\t' read -r db op qps p50 p95 p99; do
    printf "%-12s %-10s %12s %10s %10s %12s\n" "$db" "$op" "$qps" "$p50" "$p95" "$p99"
  done
  echo ""
}

# --- Main ---
check_deps
build_binary
start_stack
wait_for_kafka
wait_for_rabbitmq

if [ "$SMOKE_ONLY" = true ]; then
  smoke_test
  log "Smoke-only mode: done."
  exit 0
fi

smoke_test
run_full
merge_results
verify_results
print_table

log "Done. Results: $RESULTS_DIR/summary.json"
log "RabbitMQ Management: http://localhost:15672"
log "Prometheus: http://localhost:9094"
log "Grafana:    http://localhost:3002"
