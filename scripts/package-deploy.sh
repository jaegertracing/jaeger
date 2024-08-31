#!/bin/bash

# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0
set -euxf -o pipefail

TARCMD=${TARCMD:-tar}

platforms="$(make echo-platforms)"
while getopts "p:" opt; do
	# shellcheck disable=SC2220 # we don't need a *) case
	case "${opt}" in
	p)
		platforms=${OPTARG}
		;;
	esac
done

# stage-platform-files stages the different the platform ($1) into the package
# staging dir ($2). If you pass in a file extension ($3) it will be used when
# copying on the source
function stage-platform-files-v1 {
    local -r PLATFORM=$1
    local -r PACKAGE_STAGING_DIR=$2
    local -r FILE_EXTENSION=${3:-}

    cp "./cmd/all-in-one/all-in-one-${PLATFORM}"  "${PACKAGE_STAGING_DIR}/jaeger-all-in-one${FILE_EXTENSION}"
    cp "./cmd/agent/agent-${PLATFORM}"            "${PACKAGE_STAGING_DIR}/jaeger-agent${FILE_EXTENSION}"
    cp "./cmd/query/query-${PLATFORM}"            "${PACKAGE_STAGING_DIR}/jaeger-query${FILE_EXTENSION}"
    cp "./cmd/collector/collector-${PLATFORM}"    "${PACKAGE_STAGING_DIR}/jaeger-collector${FILE_EXTENSION}"
    cp "./cmd/ingester/ingester-${PLATFORM}"      "${PACKAGE_STAGING_DIR}/jaeger-ingester${FILE_EXTENSION}"
    cp "./examples/hotrod/hotrod-${PLATFORM}"     "${PACKAGE_STAGING_DIR}/example-hotrod${FILE_EXTENSION}"
}

function stage-platform-files-v2 {
    local -r PLATFORM=$1
    local -r PACKAGE_STAGING_DIR=$2
    local -r FILE_EXTENSION=${3:-}

    cp "./cmd/jaeger/jaeger-${PLATFORM}"          "${PACKAGE_STAGING_DIR}/jaeger${FILE_EXTENSION}"
    cp "./examples/hotrod/hotrod-${PLATFORM}"     "${PACKAGE_STAGING_DIR}/example-hotrod${FILE_EXTENSION}"
}

# stage-tool-platform-files stages the different tool files in the platform ($1) into the package
# staging dir ($2). If you pass in a file extension ($3) it will be used when
# copying on the source
function stage-tool-platform-files {
    local -r PLATFORM=$1
    local -r TOOLS_PACKAGE_STAGING_DIR=$2
    local -r FILE_EXTENSION=${3:-}

    cp "./cmd/es-index-cleaner/es-index-cleaner-${PLATFORM}"  "${TOOLS_PACKAGE_STAGING_DIR}/jaeger-es-index-cleaner${FILE_EXTENSION}"
    cp "./cmd/es-rollover/es-rollover-${PLATFORM}"            "${TOOLS_PACKAGE_STAGING_DIR}/jaeger-es-rollover${FILE_EXTENSION}"
    cp "./plugin/storage/es/esmapping-generator-${PLATFORM}"  "${TOOLS_PACKAGE_STAGING_DIR}/jaeger-esmapping-generator${FILE_EXTENSION}"
}

# package pulls built files for the platform ($2) and compresses it using the compression ($1).
# If you pass in a file extension ($3) it will be look for binaries with that extension.
function package {
    local -r COMPRESSION=$1
    local -r PLATFORM=$2
    local -r FILE_EXTENSION=${3:-}
    local -r PACKAGE_NAME_V1=jaeger-${VERSION_V1}-$PLATFORM
    local -r PACKAGE_NAME_V2=jaeger-${VERSION_V2}-$PLATFORM
    local -r TOOLS_PACKAGE_NAME=jaeger-tools-${VERSION_V1}-$PLATFORM

    echo "Packaging binaries for $PLATFORM"

    PACKAGES=("$PACKAGE_NAME_V1" "$PACKAGE_NAME_V2" "$TOOLS_PACKAGE_NAME")
    for d in "${PACKAGES[@]}"; do
      if [ -d "$d" ]; then
        rm -vrf "$d"
      fi
      mkdir "$d"
    done
    stage-platform-files-v1 "$PLATFORM" "$PACKAGE_NAME_V1" "$FILE_EXTENSION"
    stage-platform-files-v2 "$PLATFORM" "$PACKAGE_NAME_V2" "$FILE_EXTENSION"
    stage-tool-platform-files "$PLATFORM" "$TOOLS_PACKAGE_NAME" "$FILE_EXTENSION"
    # Create a checksum file for all the files being packaged in the archive. Sorted by filename.
    for d in "${PACKAGES[@]}"; do
      find "$d" -type f -exec shasum -b -a 256 {} \; | sort -k2 | tee "./deploy/$d.sha256sum.txt"
    done

    if [ "$COMPRESSION" == "zip" ]
    then
      for d in "${PACKAGES[@]}"; do
        local ARCHIVE_NAME="$d.zip"
        echo "Packaging into $ARCHIVE_NAME:"
        zip -r "./deploy/$ARCHIVE_NAME" "$d"
      done
    else
      for d in "${PACKAGES[@]}"; do
        local ARCHIVE_NAME="$d.tar.gz"
        echo "Packaging into $ARCHIVE_NAME:"
        ${TARCMD} --sort=name -czvf "./deploy/$ARCHIVE_NAME" "$d"
      done
    fi
    for d in "${PACKAGES[@]}"; do
      rm -vrf "$d"
    done
}

VERSION_V1="$(make echo-v1 | perl -lne 'print $1 if /^v(\d+.\d+.\d+)$/' )"
VERSION_V2="$(make echo-v2 | perl -lne 'print $1 if /^v(\d+.\d+.\d+(-rc\d+)?)$/' )"
echo "Working on versions: $VERSION_V1 and $VERSION_V2"
if [ -z "$VERSION_V1" ] || [ -z "$VERSION_V2" ]; then
    # We want to halt if for some reason the version string is empty as this is an obvious error case
    >&2 echo 'Failed to detect a version string'
    exit 1
fi

# make needed directories
rm -rf deploy
mkdir deploy

# Loop through each platform (separated by commas)
for platform in $(echo "$platforms" | tr ',' ' '); do
  os="${platform%%/*}"  # Remove everything after the slash
  arch=${platform##*/}  # Remove everything before the last slash
  if [[ "$os" == "windows" ]]; then
    package tar "${os}-${arch}" .exe
    package zip "${os}-${arch}" .exe
  else
    package tar "${os}-${arch}"
  fi
done

# Create a checksum file for all non-checksum files in the deploy directory. Strips the leading 'deploy/' directory from filepaths. Sort by filename.
find deploy \( ! -name '*sha256sum.txt' \) -type f -exec shasum -b -a 256 {} \; \
  | sed -r 's#(\w+\s+\*?)deploy/(.*)#\1\2#' \
  | sort -k2 \
  | tee "./deploy/jaeger-${VERSION_V1}.sha256sum.txt" \
  | tee "./deploy/jaeger-${VERSION_V2}.sha256sum.txt"

# Use gpg to sign the (g)zip files (excluding checksum files) into .asc files.
find deploy \( ! -name '*sha256sum.txt' \) -type f -exec gpg --armor --detach-sign {} \;

# show your work
ls -lF deploy/
