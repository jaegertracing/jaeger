#!/usr/bin/env python3
# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

# Replace Apache 2.0 license headers with SPDX license identifiers.

import sys

# Line-oriented match for the old multi-line Apache header. The previous
# DOTALL regex used overlapping `// Copyright.*?` repetition and was a
# ReDoS risk (CodeQL py/redos). Each pass below consumes a whole line.
_COPYRIGHT_PREFIX = '// Copyright'
_BLANK_COMMENT = '//'
_LICENSE_START = '// Licensed under the Apache License'
_LICENSE_END = 'limitations under the License.'


def _split_first_line(text, start):
    newline = text.find('\n', start)
    if newline == -1:
        return None, None, None
    return text[start:newline], newline, newline + 1


def match_old_apache_header(content):
    """Return (old_header, match_end) for a leading Apache header, or (None, 0).

    old_header is the text through 'limitations under the License.' (the old
    regex capture). match_end is the index after the trailing whitespace and
    newline the old regex consumed, so replacement still drops that suffix.
    """
    if not content.startswith(_COPYRIGHT_PREFIX):
        return None, 0

    # First line is a copyright line. Consume later // comments until the
    # blank `//` that introduces the Apache license block. Intervening
    # comments (extra copyrights, an existing SPDX line) are part of the
    # old header and must stay in the capture so the skip-if-SPDX path runs.
    line, _, pos = _split_first_line(content, 0)
    if line is None:
        return None, 0

    while True:
        line, _, next_pos = _split_first_line(content, pos)
        if line is None:
            return None, 0
        if line == _BLANK_COMMENT:
            peek, _, peek_next = _split_first_line(content, next_pos)
            if peek is not None and peek.startswith(_LICENSE_START):
                pos = peek_next
                break
        if not line.startswith('//'):
            return None, 0
        pos = next_pos

    while True:
        line, _, next_pos = _split_first_line(content, pos)
        if line is None or not line.startswith('//'):
            return None, 0
        if _LICENSE_END in line:
            # Capture ends at 'License.', matching the old regex group.
            # Then consume `\s*\n` (longest whitespace prefix that ends
            # on a newline) so the replacement still drops that suffix.
            end_in_line = line.find(_LICENSE_END) + len(_LICENSE_END)
            old_header_end = pos + end_in_line
            rest = content[old_header_end:]
            last_nl = -1
            i = 0
            while i < len(rest) and rest[i] in ' \t\r\n\f\v':
                if rest[i] == '\n':
                    last_nl = i
                i += 1
            if last_nl == -1:
                return None, 0
            return content[:old_header_end], old_header_end + last_nl + 1
        pos = next_pos


def replace_license_header(file_path, dry_run=False):
    with open(file_path, 'r') as file:
        content = file.read()

    old_header, match_end = match_old_apache_header(content)
    if old_header is not None:
        if "SPDX-License-Identifier: Apache-2.0" in old_header:
            print(f"Skipping {file_path}: SPDX identifier already present")
            return False

        if dry_run:
            print(f"Would update {file_path}")
            return True

        copyright_lines = [
            line for line in old_header.splitlines()
            if line.startswith(_COPYRIGHT_PREFIX)
        ]
        new_header = "\n".join(copyright_lines) + "\n// SPDX-License-Identifier: Apache-2.0\n\n"

        new_content = new_header + content[match_end:]

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
