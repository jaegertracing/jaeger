# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

import argparse
import sys
from difflib import unified_diff
from bisect import insort
from prometheus_client.parser import text_string_to_metric_families
import re

EXCLUDED_LABELS = {'service_instance_id', 'otel_scope_version', 'otel_scope_schema_url'}

# Configuration for transient labels that should be normalized during comparison
TRANSIENT_LABEL_PATTERNS = {
    'kafka': {
        'topic': {
            'pattern': r'jaeger-spans-\d+',
            'replacement': 'jaeger-spans-'
        }
    },
    # Add more patterns here as needed
    # Example:
    # 'elasticsearch': {
    #     'index': {
    #         'pattern': r'jaeger-\d{4}-\d{2}-\d{2}',
    #         'replacement': 'jaeger-YYYY-MM-DD'
    #     }
    # }
}

METRIC_EXCLUSION_RULES = {
    # excluding HTTP 5xx responses as these can be flaky
    'http_5xx_errors': {
        'condition': 'label_match',
        'label': 'http_response_status_code',
        'pattern': r'^5\d{2}$',
    },
    # excluding transient Badger metrics that may or may not appear
    'badger_pending_memtable': {
        'condition': 'metric_name_match',
        'pattern': r'^jaeger_storage_badger_write_pending_num_memtable',
    },
}

def should_exclude_metric(metric_name, labels):
    """
    Determines if a metric should be excluded from comparison based on configured rules.
    
    Args:
        metric_name: The name of the metric
        labels: Dictionary of labels for the metric
        
    Returns:
        tuple: (should_exclude: bool, reason: str or None)
    """
    for rule_name, rule_config in METRIC_EXCLUSION_RULES.items():
        condition = rule_config['condition']
        
        if condition == 'label_match':
            label = rule_config['label']
            pattern = rule_config['pattern']
            if label in labels and re.match(pattern, labels[label]):
                return True
        elif condition == 'metric_name_match':
            pattern = rule_config['pattern']
            if re.match(pattern, metric_name):
                return True
    return False


def suppress_transient_labels(metric_name, labels):
    """
    Suppresses transient labels in metrics based on configured patterns.
    
    Args:
        metric_name: The name of the metric
        labels: Dictionary of labels for the metric
        
    Returns:
        Dictionary of labels with transient values normalized
    """
    labels_copy = labels.copy()
    
    for service_pattern, label_configs in TRANSIENT_LABEL_PATTERNS.items():
        if service_pattern in metric_name:
            for label_name, pattern_config in label_configs.items():
                if label_name in labels_copy:
                    pattern = pattern_config['pattern']
                    replacement = pattern_config['replacement']
                    labels_copy[label_name] = re.sub(pattern, replacement, labels_copy[label_name])
    
    return labels_copy

def read_metric_file(file_path):
    with open(file_path, 'r') as f:
        return f.readlines()
    
def parse_metrics(content):
    metrics = []
    metrics_exclusion_count = 0
    for family in text_string_to_metric_families(content):
        for sample in family.samples:
            labels = dict(sample.labels)

            if should_exclude_metric(sample.name, labels):
                metrics_exclusion_count += 1
                continue

            # Remove undesirable metric labels to match the diff generation
            for label in EXCLUDED_LABELS:
                labels.pop(label, None)
            labels = suppress_transient_labels(sample.name, labels)
            
            label_pairs = sorted(labels.items(), key=lambda x: x[0])
            label_str = ','.join(f'{k}="{v}"' for k,v in label_pairs)
            metric = f"{family.name}{{{label_str}}}"
            insort(metrics , metric)
        
    return metrics,metrics_exclusion_count


def generate_diff(baseline_content, current_content):
    """Compare two Prometheus metrics snapshots and return a unified diff of metric names.

    The input files are raw Prometheus text exposition format, scraped directly from
    the Jaeger /metrics endpoint by e2e_integration.go (scrapeMetrics), e.g.:
        # HELP http_requests_total The total number of HTTP requests.
        # TYPE http_requests_total counter
        http_requests_total{method="post",code="200"} 1027 1395066363000
        http_requests_total{method="post",code="400"}    3 1395066363000

    parse_metrics() is where metric values and timestamps are dropped, retaining only
    the metric name and its normalised label set as a string like:
        http_requests_total{code="200",method="post"}
    Certain labels (e.g. service_instance_id) are dropped and entire samples
    (e.g. HTTP 5xx responses) are excluded to reduce run-to-run noise.
    This exclusion happens here at analysis time, not at snapshot capture time;
    the snapshot files always contain the full raw scrape output.

    The diff is performed on these sorted, value-free metric strings.  If the two
    snapshots produce the same set of strings the diff is empty and this function
    returns ''.  When there are differences, the return value is a unified diff
    using the standard convention:
        - lines  = present in baseline but absent from current → regression/removed
        + lines  = present in current but absent from baseline → newly added
    followed by optional comment lines reporting how many metrics were excluded, e.g.:
        # Metrics excluded from baseline: 3
        # Metrics excluded from current: 5
    These comment lines (prefixed with `# `) are appended only when the diff is
    non-empty; they are informational context, not metric differences themselves.
    """
    if isinstance(baseline_content, list):
        baseline_content = ''.join(baseline_content)
    if isinstance(current_content, list):
        current_content = ''.join(current_content)

    baseline_metrics, excluded_count_baseline = parse_metrics(baseline_content)
    current_metrics, excluded_count_current = parse_metrics(current_content)

    # unified_diff(baseline, current): - = in baseline but not current (removed/regression),
    #                                  + = in current but not baseline (newly added).
    diff = list(unified_diff(baseline_metrics, current_metrics, lineterm='', n=0))

    # Exclusion counts are informational context appended to the diff output.
    # They must not be written when the diff itself is empty: two snapshots with
    # identical non-excluded metrics but different numbers of excluded samples
    # would otherwise produce a non-empty output with no actionable differences.
    if len(diff) == 0:
        return ''

    total_excluded = excluded_count_baseline + excluded_count_current

    exclusion_lines = ''
    if total_excluded > 0:
        exclusion_lines = f'\n# Metrics excluded from baseline: {excluded_count_baseline}\n# Metrics excluded from current: {excluded_count_current}'

    return '\n'.join(diff) + exclusion_lines

def write_diff_file(diff_lines, output_path):
    
    with open(output_path, 'w') as f:
        f.write(diff_lines)
        f.write('\n')  # Add final newline
        print(f"Diff file successfully written to: {output_path}")

def main():
    parser = argparse.ArgumentParser(description='Generate diff between two Jaeger metric files')
    parser.add_argument('--current', help='Path to the current metric file (e.g. from the PR)')
    parser.add_argument('--baseline', help='Path to the baseline metric file (e.g. from main branch)')
    parser.add_argument('--output', '-o', default='metrics_diff.txt',
                       help='Output diff file path (default: metrics_diff.txt)')

    args = parser.parse_args()

    # Read input files
    baseline_lines = read_metric_file(args.baseline)
    current_lines = read_metric_file(args.current)

    # Generate diff
    diff_lines = generate_diff(baseline_lines, current_lines)

    # Check if there are any differences
    if diff_lines:
        print("differences found between the metric files.")
        print("=== Metrics Comparison Results ===")
        print(diff_lines)
        write_diff_file(diff_lines, args.output)
        return 1

    print("no difference found")
    return 0

if __name__ == '__main__':
    sys.exit(main())
