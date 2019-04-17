#!/bin/bash

set -e

python scripts/import-order-cleanup.py -o $1 -t $(git ls-files "*\.go" | \
    grep -v \
        -e thrift-gen \
        -e swagger-gen \
        -e thrift-0.9.2 \
        -e gen_assets.go \
        -e proto-gen \
        -e model.pb.go \
        -e model_test.pb.go \
        -e storage_test.pb.go
)
