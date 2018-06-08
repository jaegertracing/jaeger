import base64
import re
import os
import sys

def trace_id_base64(match):
    id = int(match.group(1), 16)
    hex = '%032x' % id
    b64 = base64.b64encode(hex.decode('hex'))
    return '"traceId": "%s"' % b64

def span_id_base64(match):
    id = int(match.group(1), 16)
    hex = '%016x' % id
    b64 = base64.b64encode(hex.decode('hex'))
    return '"spanId": "%s"' % b64

for file in sys.argv[1:]:
    print file
    backup = '%s.bak' % file
    with open(file, 'r') as fin:
        with open(backup, 'w') as fout:
            for line in fin:
                # line = line[:-1] # remove \n
                line = re.sub(r'"traceId": "(.+)"', trace_id_base64, line)
                line = re.sub(r'"spanId": "(.+)"', span_id_base64, line)
                # print(line)
                fout.write(line)
    os.remove(file)
    os.rename(backup, file)
