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
    """
    changes = {
        'added': defaultdict(list),
        'removed': defaultdict(list),
        'modified': defaultdict(list)
    }

    current_block = []
    current_change = None

    with open(diff_path, 'r') as f:
        for line in f:
            line = line.strip()
            # Skip diff headers
            if line.startswith('+++') or line.startswith('---'):
                continue

            # Start new block for changes
            if line.startswith('+') or line.startswith('-'):
                if current_block and current_change:
                    metric_name = extract_metric_name(current_block[0])
                    if metric_name:
                        changes[current_change][metric_name].append('\n'.join(current_block))
                current_block = [line[1:].strip()]  # Remove +/-
                current_change = 'added' if line.startswith('+') else 'removed'
            elif line.startswith(' '):
                if current_block:
                    current_block.append(line.strip())
            else:
                if current_block and current_change:
                    metric_name = extract_metric_name(current_block[0])
                    if metric_name:
                        changes[current_change][metric_name].append('\n'.join(current_block))
                current_block = []
                current_change = None

    # Process any remaining block
    if current_block and current_change:
        metric_name = extract_metric_name(current_block[0])
        if metric_name:
            changes[current_change][metric_name].append('\n'.join(current_block))

    # Identify modified metrics (same metric name with both additions and removals)
    common_metrics = set(changes['added'].keys()) & set(changes['removed'].keys())
    for metric in common_metrics:
        changes['modified'][metric] = {
            'added': changes['added'].pop(metric),
            'removed': changes['removed'].pop(metric)
        }

    return changes

def extract_metric_name(line):
    """Extracts metric name from a metric line, matching the diff generation format"""
    if '{' in line:
        return line.split('{')[0].strip()
    return line.strip()

def generate_diff_summary(changes):
    """
    Generates a markdown summary from the parsed diff changes.
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

    # Removed metrics
    if changes['removed']:
        summary.append("\n### ❌ Removed Metrics")
        for metric, samples in changes['removed'].items():
            summary.append(f"- `{metric}` ({len(samples)} variants)")

    # Modified metrics
    if changes['modified']:
        summary.append("\n### 🔄 Modified Metrics")
        for metric, versions in changes['modified'].items():
            summary.append(f"- `{metric}`")
            summary.append(f"  - Added variants: {len(versions['added'])}")
            summary.append(f"  - Removed variants: {len(versions['removed'])}")

    return "\n".join(summary)

def main():
    parser = argparse.ArgumentParser(description='Generate metrics diff summary')
    parser.add_argument('--diff', required=True, help='Path to unified diff file')
    parser.add_argument('--output', required=True, help='Output summary file path')

    args = parser.parse_args()

    changes = parse_diff_file(args.diff)
    summary = generate_diff_summary(changes)

    with open(args.output, 'w') as f:
        f.write(summary)

    print(f"Generated diff summary with {len(changes['added'])} additions, "
          f"{len(changes['removed'])} removals and "
          f"{len(changes['modified'])} modifications")
    print(f"Summary saved to {args.output}")

if __name__ == '__main__':
    main()