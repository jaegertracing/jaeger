#!/usr/bin/env python3
import re
import sys

def update_license_header(file_path):
    with open(file_path, 'r') as file:
        content = file.read()

    apache_header_pattern = re.compile(
        r'(/\*|#|//)\s*Licensed under the Apache License, Version 2\.0.*?'
        r'limitations under the License\.',
        re.DOTALL
    )
    spdx_header = "// SPDX-License-Identifier: Apache-2.0\n"

    if content.startswith(spdx_header):
        print(f"Skipping {file_path}: SPDX header already present")
        return False

    new_content, count = apache_header_pattern.subn(spdx_header, content, count=1)

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
