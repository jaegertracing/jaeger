#!/usr/bin/env python3
# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

# Replace Apache 2.0 license headers with SPDX license identifiers.

import re
import sys

def replace_license_header(file_path, dry_run=False):
    with open(file_path, 'r') as file:
        content = file.read()

    # Pattern to match the entire old header, including multiple copyright lines
    header_pattern = re.compile(r'(?s)^(// Copyright.*?(?:\n// Copyright.*?)*\n//\n// Licensed under the Apache License.*?limitations under the License\.)\s*\n')
    
    match = header_pattern.match(content)
    if match:
        old_header = match.group(1)
        if "SPDX-License-Identifier: Apache-2.0" in old_header:
            print(f"Skipping {file_path}: SPDX identifier already present")
            return False
        
        if dry_run:
            print(f"Would update {file_path}")
            return True
        
        # Preserve all copyright lines and add SPDX identifier
        copyright_lines = re.findall(r'// Copyright.*', old_header)
        new_header = "\n".join(copyright_lines) + "\n// SPDX-License-Identifier: Apache-2.0\n\n"
        
        new_content = header_pattern.sub(new_header, content, count=1)
        
        with open(file_path, 'w') as file:
            file.write(new_content)
        print(f"Updated {file_path}")
        return True
    else:
        print(f"Warning: {file_path} - Could not find expected license header")
        return False

def main():
    dry_run = '--dry-run' in sys.argv
    files = [f for f in sys.argv[1:] if f != '--dry-run']
    
    if not files:
        print("Usage: python replace_license_headers.py [--dry-run] <file> [<file> ...]")
        sys.exit(1)
    
    if dry_run:
        print("Performing dry run - no files will be modified")
    
    total_updated = sum(replace_license_header(file, dry_run) for file in files)
    print(f"Total files {'that would be' if dry_run else ''} updated: {total_updated}")

if __name__ == "__main__":
    main()
