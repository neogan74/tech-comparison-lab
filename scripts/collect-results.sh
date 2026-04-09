#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/.." && pwd)
OUTPUT_PATH="${1:-$REPO_ROOT/docs/index.json}"
TMP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/collect-results.XXXXXX")

cleanup() {
  rm -rf "$TMP_DIR"
}

trap cleanup EXIT

log() {
  echo "[collect-results] $*"
}

check_deps() {
  command -v jq >/dev/null 2>&1 || {
    echo "error: 'jq' not found." >&2
    exit 1
  }
}

collect_summaries() {
  find "$REPO_ROOT/experiments" -type f -path '*/results/summary.json' | sort
}

decorate_summary() {
  local src_path="$1"
  local rel_path="${src_path#$REPO_ROOT/}"
  local dest_path="$2"

  jq --arg summary_file "$rel_path" '
    if .schema_version != "results-summary/v1" then
      error("unsupported summary schema in " + $summary_file)
    else
      {
        id: .experiment.id,
        name: .experiment.name,
        category: .experiment.category,
        summary_file: $summary_file,
        timestamp: .timestamp,
        mode: .mode,
        result_count: (.results | length),
        summary: .
      }
    end
  ' "$src_path" > "$dest_path"
}

write_index() {
  local summaries_dir="$1"
  local output_path="$2"
  mkdir -p "$(dirname "$output_path")"

  if ! find "$summaries_dir" -type f | grep -q .; then
    jq -n '{
      schema_version: "results-index/v1",
      generated_at: (now | todateiso8601),
      count: 0,
      experiments: []
    }' > "$output_path"
    return
  fi

  jq -s '{
    schema_version: "results-index/v1",
    generated_at: (now | todateiso8601),
    count: length,
    experiments: sort_by(.category, .id)
  }' "$summaries_dir"/*.json > "$output_path"
}

main() {
  check_deps

  local count=0
  while IFS= read -r summary; do
    decorate_summary "$summary" "$TMP_DIR/$(printf '%03d' "$count").json"
    count=$((count + 1))
  done < <(collect_summaries)

  write_index "$TMP_DIR" "$OUTPUT_PATH"
  log "Wrote $OUTPUT_PATH ($count summaries)"
}

main "$@"
