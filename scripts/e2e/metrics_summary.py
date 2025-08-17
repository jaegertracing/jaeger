# Copyright (c) 2025 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

import argparse
from collections import defaultdict
from prometheus_client.parser import text_string_to_metric_families

def parse_metrics(content):
    metrics = []
    for family in text_string_to_metric_families(content):
        for sample in family.samples:
            labels = dict(sample.labels)
            # Simply pop undesirable metric labels to match the diff generation
            labels.pop('service_instance_id', None)
            label_pairs = sorted(labels.items(), key=lambda x: x[0])
            label_str = ','.join(f'{k}="{v}"' for k, v in label_pairs)
            metric = f"{family.name}{{{label_str}}}"
            metrics.append(metric)
    return metrics

def parse_diff_file(diff_path):
    """
    Parses a unified diff file and categorizes changes into added, removed, and modified metrics.
    Also captures the raw diff sections for each metric.
    """
    changes = {
        'added': defaultdict(list),
        'removed': defaultdict(list),
        'modified': defaultdict(list)
    }

    # Store raw diff sections for each metric - just collect all lines related to each metric
    raw_diff_sections = defaultdict(list)

    with open(diff_path, 'r') as f:
        lines = f.readlines()

    current_metric = None

    for line in lines:
        original_line = line.rstrip()
        stripped = line.strip()

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

    return changes, raw_diff_sections

def extract_metric_name(line):
    """Extracts metric name from a metric line, matching the diff generation format"""
    if '{' in line:
        return line.split('{')[0].strip()
    return line.strip()

def get_raw_diff_sample(raw_sections, max_lines=7):
    """
    Get sample raw diff lines, preserving original diff formatting.
    """
    if not raw_sections:
        return []

    # Take first section and limit lines
    sample_lines = []
    for section in raw_sections[:1]:  # Just take first section to keep it concise
        lines = section.split('\n')
        for i, line in enumerate(lines):
            if i >= max_lines:
                sample_lines.append("...")
                break
            sample_lines.append(line)

    return sample_lines

def generate_diff_summary(changes):
    """
    Generates a markdown summary from the parsed diff changes with sample lines.
    """
    summary = ["## 📊 Metrics Diff Summary\n"]

    # Statistics header
    total_added = sum(len(v) for v in changes['added'].values())
    total_removed = sum(len(v) for v in changes['removed'].values())
    total_modified = len(changes['modified'])

    summary.append(f"**Total Changes:** {total_added + total_removed + total_modified}\n")
    summary.append(f"- 🆕 Added: {total_added} metrics")
    summary.append(f"- ❌ Removed: {total_removed} metrics")
    summary.append(f"- 🔄 Modified: {total_modified} metrics\n")

    # Added metrics
    if changes['added']:
        summary.append("\n### 🆕 Added Metrics")
        for metric, samples in changes['added'].items():
            summary.append(f"- `{metric}` ({len(samples)} variants)")
            sample_lines = get_sample_lines(samples)
            if sample_lines:
                summary.append("<details>")
                summary.append("<summary>View samples</summary>")
                summary.append("")
                summary.append("```")
                summary.extend(sample_lines)
                summary.append("```")
                summary.append("</details>")

    # Removed metrics
    if changes['removed']:
        summary.append("\n### ❌ Removed Metrics")
        for metric, samples in changes['removed'].items():
            summary.append(f"- `{metric}` ({len(samples)} variants)")
            sample_lines = get_sample_lines(samples)
            if sample_lines:
                summary.append("<details>")
                summary.append("<summary>View samples</summary>")
                summary.append("")
                summary.append("```")
                summary.extend(sample_lines)
                summary.append("```")
                summary.append("</details>")

    # Modified metrics
    if changes['modified']:
        summary.append("\n### 🔄 Modified Metrics")
        for metric, versions in changes['modified'].items():
            summary.append(f"- `{metric}`")
            summary.append(f"  - Added variants: {len(versions['added'])}")
            summary.append(f"  - Removed variants: {len(versions['removed'])}")

            # Show samples of added variants
            if versions['added']:
                added_samples = get_sample_lines(versions['added'], max_lines=3)
                if added_samples:
                    summary.append("  - Sample added:")
                    summary.append("    <details>")
                    summary.append("    <summary>View added samples</summary>")
                    summary.append("")
                    summary.append("    ```")
                    summary.extend([f"  {line}" for line in added_samples])
                    summary.append("    ```")
                    summary.append("    </details>")

            # Show samples of removed variants
            if versions['removed']:
                removed_samples = get_sample_lines(versions['removed'], max_lines=3)
                if removed_samples:
                    summary.append("  - Sample removed:")
                    summary.append("    <details>")
                    summary.append("    <summary>View removed samples</summary>")
                    summary.append("")
                    summary.append("    ```")
                    summary.extend([f"  {line}" for line in removed_samples])
                    summary.append("    ```")
                    summary.append("    </details>")

    return "\n".join(summary)

def main():
    parser = argparse.ArgumentParser(description='Generate metrics diff summary')
    parser.add_argument('--diff', required=True, help='Path to unified diff file')
    parser.add_argument('--output', required=True, help='Output summary file path')

    args = parser.parse_args()

    changes, raw_diff_sections = parse_diff_file(args.diff)
    summary = generate_diff_summary(changes, raw_diff_sections)

    with open(args.output, 'w') as f:
        f.write(summary)

    print(f"Generated diff summary with {len(changes['added'])} additions, "
          f"{len(changes['removed'])} removals and "
          f"{len(changes['modified'])} modifications")
    print(f"Summary saved to {args.output}")

if __name__ == '__main__':
    main()