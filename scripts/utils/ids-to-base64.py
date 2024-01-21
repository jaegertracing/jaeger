#!/usr/bin/env python3

import base64
import os
import re
import sys

# Compiled regex patterns for traceId and spanId
trace_id_pattern = re.compile(r'"traceId": "(.+)"')
span_id_pattern = re.compile(r'"spanId": "(.+)"')

def trace_id_base64(match):
    id = int(match.group(1), 16)
    hex_bytes = id.to_bytes(16, 'big')
    b64 = base64.b64encode(hex_bytes).decode('utf-8')
    return f'"traceId": "{b64}"'

def span_id_base64(match):
    id = int(match.group(1), 16)
    hex_bytes = id.to_bytes(8, 'big')
    b64 = base64.b64encode(hex_bytes).decode('utf-8')
    return f'"spanId": "{b64}"'

for file in sys.argv[1:]:
    print(file)
    backup = f'{file}.bak'
    
    with open(file, 'r') as fin:
        content = fin.read()
        content = trace_id_pattern.sub(trace_id_base64, content)
        content = span_id_pattern.sub(span_id_base64, content)

    with open(backup, 'w') as fout:
        fout.write(content)

    os.remove(file)
    os.rename(backup, file)
