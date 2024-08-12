#!/bin/bash

# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

set -euf -o pipefail

print_help() {
  echo "Usage: $0 [-D] [-l] [-p platforms]"
  echo "-h: Print help"
  echo "-D: Disable building of images with debugger"
  echo "-l: Enable local-only mode that only pushes images to local registry"
  echo "-p: Comma-separated list of platforms to build for (default: all supported)"
  exit 1
}

add_debugger='Y'
platforms="linux/amd64,linux/s390x,linux/ppc64le,linux/arm64"
LOCAL_FLAG=''

while getopts "Dhlp:" opt; do
	case "${opt}" in
	D)
		add_debugger='N'
		;;
	l)
		# in the local-only mode the images will only be pushed to local registry
		LOCAL_FLAG='-l'
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

# Loop through each platform (separated by commas)
for platform in $(echo "$platforms" | tr ',' ' '); do
  # Extract the architecture from the platform string
  arch=${platform##*/}  # Remove everything before the last slash
  make "build-binaries-$arch"
done

if [[ "${add_debugger}" == "N" ]]; then
  make create-baseimg
else
  make create-baseimg-debugimg
fi

# build/upload raw and debug images of Jaeger backend components
for component in agent collector query ingester remote-storage
do
  bash scripts/build-upload-a-docker-image.sh ${LOCAL_FLAG} -b -c "jaeger-${component}" -d "cmd/${component}" -p "${platforms}" -t release
  # do not need debug image built for PRs
  if [[ "${add_debugger}" == "Y" ]]; then
    bash scripts/build-upload-a-docker-image.sh ${LOCAL_FLAG} -b -c "jaeger-${component}-debug" -d "cmd/${component}" -t debug
  fi
done

bash scripts/build-upload-a-docker-image.sh ${LOCAL_FLAG} -b -c jaeger-es-index-cleaner -d cmd/es-index-cleaner -p "${platforms}" -t release
bash scripts/build-upload-a-docker-image.sh ${LOCAL_FLAG} -b -c jaeger-es-rollover -d cmd/es-rollover  -p "${platforms}" -t release
bash scripts/build-upload-a-docker-image.sh ${LOCAL_FLAG} -c jaeger-cassandra-schema -d plugin/storage/cassandra/ -p "${platforms}"

# build/upload images for jaeger-tracegen and jaeger-anonymizer
for component in tracegen anonymizer
do
  bash scripts/build-upload-a-docker-image.sh ${LOCAL_FLAG} -c "jaeger-${component}" -d "cmd/${component}" -p "${platforms}"
done
