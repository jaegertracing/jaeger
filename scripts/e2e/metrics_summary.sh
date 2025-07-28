#!/bin/bash

# Copyright (c) 2025 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

# Enable debug tracing and exit on error
set -exo pipefail

METRICS_DIR="./.metrics"
DIFF_FOUND=false
declare -a summary_files=()

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
    DIFF_FOUND=true

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
if $DIFF_FOUND; then
    echo "Metric differences detected"
    echo "DIFF_FOUND=true" >> "$GITHUB_OUTPUT"

    # Combine all summaries into one
    echo "## Metrics Comparison Summary" > "$METRICS_DIR/combined_summary.md"
    echo "" >> "$METRICS_DIR/combined_summary.md"

    if [ ${#summary_files[@]} -gt 0 ]; then
        for summary_file in "${summary_files[@]}"; do
            echo "Appending $summary_file to combined summary"
            {
              echo "### $(basename "$summary_file" .md)"
              cat "$summary_file"
            } >> "$METRICS_DIR/combined_summary.md"
            echo "" >> "$METRICS_DIR/combined_summary.md"
        done
    fi

    echo -e "\n\n➡️ [View full metrics file]($LINK_TO_ARTIFACT)" >> "$METRICS_DIR/combined_summary.md"
else
    echo "No metric differences detected"
fi

echo "Metrics diff processing completed"