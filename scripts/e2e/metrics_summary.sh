#!/bin/bash
# Enable debug tracing and exit on error
set -exo pipefail

METRICS_DIR="./.metrics"
DIFF_FOUND=false
summary_files=""

echo "Starting metrics comparison in directory: $METRICS_DIR"
ls -la "$METRICS_DIR" || echo "Metrics directory listing failed"

# Loop through all metric files
for file in "$METRICS_DIR"/*.txt; do
    echo "Processing file: $file"

    if [[ $file == *"baseline_"* ]] || [[ $file == *"diff_"* ]]; then
        echo "Skipping baseline/diff file: $file"
        continue
    fi

    base_file="$METRICS_DIR/baseline_$(basename "$file")"
    if [ -f "$base_file" ]; then
        snapshot_name=$(basename "$file" .txt)
        echo "Comparing against baseline: $base_file"

        # First run comparison to check for differences
        python3 ./scripts/e2e/compare_metrics.py \
            --file1 "$file" \
            --file2 "$base_file" \
            --output "$METRICS_DIR/diff_$snapshot_name.txt"

        if [ $? -eq 1 ]; then
            DIFF_FOUND=true
            echo "Differences found for $snapshot_name"

            # Only generate summary if there are differences
            python3 ./scripts/e2e/metrics_summary.py \
                --base "$base_file" \
                --pr "$file" \
                --output "$METRICS_DIR/summary_$snapshot_name.md"
            summary_files+="$METRICS_DIR/summary_$snapshot_name.md "
        else
            echo "No differences found for $snapshot_name"
        fi
    else
        echo "No baseline file found for $file"
    fi
done

if $DIFF_FOUND; then
    echo "Metric differences detected"
    echo "DIFF_FOUND=true" >> $GITHUB_OUTPUT

    # Combine all summaries into one
    echo "## Metrics Comparison Summary" > "$METRICS_DIR/combined_summary.md"
    cat $summary_files >> "$METRICS_DIR/combined_summary.md"
    echo -e "\n\n[View detailed metrics differences]($LINK_TO_ARTIFACT)" >> "$METRICS_DIR/combined_summary.md"
else
    echo "No metric differences detected"
fi

echo "Metrics comparison completed"