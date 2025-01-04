# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

import argparse
import sys
import json 
import deepdiff
from pathlib import Path
from prometheus_client.parser import text_string_to_metric_families

def read_metric_file(file_path):
    with open(file_path, 'r') as f:
        return f.readlines()
    
def parse_metrics(content):
    metrics = {}
    for family in text_string_to_metric_families(content):
        for sample in family.samples:
            key = (family.name, frozenset(sorted(sample.labels.items())))
            metrics[key] = {
                'name': family.name,
                'labels': dict(sample.labels),
            }
    return metrics

def generate_diff(file1_content, file2_content):
    if isinstance(file1_content, list):
        file1_content = ''.join(file1_content)
    if isinstance(file2_content, list):
        file2_content = ''.join(file2_content)

    metrics1 = parse_metrics(file1_content)
    metrics2 = parse_metrics(file2_content)

    # Compare metrics ignoring values
    diff = deepdiff.DeepDiff(metrics1, metrics2,ignore_order=True,exclude_paths=["root['value']"])
    diff_dict = diff.to_dict()
    diff_dict['metrics_in_tagged_only'] = diff_dict.pop('dictionary_item_added')
    diff_dict['metrics_in_current_only'] = diff_dict.pop('dictionary_item_removed')
    diff_json = json.dumps(diff_dict,indent=4,default=tuple,sort_keys=True)
    return diff_json

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
    diff_lines = generate_diff(file1_lines, file2_lines, str(args.file1), str(args.file2))
    
    # Check if there are any differences
    if not diff_lines:
        print("No differences found between the metric files.")
        sys.exit(0)
    
    # Write diff to output file
    write_diff_file(diff_lines, args.output)

if __name__ == '__main__':
    main()