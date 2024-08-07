#!/usr/bin/env python3
import re
import sys

def update_license_header(file_path):
    with open(file_path, 'r') as file:
        content = file.read()

    # Pattern to match copyright lines and Apache license
    header_pattern = re.compile(
        r'^(// Copyright.*\n)+'  # Match one or more copyright lines
        r'(//\s*\n)*'  # Match zero or more empty comment lines
        r'// Licensed under the Apache License, Version 2\.0.*?'
        r'limitations under the License\.\n+',
        re.MULTILINE | re.DOTALL
    )

    spdx_header = "// SPDX-License-Identifier: Apache-2.0\n"

    new_content, count = header_pattern.subn(
        lambda m: ''.join(re.findall(r'^// Copyright.*\n', m.group(0), re.MULTILINE)) + spdx_header,
        content
    )

    if count > 0:
        with open(file_path, 'w') as file:
            file.write(new_content)
        print(f"Updated {file_path}")
        return True
    else:
        print(f"Skipping {file_path}: No Apache header found")
        return False

def main():
    if len(sys.argv) < 2:
        print("Usage: python update_license_headers.py <file> [<file> ...]")
        sys.exit(1)

    total_updated = sum(update_license_header(file) for file in sys.argv[1:])
    print(f"Total files updated: {total_updated}")

if __name__ == "__main__":
    main()
