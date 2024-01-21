#!/usr/bin/env python3

import base64
import os
import re
import sys

# Compiled regex patterns for traceId and spanId
trace_id_pattern = re.compile(r'"traceId": "(.+)"')
span_id_pattern = re.compile(r'"spanId": "(.+)"')

def trace_id_base64(match):
    id_value = int(match.group(1), 16)
    hex_bytes = id_value.to_bytes(16, 'big')
    b64 = base64.b64encode(hex_bytes).decode('utf-8')
    return f'"traceId": "{b64}"'

def span_id_base64(match):
    id_value = int(match.group(1), 16)
    hex_bytes = id_value.to_bytes(8, 'big')
    b64 = base64.b64encode(hex_bytes).decode('utf-8')
    return f'"spanId": "{b64}"'

def process_file(file_path):
    backup_path = f'{file_path}.bak'

    with open(file_path, 'r') as fin:
        content = fin.read()

        # Apply replacements using compiled regex patterns
        content = trace_id_pattern.sub(trace_id_base64, content)
        content = span_id_pattern.sub(span_id_base64, content)

    with open(backup_path, 'w') as fout:
        fout.write(content)

    os.remove(file_path)
    os.rename(backup_path, file_path)

# Process each file provided as a command-line argument
for file_path in sys.argv[1:]:
    print(file_path)
    process_file(file_path)
