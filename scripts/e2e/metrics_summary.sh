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

# Verify 1-to-1: every metrics_snapshot_* artifact must have a diff_metrics_snapshot_* artifact.
# verify-metrics-snapshot always uploads a diff artifact on PRs (empty stub if no baseline),
# so a missing diff dir means that action never ran for that snapshot — an infra failure.
echo "=== Checking for missing diff artifacts ==="
declare -a missing_diffs=()
found_any_snapshot=false
for snapshot_dir in "$METRICS_DIR"/metrics_snapshot_*/; do
    [ -d "$snapshot_dir" ] || continue
    found_any_snapshot=true
    name=$(basename "$snapshot_dir")
    if [ ! -d "$METRICS_DIR/diff_$name" ]; then
        echo "::error::Missing diff artifact for snapshot: $name"
        missing_diffs+=("$name")
    else
        echo "OK: diff_$name present"
    fi
done
if [ "$found_any_snapshot" = false ]; then
    echo "::error::No metrics_snapshot_* artifacts found; E2E jobs may not have run"
    missing_diffs+=("(no snapshot artifacts found)")
fi
if [ ${#missing_diffs[@]} -gt 0 ]; then
    echo "INFRA_ERRORS=${missing_diffs[*]}" >> "$GITHUB_OUTPUT"
else
    echo "INFRA_ERRORS=" >> "$GITHUB_OUTPUT"
fi

# Debug: List all diff files found
echo "=== Searching for diff files ==="
find "$METRICS_DIR" -type f -name "diff_*.txt" | while read -r file; do
    echo "Found diff file: $file"
done

# Process all non-empty diff files.
# Empty diff files are stubs uploaded by verify-metrics-snapshot when there is no
# baseline or when compare_metrics.py found no differences (it only writes to the
# output file when differences exist). The 1-to-1 directory check above already
# verified the action ran; here we only want to summarise actual changes.
while IFS= read -r -d '' diff_file; do
    if [ ! -s "$diff_file" ]; then
        echo "Skipping empty diff file (no changes or no baseline): $diff_file"
        continue
    fi
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

if [ ${#summary_files[@]} -eq 0 ]; then
    echo "No diff files found; all metrics are within baseline."
else
    for summary_file in "${summary_files[@]}"; do
        changes=$(grep -F "**Total Changes:**" "$summary_file" | awk '{print $3}')
        total_changes=$((total_changes + changes))
    done
fi

echo "Total changes across all snapshots: $total_changes"
echo "TOTAL_CHANGES=$total_changes" >> "$GITHUB_OUTPUT"

# Emit a single conclusion/summary so the workflow check run step
# doesn't need to duplicate this decision logic.
if [ ${#missing_diffs[@]} -gt 0 ]; then
    echo "CONCLUSION=failure" >> "$GITHUB_OUTPUT"
    echo "SUMMARY=❌ Infrastructure error: diff artifacts missing for: ${missing_diffs[*]}" >> "$GITHUB_OUTPUT"
elif [ "$total_changes" -gt 0 ]; then
    echo "CONCLUSION=failure" >> "$GITHUB_OUTPUT"
    echo "SUMMARY=❌ ${total_changes} metric changes detected" >> "$GITHUB_OUTPUT"
else
    echo "CONCLUSION=success" >> "$GITHUB_OUTPUT"
    echo "SUMMARY=✅ No significant metric changes detected" >> "$GITHUB_OUTPUT"
fi

# Always generate combined summary report
combined_file="$METRICS_DIR/combined_summary.md"
echo "## Metrics Comparison Summary" > "$combined_file"

if [ ${#missing_diffs[@]} -gt 0 ]; then
    {
      echo ""
      echo "❌ **Infrastructure error**: diff artifacts missing for: ${missing_diffs[*]}"
      echo "(These snapshots did not produce a diff artifact — the verify-metrics-snapshot action may not have run.)"
      echo ""
    } >> "$combined_file"
fi

if [ ${#summary_files[@]} -eq 0 ]; then
    {
      echo ""
      echo "✅ No metric changes detected."
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
        file_name=$(basename "$summary_file" .md)
        friendly_name=${file_name#summary_metrics_snapshot_}
        # Title-case: replace underscores with spaces, capitalize first letter of each word.
        # Uses awk because sed's \b is backspace (not word-boundary).
        friendly_name=$(awk '{for(i=1;i<=NF;i++) $i=toupper(substr($i,1,1)) substr($i,2)}1' <<< "${friendly_name//_/ }")
        {
          echo "### 📊 ${friendly_name}"
          echo "File Name: ${file_name}"
          cat "$summary_file"
        } >> "$combined_file"
        echo "" >> "$combined_file"
    done

    echo "</details>" >> "$combined_file"
fi

echo -e "\n\n➡️ [View CI artifacts]($LINK_TO_ARTIFACT) | [View Summary Report logs]($SUMMARY_RUN_URL)" >> "$combined_file"


echo "Metrics diff processing completed"
