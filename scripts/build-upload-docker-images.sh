#!/bin/bash

set -euxf -o pipefail

mode=${1-main}

make build-binaries-linux

if [[ "$mode" == "pr-only" ]]; then
  make create-baseimg
  # build artifacts for linux/amd64 only for pull requests
  platforms="linux/amd64"
else
  make create-baseimg-debugimg
  platforms="linux/amd64,linux/s390x,linux/ppc64le,linux/arm64"
  # build multi-arch binaries
  make build-binaries-s390x
  make build-binaries-ppc64le
  make build-binaries-arm64
fi

# build/upload raw and debug images of Jaeger backend components
for component in agent collector query ingester remote-storage
do
  bash scripts/build-upload-a-docker-image.sh -b -c "jaeger-${component}" -d "cmd/${component}" -p "${platforms}" -t release
  # do not need debug image built for PRs
  if [[ "$mode" != "pr-only" ]]; then
    bash scripts/build-upload-a-docker-image.sh -b -c "jaeger-${component}-debug" -d "cmd/${component}" -t debug
  fi
done

bash scripts/build-upload-a-docker-image.sh -b -c jaeger-es-index-cleaner -d cmd/es-index-cleaner -p "${platforms}" -t release
bash scripts/build-upload-a-docker-image.sh -b -c jaeger-es-rollover -d cmd/es-rollover  -p "${platforms}" -t release
bash scripts/build-upload-a-docker-image.sh -c jaeger-cassandra-schema -d plugin/storage/cassandra/ -p "${platforms}"

# build/upload images for jaeger-tracegen and jaeger-anonymizer
for component in tracegen anonymizer
do
  bash scripts/build-upload-a-docker-image.sh -c "jaeger-${component}" -d "cmd/${component}" -p "${platforms}"
done
