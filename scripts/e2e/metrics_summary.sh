#!/bin/bash

# Copyright (c) 2025 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

# Enable debug tracing and exit on error
set -exo pipefail

METRICS_DIR="${METRICS_DIR:-./.artifacts}"
declare -a summary_files=()
declare -a json_files=()
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
    echo "INFRA_ERRORS=true" >> "$GITHUB_OUTPUT"
else
    echo "INFRA_ERRORS=false" >> "$GITHUB_OUTPUT"
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

    # Derive the unique snapshot name from the artifact directory (e.g.,
    # diff_metrics_snapshot_cassandras_4.x_v004_v2_manual -> metrics_snapshot_cassandras_4.x_v004_v2_manual).
    # Using the directory rather than the file name is necessary because all matrix
    # variants of the same backend share an identical file name inside their artifact
    # (e.g., diff_metrics_snapshot_cassandra.txt), while the artifact directory name
    # is always unique (it includes major version, schema, jaeger-version, etc.).
    dir=$(dirname "$diff_file")
    snapshot_name=$(basename "$dir")
    snapshot_name=${snapshot_name#diff_}

    # Generate summary for this diff
    summary_file="$dir/summary_$snapshot_name.md"
    json_file="$dir/changes_$snapshot_name.json"

    echo "Generating summary for $snapshot_name"
    python3 ./scripts/e2e/metrics_summary.py \
        --diff "$diff_file" \
        --output "$summary_file" \
        --json-output "$json_file"

    summary_files+=("$summary_file")
    json_files+=("$json_file")
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

if [ ${#missing_diffs[@]} -gt 0 ]; then
    echo "CONCLUSION=failure" >> "$GITHUB_OUTPUT"
elif [ "$total_changes" -gt 0 ]; then
    echo "CONCLUSION=failure" >> "$GITHUB_OUTPUT"
else
    echo "CONCLUSION=success" >> "$GITHUB_OUTPUT"
fi

# Merge per-snapshot JSON files into a single metrics_snapshots.json.
# Each entry gets a "snapshot" field with the snapshot name.
# Capped at 50 entries to match the publish workflow's MAX_SNAPSHOTS limit.
# The trusted publish workflow validates this data before rendering.
python3 - "$METRICS_DIR" "${json_files[@]}" <<'PYEOF'
import json, os, sys
metrics_dir = sys.argv[1]
MAX_SNAPSHOTS = 50
snapshots = []
for path in sys.argv[2:]:
    if len(snapshots) >= MAX_SNAPSHOTS:
        print(f"Warning: capped at {MAX_SNAPSHOTS} snapshots", file=sys.stderr)
        break
    try:
        with open(path) as f:
            data = json.load(f)
        basename = os.path.basename(path)
        name = basename.removeprefix('changes_').removesuffix('.json')
        data['snapshot'] = name
        snapshots.append(data)
    except Exception as e:
        print(f"Warning: could not read {path}: {e}", file=sys.stderr)
output_path = os.path.join(metrics_dir, 'metrics_snapshots.json')
snapshots.sort(key=lambda s: s.get('snapshot', ''))
with open(output_path, 'w') as f:
    json.dump(snapshots, f, indent=2)
print(f"Merged {len(snapshots)} snapshot(s) into {output_path}")
PYEOF

# Log the combined summary to the console (visible in CI run logs).
# Structured conclusions are already emitted to $GITHUB_OUTPUT above.
echo "=== Metrics Comparison Summary ==="

if [ ${#missing_diffs[@]} -gt 0 ]; then
    echo "::error::Infrastructure error: diff artifacts missing for: ${missing_diffs[*]}"
    echo "(These snapshots did not produce a diff artifact — the verify-metrics-snapshot action may not have run.)"
fi

if [ "$total_changes" -gt 0 ]; then
    echo "::error::${total_changes} metric change(s) detected across all snapshots"
    echo ""
    for summary_file in "${summary_files[@]}"; do
        file_name=$(basename "$summary_file" .md)
        echo "--- ${file_name} ---"
        echo ""
        cat "$summary_file"
        echo ""
    done
elif [ ${#missing_diffs[@]} -gt 0 ]; then
    echo "No metric changes in available diffs, but some diff artifacts were missing (see above)."
else
    echo "No metric changes detected."
fi

echo "Metrics diff processing completed"
