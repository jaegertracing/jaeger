# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

import argparse
import sys
from difflib import unified_diff
from bisect import insort
from prometheus_client.parser import text_string_to_metric_families

def read_metric_file(file_path):
    with open(file_path, 'r') as f:
        return f.readlines()
    
def parse_metrics(content):
    metrics = []
    for family in text_string_to_metric_families(content):
        for sample in family.samples:
            labels = dict(sample.labels)
            #simply pop undesirable metric labels
            labels.pop('service_instance_id',None)
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
    diff_lines = generate_diff(file1_lines, file2_lines, str(args.file1), str(args.file2))
    
    # Check if there are any differences
    if not diff_lines:
        print("No differences found between the metric files.")
        sys.exit(0)
    
    # Write diff to output file
    write_diff_file(diff_lines, args.output)

if __name__ == '__main__':
    main()