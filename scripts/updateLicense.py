#!/usr/bin/env python3

# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

import logging
import re
import sys
from datetime import datetime

logging.basicConfig(level=logging.DEBUG)
logger = logging.getLogger(__name__)

CURRENT_YEAR = datetime.today().year

LICENSE_BLOB = """Copyright (c) %d The Jaeger Authors.
SPDX-License-Identifier: Apache-2.0""" % CURRENT_YEAR

def get_license_blob_lines(comment_prefix):
    return [
        (comment_prefix + ' ' + l).strip() + '\n' for l in LICENSE_BLOB.split('\n')
    ]

COPYRIGHT_RE = re.compile(r'Copyright \(c\) (\d+)', re.I)

SHEBANG_RE = re.compile(r'^#!\s*/[^\s]+')

def update_license(name, license_lines):
    with open(name) as f:
        orig_lines = list(f)
    lines = list(orig_lines)

    found = False
    changed = False
    jaeger = False
    for i, line in enumerate(lines[:5]):
        m = COPYRIGHT_RE.search(line)
        if not m:
            continue

        found = True
        jaeger = 'Jaeger' in line

        year = int(m.group(1))
        if year == CURRENT_YEAR:
            break

        # Avoid updating the copyright year.
        #
        # new_line = COPYRIGHT_RE.sub('Copyright (c) %d' % CURRENT_YEAR, line)
        # assert line != new_line, ('Could not change year in: %s' % line)
        # lines[i] = new_line
        # changed = True
        break

    # print('found=%s, changed=%s, jaeger=%s' % (found, changed, jaeger))

    first_line = lines[0]
    shebang_match = SHEBANG_RE.match(first_line)
        
    def replace(header_lines):

        if 'Code generated by' in first_line:
            lines[1:1] = ['\n'] + header_lines
        elif shebang_match:
            lines[1:1] = header_lines
        else:
            lines[0:0] = header_lines

    if not found:
        # depend on file type
        if(shebang_match):
            replace(['\n'] + license_lines)
        else:
            replace(license_lines + ['\n'])

        changed = True
    else:
        if not jaeger:
            replace(license_lines[0])            
            changed = True

    if changed:
        with open(name, 'w') as f:
            for line in lines:
                f.write(line)
        print(name)

def get_license_type(file):
    license_blob_lines_go = get_license_blob_lines('//')
    license_blob_lines_script = get_license_blob_lines('#')

    ext_map = {
        '.go' : license_blob_lines_go,
        '.mk' : license_blob_lines_script,
        'Makefile' : license_blob_lines_script,
        'Dockerfile' : license_blob_lines_script,
        '.py' : license_blob_lines_script,
        '.sh' : license_blob_lines_script,
    }

    license_type = None

    for ext, license in ext_map.items():
        if file.endswith(ext):
            license_type = license
            break

    return license_type

def main():
    if len(sys.argv) == 1:
        print('USAGE: %s FILE ...' % sys.argv[0])
        sys.exit(1)

    for name in sys.argv[1:]:
        license_type = get_license_type(name)
        if license_type:
            try:
                update_license(name, license_type)
            except Exception as error:
                logger.error('Failed to process file %s', name)
                logger.exception(error)
                raise error
        else:
            raise NotImplementedError('Unsupported file type: %s' % name)


if __name__ == "__main__":
    main()
