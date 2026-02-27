#!/bin/bash

# Copyright (c) 2025 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

# Enable debug tracing and exit on error
set -exo pipefail

METRICS_DIR="${METRICS_DIR:-./.artifacts}"
declare -a summary_files=()
total_changes=0

echo "Starting metrics diff processing in directory: $METRICS_DIR"
echo "Directory structure:"
ls -la "$METRICS_DIR" || echo "Metrics directory listing failed"

# Debug: List all diff files found
echo "=== Searching for diff files ==="
find "$METRICS_DIR" -type f -name "diff_*.txt" | while read -r file; do
    echo "Found diff file: $file"
done

# Process all diff files
while IFS= read -r -d '' diff_file; do
    echo "Processing diff file: $diff_file"

    # Extract the base name (e.g., diff_metrics_snapshot_cassandra.txt -> metrics_snapshot_cassandra)
    base_name=$(basename "$diff_file" .txt)
    snapshot_name=${base_name#diff_}
    dir=$(dirname "$diff_file")

    # Generate summary for this diff
    summary_file="$dir/summary_$snapshot_name.md"

    echo "Generating summary for $snapshot_name"
    python3 ./scripts/e2e/metrics_summary.py \
        --diff "$diff_file" \
        --output "$summary_file"

    summary_files+=("$summary_file")
    echo "Generated summary at: $summary_file"
done < <(find "$METRICS_DIR" -type f -name "diff_*.txt" -print0)

# Output results
# Calculate total changes across all files
total_changes=0
has_error=false

if [ ${#summary_files[@]} -eq 0 ]; then
    echo "ERROR: No summary files were generated. Expected at least 8 diff files from CI." >&2
    has_error=true
else
    for summary_file in "${summary_files[@]}"; do
        changes=$(grep -F "**Total Changes:**" "$summary_file" | awk '{print $3}')
        total_changes=$((total_changes + changes))
    done
fi

echo "Total changes across all snapshots: $total_changes"
echo "TOTAL_CHANGES=$total_changes" >> "$GITHUB_OUTPUT"
echo "HAS_ERROR=$has_error" >> "$GITHUB_OUTPUT"

# Always generate combined summary report
combined_file="$METRICS_DIR/combined_summary.md"
echo "## Metrics Comparison Summary" > "$combined_file"

if [ "$has_error" = true ]; then
    {
      echo ""
      echo "❌ **ERROR: No summary files were generated. Expected at least 8 diff files from CI.**"
      echo ""
      echo "This indicates a failure in the E2E test execution or metrics collection process."
      echo ""
    } >> "$combined_file"
else
    {
      echo ""
      echo "Total changes across all snapshots: $total_changes"
      echo ""
      echo "<details>"
      echo "<summary>Detailed changes per snapshot</summary>"
      echo ""
    } >> "$combined_file"

    for summary_file in "${summary_files[@]}"; do
        echo "Appending $summary_file to combined summary"
        {
          echo "### $(basename "$summary_file" .md)"
          cat "$summary_file"
        } >> "$combined_file"
        echo "" >> "$combined_file"
    done

    echo "</details>" >> "$combined_file"
fi

echo -e "\n\n➡️ [View full metrics file]($LINK_TO_ARTIFACT)" >> "$combined_file"


echo "Metrics diff processing completed"
