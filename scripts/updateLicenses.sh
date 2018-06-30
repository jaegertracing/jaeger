#!/bin/bash

set -e

python scripts/updateLicense.py $(git ls-files "*\.go" | \
    grep -v \
        -e thrift-gen \
        -e swagger-gen \
        -e bindata_assetfs.go \
        -e model.pb.go \
        -e model.pb.gw.go \
        -e model_test.pb.go \
)
