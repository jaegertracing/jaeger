# Copyright (c) 2025 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

import argparse
import json
from collections import defaultdict
from prometheus_client.parser import text_string_to_metric_families

def analyze_metrics(file_path):
    metric_details = defaultdict(list)
    with open(file_path, 'r') as f:
        content = f.read()

    for family in text_string_to_metric_families(content):
        for sample in family.samples:
            labels = dict(sample.labels)
            labels.pop('service_instance_id', None)
            metric_details[family.name].append({
                'labels': labels,
                'value': sample.value
            })
    return metric_details

def compare_metric_details(base_details, pr_details):
    added = []
    removed = []
    changed = []

    base_metrics = set(base_details.keys())
    pr_metrics = set(pr_details.keys())

    # Find added metrics
    for metric in pr_metrics - base_metrics:
        added.append({
            'name': metric,
            'samples': pr_details[metric]
        })

    # Find removed metrics
    for metric in base_metrics - pr_metrics:
        removed.append({
            'name': metric,
            'samples': base_details[metric]
        })

    # Find changed metrics
    for metric in base_metrics & pr_metrics:
        base_samples = {json.dumps(s) for s in base_details[metric]}
        pr_samples = {json.dumps(s) for s in pr_details[metric]}

        if base_samples != pr_samples:
            changed.append({
                'name': metric,
                'added': [s for s in pr_details[metric] if json.dumps(s) not in base_samples],
                'removed': [s for s in base_details[metric] if json.dumps(s) not in pr_samples]
            })

    return {
        'added': added,
        'removed': removed,
        'changed': changed
    }

def generate_markdown_summary(comparison):
    summary = []
    summary.append("## Metrics Comparison Summary")
    summary.append(f"- ✅ **Added metrics:** {len(comparison['added'])}")
    summary.append(f"- ❌ **Removed metrics:** {len(comparison['removed'])}")
    summary.append(f"- 🔄 **Changed metrics:** {len(comparison['changed'])}")

    if comparison['added']:
        summary.append("\n### 🆕 Added Metrics")
        for metric in comparison['added'][:5]:  # Show first 5 for brevity
            summary.append(f"- `{metric['name']}`")
        if len(comparison['added']) > 5:
            summary.append(f"- ... and {len(comparison['added']) - 5} more")

    if comparison['removed']:
        summary.append("\n### 🗑️ Removed Metrics")
        for metric in comparison['removed'][:5]:
            summary.append(f"- `{metric['name']}`")
        if len(comparison['removed']) > 5:
            summary.append(f"- ... and {len(comparison['removed']) - 5} more")

    return '\n'.join(summary)

def main():
    parser = argparse.ArgumentParser(description='Generate metrics comparison summary')
    parser.add_argument('--base', required=True, help='Path to base metric file')
    parser.add_argument('--pr', required=True, help='Path to PR metric file')
    parser.add_argument('--output', required=True, help='Output summary file path')

    args = parser.parse_args()

    base_details = analyze_metrics(args.base)
    pr_details = analyze_metrics(args.pr)

    comparison = compare_metric_details(base_details, pr_details)
    markdown_summary = generate_markdown_summary(comparison)

    with open(args.output, 'w') as f:
        f.write(markdown_summary)

if __name__ == '__main__':
    main()