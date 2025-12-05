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

# Helper function to build and upload docker images
# Args: component_name, source_dir, [use_base_image], [build_debug]
build_image() {
  local component=$1
  local dir=$2
  local use_base_image=${3:-false}
  local build_debug=${4:-false}

  local base_flags=()
  if [[ "$use_base_image" == "true" ]]; then
    base_flags=(-b)
  fi

  bash scripts/build/build-upload-a-docker-image.sh "${FLAGS[@]}" "${base_flags[@]}" -c "$component" -d "$dir" -p "${platforms}" -t release

  if [[ "$build_debug" == "true" ]] && [[ "${add_debugger}" == "Y" ]]; then
    bash scripts/build/build-upload-a-docker-image.sh "${FLAGS[@]}" "${base_flags[@]}" -c "${component}-debug" -d "$dir" -t debug
  fi
}

# Build images with special handling for debug images
build_image jaeger-remote-storage cmd/remote-storage true true

# Build utility images
build_image jaeger-es-index-cleaner cmd/es-index-cleaner true false
build_image jaeger-es-rollover cmd/es-rollover true false
build_image jaeger-cassandra-schema internal/storage/v1/cassandra/ false false

# Build tool images
build_image jaeger-tracegen cmd/tracegen false false
build_image jaeger-anonymizer cmd/anonymizer false false
