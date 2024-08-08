#!/usr/bin/env python3
import re
import sys

def update_license_header(file_path, dry_run=False):
    with open(file_path, 'r') as file:
        content = file.read()

    # Pattern to match the old header
    header_pattern = re.compile(r'(?s)(// Copyright.*?)\n.*?limitations under the License\.\s*')
    
    match = header_pattern.match(content)
    if match:
        if dry_run:
            print(f"Would update {file_path}")
            return True
        
        new_header = f"{match.group(1)}\n// SPDX-License-Identifier: Apache-2.0\n\n"
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
        print("Usage: python update_license_headers.py [--dry-run] <file> [<file> ...]")
        sys.exit(1)

    if dry_run:
        print("Performing dry run - no files will be modified")

    total_updated = sum(update_license_header(file, dry_run) for file in files)
    print(f"Total files {'that would be' if dry_run else ''} updated: {total_updated}")

if __name__ == "__main__":
    main()
