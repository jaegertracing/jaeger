#!/bin/bash

# Copyright (c) 2025 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

# Enable debug tracing and exit on error
set -exo pipefail

METRICS_DIR="./.metrics"
DIFF_FOUND=false
declare -a summary_files=()

echo "Starting metrics comparison in directory: $METRICS_DIR"
echo "Directory structure:"
ls -la "$METRICS_DIR" || echo "Metrics directory listing failed"

# Debug: List all metric files found
echo "=== Searching for metric files ==="
find "$METRICS_DIR" -type f -name "*.txt" | while read -r file; do
    echo "Found file: $file"
done

# Process all metric files (excluding baseline/diff files)
while IFS= read -r -d '' file; do
    echo "Processing file: $file"

    # Skip baseline/diff files
    if [[ $file == *"baseline_"* ]] || [[ $file == *"diff_"* ]]; then
        echo "Skipping baseline/diff file: $file"
        continue
    fi

    # Construct baseline file path
    dir=$(dirname "$file")
    filename=$(basename "$file")
    base_file="$dir/baseline_$filename"

    if [ -f "$base_file" ]; then
        snapshot_name=$(basename "$file" .txt)
        echo "Comparing against baseline: $base_file"

        # First run comparison to check for differences
        python3 ./scripts/e2e/compare_metrics.py \
            --file1 "$file" \
            --file2 "$base_file" \
            --output "$dir/diff_$snapshot_name.txt"

        if [ $? -eq 1 ]; then
            DIFF_FOUND=true
            echo "Differences found for $snapshot_name"

            # Only generate summary if there are differences
            python3 ./scripts/e2e/metrics_summary.py \
                --base "$base_file" \
                --pr "$file" \
                --output "$dir/summary_$snapshot_name.md"
            summary_files+=("$dir/summary_$snapshot_name.md")
        else
            echo "No differences found for $snapshot_name"
        fi
    else
        echo "No baseline file found for $file (expected at: $base_file)"
    fi
done < <(find "$METRICS_DIR" -type f -name "*.txt" ! -name "baseline_*" ! -name "diff_*" -print0)

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
            echo "### $(basename "$summary_file" .md)" >> "$METRICS_DIR/combined_summary.md"
            cat "$summary_file" >> "$METRICS_DIR/combined_summary.md"
            echo "" >> "$METRICS_DIR/combined_summary.md"
        done
    fi

    echo -e "\n\n[View detailed metrics differences]($LINK_TO_ARTIFACT)" >> "$METRICS_DIR/combined_summary.md"
else
    echo "No metric differences detected"
fi

echo "Metrics comparison completed"