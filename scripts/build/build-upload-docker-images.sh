#!/bin/bash
#
# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

set -euf -o pipefail

print_help() {
  echo "Usage: $0 [-B] [-D] [-h] [-l] [-o] [-p platforms]"
  echo "-h: Print help"
  echo "-B: Skip building of the binaries (e.g. when they were already built)"
  echo "-D: Disable building of images with debugger"
  echo "-l: Enable local-only mode that only pushes images to local registry"
  echo "-o: overwrite image in the target remote repository even if the semver tag already exists"
  echo "-p: Comma-separated list of platforms to build for (default: all supported)"
  exit 1
}

add_debugger='Y'
build_binaries='Y'
platforms="$(make echo-linux-platforms)"
FLAGS=()

while getopts "BDhlop:" opt; do
  case "${opt}" in
  B)
    build_binaries='N'
    echo "Will not build binaries as requested"
    ;;
  D)
    add_debugger='N'
    echo "Will not build debug images as requested"
    ;;
  l)
    # in the local-only mode the images will only be pushed to local registry
    FLAGS=("${FLAGS[@]}" -l)
    ;;
  o)
    FLAGS=("${FLAGS[@]}" -o)
    ;;
  p)
    platforms=${OPTARG}
    ;;
  ?)
    print_help
    ;;
  esac
done

set -x

if [[ "$build_binaries" == "Y" ]]; then
  for platform in $(echo "$platforms" | tr ',' ' '); do
    arch=${platform##*/}  # Remove everything before the last slash
    make "build-binaries-linux-$arch"
  done
fi

baseimg_target='create-baseimg-debugimg'
if [[ "${add_debugger}" == "N" ]]; then
  baseimg_target='create-baseimg'
fi
make "$baseimg_target" LINUX_PLATFORMS="$platforms"

# build/upload raw and debug images of Jaeger backend components
for component in collector query ingester remote-storage
do
  bash scripts/build/build-upload-a-docker-image.sh "${FLAGS[@]}" -b -c "jaeger-${component}" -d "cmd/${component}" -p "${platforms}" -t release
  # do not need debug image built for PRs
  if [[ "${add_debugger}" == "Y" ]]; then
    bash scripts/build/build-upload-a-docker-image.sh "${FLAGS[@]}" -b -c "jaeger-${component}-debug" -d "cmd/${component}" -t debug
  fi
done

bash scripts/build/build-upload-a-docker-image.sh "${FLAGS[@]}" -b -c jaeger-es-index-cleaner -d cmd/es-index-cleaner -p "${platforms}" -t release
bash scripts/build/build-upload-a-docker-image.sh "${FLAGS[@]}" -b -c jaeger-es-rollover -d cmd/es-rollover  -p "${platforms}" -t release
bash scripts/build/build-upload-a-docker-image.sh "${FLAGS[@]}" -c jaeger-cassandra-schema -d plugin/storage/cassandra/ -p "${platforms}"

# build/upload images for jaeger-tracegen and jaeger-anonymizer
for component in tracegen anonymizer
do
  bash scripts/build/build-upload-a-docker-image.sh "${FLAGS[@]}" -c "jaeger-${component}" -d "cmd/${component}" -p "${platforms}"
done
