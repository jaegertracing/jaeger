# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

import argparse
import difflib
import sys
from pathlib import Path

def read_metric_file(file_path):
    try:
        with open(file_path, 'r') as f:
            return f.readlines()
    except FileNotFoundError:
        print(f"Error: File not found - {file_path}")
        sys.exit(1)
    except Exception as e:
        print(f"Error reading file {file_path}: {str(e)}")
        sys.exit(1)

def generate_diff(file1_lines, file2_lines, file1_name, file2_name):

    return list(difflib.unified_diff(
        file1_lines,
        file2_lines,
        fromfile=file1_name,
        tofile=file2_name,
        lineterm=''
    ))

def write_diff_file(diff_lines, output_path):
    try:
        with open(output_path, 'w') as f:
            f.write('\n'.join(diff_lines))
            f.write('\n')  # Add final newline
        print(f"Diff file successfully written to: {output_path}")
    except Exception as e:
        print(f"Error writing diff file: {str(e)}")
        sys.exit(1)

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