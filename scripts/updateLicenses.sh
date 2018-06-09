#!/bin/bash

set -e

python scripts/updateLicense.py $(git ls-files "*\.go" | grep -v -e thrift-gen -e swagger-gen -e bindata_assetfs.go -e jaeger.pb.go -e jaeger.pb.gw.go -e jaeger_test.pb.go)
