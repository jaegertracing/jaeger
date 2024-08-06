#!/usr/bin/env python3

import os
import re
import sys

def update_license_header(file_path):
    with open(file_path, 'r') as file:
        content = file.read()

    # patterns
    apache_header_pattern = re.compile(
        r'(/\*|#|//)\s*Licensed under the Apache License, Version 2\.0.*?'
        r'limitations under the License\.',
        re.DOTALL
    )
    spdx_header = "// SPDX-License-Identifier: Apache-2.0\n"

    # Check if the file already has the SPDX header
    if content.startswith(spdx_header):
        print(f"Skipping {file_path}: SPDX header already present")
        return False

    # Replace Apache header with SPDX
    new_content, count = apache_header_pattern.subn(spdx_header, content, count=1)

    if count > 0:
        with open(file_path, 'w') as file:
            file.write(new_content)
        print(f"Updated {file_path}")
        return True
    else:
        print(f"Skipping {file_path}: No Apache header found")
        return False

def process_directory(directory):
    updated_files = 0
    for root, _, files in os.walk(directory):
        for file in files:
            if file.endswith(('.go')):
                file_path = os.path.join(root, file)
                if update_license_header(file_path):
                    updated_files += 1
    return updated_files

def main():
    if len(sys.argv) != 2:
        print("Usage: python update_license_headers.py <directory>")
        sys.exit(1)

    directory = sys.argv[1]
    if not os.path.isdir(directory):
        print(f"Error: {directory} is not a valid directory")
        sys.exit(1)

    updated_files = process_directory(directory)
    print(f"Total files updated: {updated_files}")

if __name__ == "__main__":
    main()
