#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/.." && pwd)
INDEX_PATH="${1:-$REPO_ROOT/docs/index.json}"
DOCS_DIR="${2:-$REPO_ROOT/docs}"
EXPERIMENTS_DIR="$DOCS_DIR/experiments"

log() {
  echo "[render-results] $*"
}

check_deps() {
  command -v jq >/dev/null 2>&1 || {
    echo "error: 'jq' not found." >&2
    exit 1
  }
  [ -f "$INDEX_PATH" ] || {
    echo "error: index not found: $INDEX_PATH" >&2
    exit 1
  }
}

render_experiment_page() {
  local experiment_json="$1"
  local output_path="$2"

  jq -r '
    def metric_value:
      .throughput_ops_sec // .throughput_rows_sec // .throughput_msg_sec // .throughput_rps;

    def metric_label:
      if has("throughput_ops_sec") then "ops/s"
      elif has("throughput_rows_sec") then "rows/s"
      elif has("throughput_msg_sec") then "msg/s"
      elif has("throughput_rps") then "rps"
      else "-" end;

    def subject:
      .db // .protocol // .cluster // .cluster_type // "n/a";

    def fmt_num:
      if . == null then "-"
      elif (type == "number") then
        if . == floor then tostring else ((. * 1000 | round) / 1000 | tostring) end
      else tostring end;

    def fmt_rate:
      if metric_value == null then "-"
      else (((metric_value * 100 | round) / 100) | tostring) + " " + metric_label
      end;

    def fmt_error:
      if .errors == null then "-"
      else (.errors | tostring)
      end;

    def fmt_storage:
      if .storage_bytes != null then (.storage_bytes | tostring)
      elif .memory_used != null then .memory_used
      else "-"
      end;

    def highlights:
      .summary.results
      | group_by(.op)
      | map(
          . as $rows
          | ($rows | map(select(metric_value != null)) | sort_by(metric_value)) as $throughput_rows
          | ($rows | map(select(.p95_ms != null)) | sort_by(.p95_ms)) as $latency_rows
          | {
              op: $rows[0].op,
              throughput_best: ($throughput_rows[-1] // null),
              throughput_runner_up: ($throughput_rows[-2] // null),
              latency_best: ($latency_rows[0] // null),
              latency_runner_up: ($latency_rows[1] // null)
            }
        );

    "# " + .name,
    "",
    "Generated from `" + .summary_file + "`.",
    "",
    "## Metadata",
    "",
    "| Field | Value |",
    "|-------|-------|",
    "| Experiment | " + .name + " |",
    "| Category | `" + .category + "` |",
    "| Run Timestamp | `" + .timestamp + "` |",
    "| Mode | `" + .mode + "` |",
    "| Run ID | `" + .summary.run_id + "` |",
    "| Result Count | " + (.result_count | tostring) + " |",
    "",
    "## Config",
    "",
    "| Key | Value |",
    "|-----|-------|",
    (.summary.config | to_entries[] | "| `" + .key + "` | `" + (.value | tostring) + "` |"),
    "",
    "## Sources",
    "",
    "| Name | File |",
    "|------|------|",
    (.summary.sources[] | "| `" + .name + "` | `" + .file + "` |"),
    "",
    "## Highlights",
    "",
    (
      highlights[]
      | "- `" + .op + "`: "
        + (
            if .throughput_best != null and .throughput_runner_up != null and (.throughput_runner_up | metric_value) > 0 then
              "throughput leader `" + (.throughput_best | subject) + "` (" + (.throughput_best | fmt_rate)
              + ", " + ((((.throughput_best | metric_value) / (.throughput_runner_up | metric_value)) * 100 | round) / 100 | tostring)
              + "x vs `" + (.throughput_runner_up | subject) + "`)"
            elif .throughput_best != null then
              "throughput sample `" + (.throughput_best | subject) + "` (" + (.throughput_best | fmt_rate) + ")"
            else
              "no throughput metric"
            end
          )
        + "; "
        + (
            if .latency_best != null and .latency_runner_up != null and (.latency_best.p95_ms > 0) then
              "lowest p95 `" + (.latency_best | subject) + "` (" + (.latency_best.p95_ms | fmt_num) + " ms"
              + ", " + ((((.latency_runner_up.p95_ms) / (.latency_best.p95_ms)) * 100 | round) / 100 | tostring)
              + "x better than `" + (.latency_runner_up | subject) + "`)"
            elif .latency_best != null then
              "lowest p95 `" + (.latency_best | subject) + "` (" + (.latency_best.p95_ms | fmt_num) + " ms)"
            else
              "no p95 metric"
            end
          )
    ),
    "",
    "## Results",
    "",
    "| Subject | Operation | Count | p50 ms | p95 ms | p99 ms | Total ms | Throughput | Errors | Storage / Memory |",
    "|---------|-----------|-------|--------|--------|--------|----------|------------|--------|------------------|",
    (
      .summary.results[]
      | "| `" + (subject) + "`"
        + " | `" + .op + "`"
        + " | " + (.count | tostring)
        + " | " + (.p50_ms | fmt_num)
        + " | " + (.p95_ms | fmt_num)
        + " | " + (.p99_ms | fmt_num)
        + " | " + (.total_ms | fmt_num)
        + " | " + (fmt_rate)
        + " | " + (fmt_error)
        + " | " + (fmt_storage)
        + " |"
    ),
    ""
  ' "$experiment_json" > "$output_path"
}

render_index_page() {
  local output_path="$1"

  jq -r '
    def metric_value($r):
      $r.throughput_ops_sec // $r.throughput_rows_sec // $r.throughput_msg_sec // $r.throughput_rps;

    "# Benchmark Results",
    "",
    "Generated from `docs/index.json` at `" + .generated_at + "`.",
    "",
    "## Runs",
    "",
    "| Experiment | Category | Timestamp | Mode | Results | Report |",
    "|------------|----------|-----------|------|---------|--------|",
    (
      .experiments[]
      | "| " + .name
        + " | `" + .category + "`"
        + " | `" + .timestamp + "`"
        + " | `" + .mode + "`"
        + " | " + (.result_count | tostring)
        + " | [report](experiments/" + .id + ".md) |"
    ),
    "",
    "## Snapshot",
    "",
    (
      .experiments[]
      | . as $exp
      | ($exp.summary.results | map(select(metric_value(.) != null)) | sort_by(metric_value(.)) | reverse | .[0]) as $best
      | "- **" + $exp.name + "**: "
        + (if $best == null
           then "no throughput metric found"
           else "top throughput `" + ($best.db // $best.protocol // "n/a") + "/" + $best.op + "` = "
             + ((((metric_value($best) * 100) | round) / 100) | tostring)
           end)
    ),
    "",
    "## Schemas",
    "",
    "- [`results-summary/v1`](results-summary-v1.md)",
    "- [`results-index/v1`](results-index-v1.md)",
    ""
  ' "$INDEX_PATH" > "$output_path"
}

main() {
  check_deps
  mkdir -p "$EXPERIMENTS_DIR"

  jq -c '.experiments[]' "$INDEX_PATH" | while IFS= read -r experiment; do
    local_tmp=$(mktemp "${TMPDIR:-/tmp}/render-results-exp.XXXXXX")
    printf '%s\n' "$experiment" > "$local_tmp"
    render_experiment_page "$local_tmp" "$EXPERIMENTS_DIR/$(jq -r '.id' "$local_tmp").md"
    rm -f "$local_tmp"
  done

  render_index_page "$DOCS_DIR/index.md"
  log "Wrote $DOCS_DIR/index.md and experiment reports in $EXPERIMENTS_DIR"
}

main "$@"
