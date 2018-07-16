#!/bin/bash

set -e

python scripts/updateLicense.py $(git ls-files "*\.go" | \
    grep -v \
        -e thrift-gen \
        -e swagger-gen \
        -e statik.go \
        -e model.pb.go \
        -e model.pb.gw.go \
        -e model_test.pb.go \
)
