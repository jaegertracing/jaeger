# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

import argparse
import re
import sys
from pathlib import Path

def read_metric_file(file_path):
    try:
        with open(file_path, 'r') as f:
            return f.readlines()
    except Exception as e:
        print(f"Error reading file {file_path}: {str(e)}")

def parse_metric_line(line):

    # Skip empty lines or comments
    line = line.strip()
    if not line or line.startswith('#'):
        return None
    
    # Extract metric name and labels section
    match = re.match(r'([^{]+)(?:{(.+)})?(?:\s+[-+]?[0-9.eE+-]+)?$', line)
    if not match:
        return None
        
    name = match.group(1)
    labels_str = match.group(2) or ''
    
    # Parse labels into a frozenset of (key, value) tuples
    labels = []
    if labels_str:
        for label in labels_str.split(','):
            label = label.strip()
            if '=' in label:
                key, value = label.split('=', 1)
                labels.append((key.strip(), value.strip()))
    
    # Sort labels and convert to frozenset for comparison
    return (name, frozenset(sorted(labels)))

def generate_diff(file1_lines, file2_lines, file1_name, file2_name):

    # Parse metrics from both files
    metrics1 = {}  # {(name, labels_frozenset): original_line}
    metrics2 = {}
    
    for line in file1_lines:
        parsed = parse_metric_line(line)
        if parsed:
            metrics1[parsed] = line.strip()
            
    for line in file2_lines:
        parsed = parse_metric_line(line)
        if parsed:
            metrics2[parsed] = line.strip()
    
    # Generate diff
    diff_lines = []
    diff_lines.append(f'--- {file1_name}')
    diff_lines.append(f'+++ {file2_name}')
    
    # Find metrics unique to file1 (removed metrics)
    for metric in sorted(set(metrics1.keys()) - set(metrics2.keys())):
        name, labels = metric
        diff_lines.append(f'-{metrics1[metric]}')
    
    # Find metrics unique to file2 (added metrics)
    for metric in sorted(set(metrics2.keys()) - set(metrics1.keys())):
        name, labels = metric
        diff_lines.append(f'+{metrics2[metric]}')
    
    return diff_lines

def write_diff_file(diff_lines, output_path):
    try:
        with open(output_path, 'w') as f:
            f.write('\n'.join(diff_lines))
            f.write('\n')  # Add final newline
        print(f"Diff file successfully written to: {output_path}")
    except Exception as e:
        print(f"Error writing diff file: {str(e)}")

def main():
    parser = argparse.ArgumentParser(description='Generate diff between two Jaeger metric files')
    parser.add_argument('--file1', help='Path to first metric file')
    parser.add_argument('--file2', help='Path to second metric file')
    parser.add_argument('--output', '-o', default='metrics_diff.txt',
                       help='Output diff file path (default: metrics_diff.txt)')
    
    args = parser.parse_args()
    
    # Convert paths to absolute paths
    file1_path = Path(args.file1).absolute()
    file2_path = Path(args.file2).absolute()
    output_path = Path(args.output).absolute()
    
    # Read input files
    file1_lines = read_metric_file(file1_path)
    file2_lines = read_metric_file(file2_path)
    
    # Generate diff
    diff_lines = generate_diff(file1_lines, file2_lines, str(file1_path), str(file2_path))
    
    # Check if there are any differences
    if not diff_lines:
        print("No differences found between the metric files.")
        sys.exit(0)
    
    # Write diff to output file
    write_diff_file(diff_lines, output_path)

if __name__ == '__main__':
    main()