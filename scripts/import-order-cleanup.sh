#!/bin/bash

set -e

python scripts/import-order-cleanup.py -o inplace -t $(git ls-files "*\.go" | grep -v -e thrift-gen -e swagger-gen -e thrift-0.9.2)
