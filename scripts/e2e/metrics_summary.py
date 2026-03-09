# Copyright (c) 2025 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

import argparse
import json
from collections import defaultdict

def parse_diff_file(diff_path):
    """
    Parses a unified diff file and categorizes changes into added, removed, and modified metrics.
    Also captures the raw diff sections for each metric and the exclusion count.
    """
    changes = {
        'added': defaultdict(list),
        'removed': defaultdict(list),
        'modified': defaultdict(list)
    }

    # Store raw diff sections for each metric - just collect all lines related to each metric
    raw_diff_sections = defaultdict(list)
    exclusion_count = 0

    with open(diff_path, 'r') as f:
        lines = f.readlines()

    current_metric = None
    for line in lines:
        original_line = line.rstrip('\n')
        stripped = original_line.strip()

        if stripped.startswith('Metrics excluded from A: ') or stripped.startswith('Metrics excluded from B: '):
            count_str = stripped.split(': ')[1]
            exclusion_count += int(count_str)
            continue
        # Skip diff headers
        if stripped.startswith('+++') or stripped.startswith('---'):
            continue
        # Check if this line contains a metric change
        if stripped.startswith('+') or stripped.startswith('-'):
            metric_name = extract_metric_name(stripped[1:].strip())
            if metric_name:
                # Track the change type
                change_type = 'added' if stripped.startswith('+') else 'removed'
                changes[change_type][metric_name].append(stripped[1:].strip())

                # Always add to raw diff sections regardless of change type
                raw_diff_sections[metric_name].append(original_line)
                current_metric = metric_name
            else:
                # If we're in a metric section, keep adding lines
                if current_metric:
                    raw_diff_sections[current_metric].append(original_line)
        elif stripped.startswith(' ') and current_metric:
            # Context line - add to current metric's raw section
            raw_diff_sections[current_metric].append(original_line)
        else:
            # End of current metric section
            current_metric = None

    # Identify modified metrics (same metric name with both additions and removals)
    common_metrics = set(changes['added'].keys()) & set(changes['removed'].keys())
    for metric in common_metrics:
        changes['modified'][metric] = {
            'added': changes['added'].pop(metric),
            'removed': changes['removed'].pop(metric)
        }

    return changes, raw_diff_sections, exclusion_count

def extract_metric_name(line):
    """Extracts metric name from a metric line, matching the diff generation format"""
    if '{' in line:
        return line.split('{')[0].strip()
    return line.strip()

def get_raw_diff_sample(raw_lines, max_lines=7):
    """
    Get sample raw diff lines, preserving original diff formatting.
    """
    if not raw_lines:
        return []

    # Take up to max_lines
    sample_lines = raw_lines[:max_lines]
    if len(raw_lines) > max_lines:
        sample_lines.append("...")

    return sample_lines

def generate_diff_summary(changes, raw_diff_sections, exclusion_count):
    """
    Generates a markdown summary from the parsed diff changes with raw diff samples.
    """
    summary = []

    # Statistics header
    total_added = sum(len(v) for v in changes['added'].values())
    total_removed = sum(len(v) for v in changes['removed'].values())
    total_modified = len(changes['modified'])

    summary.append(f"**Total Changes:** {total_added + total_removed + total_modified}\n")
    summary.append(f"- 🆕 Added: {total_added} metrics")
    summary.append(f"- ❌ Removed: {total_removed} metrics")
    summary.append(f"- 🔄 Modified: {total_modified} metrics")
    summary.append(f"- 🚫 Excluded: {exclusion_count} metrics\n")

    # Added metrics
    if changes['added']:
        summary.append("\n#### 🆕 Added Metrics")
        for metric, samples in changes['added'].items():
            summary.append(f"- `{metric}` ({len(samples)} variants)")
            raw_samples = get_raw_diff_sample(raw_diff_sections.get(metric, []))
            if raw_samples:
                summary.append("<details>")
                summary.append("<summary>View diff sample</summary>")
                summary.append("")
                summary.append("```diff")
                summary.extend(raw_samples)
                summary.append("```")
                summary.append("</details>")

    # Removed metrics
    if changes['removed']:
        summary.append("\n#### ❌ Removed Metrics")
        for metric, samples in changes['removed'].items():
            summary.append(f"- `{metric}` ({len(samples)} variants)")
            raw_samples = get_raw_diff_sample(raw_diff_sections.get(metric, []))
            if raw_samples:
                summary.append("<details>")
                summary.append("<summary>View diff sample</summary>")
                summary.append("")
                summary.append("```diff")
                summary.extend(raw_samples)
                summary.append("```")
                summary.append("</details>")

    # Modified metrics
    if changes['modified']:
        summary.append("\n#### 🔄 Modified Metrics")
        for metric, versions in changes['modified'].items():
            summary.append(f"- `{metric}`")
            summary.append(f"  - Added variants: {len(versions['added'])}")
            summary.append(f"  - Removed variants: {len(versions['removed'])}")

            raw_samples = get_raw_diff_sample(raw_diff_sections.get(metric, []))
            if raw_samples:
                summary.append("  <details>")
                summary.append("  <summary>View diff sample</summary>")
                summary.append("")
                summary.append("  ```diff")
                summary.extend([f"  {line}" for line in raw_samples])
                summary.append("  ```")
                summary.append("  </details>")

    return "\n".join(summary)

MAX_METRIC_NAMES = 200

def generate_structured_json(changes):
    """
    Generates a structured JSON-serializable dict of metric change data.
    Contains only metric names (strings) and counts (ints) — no raw diff
    lines or free-form text — so it is safe to pass through ci-summary.json
    to the trusted publish workflow.

    Counts use metric-name semantics (number of unique metric names per
    category) so that they match the displayed metric_names list.
    Note: the TOTAL_CHANGES headline uses variant-level counts from the
    markdown summary; the per-snapshot detail intentionally shows the
    simpler metric-name-level view.
    """
    added_names = sorted(changes['added'].keys())
    removed_names = sorted(changes['removed'].keys())
    modified_names = sorted(changes['modified'].keys())

    # Union of all changed metric names, deduplicated, sorted, and capped
    # to avoid unbounded artifact growth. The publish workflow enforces a
    # matching cap (MAX_METRIC_NAMES_PER_SNAPSHOT).
    all_names = sorted(set(added_names) | set(removed_names) | set(modified_names))
    capped = all_names[:MAX_METRIC_NAMES]

    # Compute counts from the capped list so they match the displayed names.
    added_set = set(added_names)
    removed_set = set(removed_names)
    modified_set = set(modified_names)

    return {
        'added': sum(1 for n in capped if n in added_set),
        'removed': sum(1 for n in capped if n in removed_set),
        'modified': sum(1 for n in capped if n in modified_set),
        'metric_names': capped,
    }


def main():
    parser = argparse.ArgumentParser(description='Generate metrics diff summary')
    parser.add_argument('--diff', required=True, help='Path to unified diff file')
    parser.add_argument('--output', required=True, help='Output summary file path')
    parser.add_argument('--json-output', default=None,
                       help='Optional path to write structured JSON change data')

    args = parser.parse_args()

    changes, raw_diff_sections, exclusion_count = parse_diff_file(args.diff)
    summary = generate_diff_summary(changes, raw_diff_sections, exclusion_count)

    with open(args.output, 'w') as f:
        f.write(summary)

    if args.json_output:
        structured = generate_structured_json(changes)
        with open(args.json_output, 'w') as f:
            json.dump(structured, f, indent=2)
        print(f"Structured JSON saved to {args.json_output}")

    print(f"Generated diff summary with {len(changes['added'])} additions, "
          f"{len(changes['removed'])} removals, "
          f"{len(changes['modified'])} modifications and "
          f"{exclusion_count} exclusions")
    print(f"Summary saved to {args.output}")

if __name__ == '__main__':
    main()