# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

import argparse
import sys
from difflib import unified_diff
from bisect import insort
from prometheus_client.parser import text_string_to_metric_families
import re

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
                
    
    return False, None


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
    for family in text_string_to_metric_families(content):
        for sample in family.samples:
            labels = dict(sample.labels)
            
            should_exclude= should_exclude_metric(sample.name, labels)
            if should_exclude:
                continue

            labels.pop('service_instance_id', None)
            labels = suppress_transient_labels(sample.name, labels)
            
            label_pairs = sorted(labels.items(), key=lambda x: x[0])
            label_str = ','.join(f'{k}="{v}"' for k,v in label_pairs)
            metric = f"{family.name}{{{label_str}}}"
            insort(metrics , metric)
        
    return metrics


def generate_diff(file1_content, file2_content):
    if isinstance(file1_content, list):
        file1_content = ''.join(file1_content)
    if isinstance(file2_content, list):
        file2_content = ''.join(file2_content)

    metrics1 = parse_metrics(file1_content)
    metrics2 = parse_metrics(file2_content)

    diff = unified_diff(metrics1, metrics2,lineterm='',n=0)
    
    return '\n'.join(diff)

def write_diff_file(diff_lines, output_path):
    
    with open(output_path, 'w') as f:
        f.write(diff_lines)
        f.write('\n')  # Add final newline
        print(f"Diff file successfully written to: {output_path}")

def main():
    parser = argparse.ArgumentParser(description='Generate diff between two Jaeger metric files')
    parser.add_argument('--file1', help='Path to first metric file')
    parser.add_argument('--file2', help='Path to second metric file')
    parser.add_argument('--output', '-o', default='metrics_diff.txt',
                       help='Output diff file path (default: metrics_diff.txt)')
    
    args = parser.parse_args()
    
    # Read input files
    file1_lines = read_metric_file(args.file1)
    file2_lines = read_metric_file(args.file2)
    
    # Generate diff
    diff_lines = generate_diff(file1_lines, file2_lines)
    
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
