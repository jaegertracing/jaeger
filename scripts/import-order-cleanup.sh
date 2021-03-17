#!/bin/bash

set -e

python scripts/import-order-cleanup.py -o $1 -t $(git ls-files "*\.go" | \
    grep -v \
        -e gen_assets.go \
        -e model_test.pb.go \
        -e model.pb.go \
        -e proto-gen \
        -e storage_test.pb.go \
        -e swagger-gen \
        -e thrift-0.9.2 \
        -e thrift-gen \
        -e v2/
)
