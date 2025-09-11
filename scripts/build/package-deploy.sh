#!/bin/bash
#
# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0
set -euxf -o pipefail

# This script uses --sort=name option that is not supported by MacOS tar.
# On MacOS, install `brew install gnu-tar` and run this script with TARCMD=gtar.
TARCMD=${TARCMD:-tar}

print_help() {
  echo "Usage: $0 [-h] [-k gpg_key_id] [-p platforms] [-t track]"
  echo "-h: Print help"
  echo "-k: Override default GPG signing key ID. Use 'skip' to skip signing."
  echo "-p: Comma-separated list of platforms to build for (default: all supported)"
  echo "-t: Build track: v1, v2, or both (default: uses BUILD_TRACK env var or both)"
  exit 1
}

# Default signing key (accessible to maintainers-only), documented in https://www.jaegertracing.io/download/.
gpg_key_id="B42D1DB0F079690F"
platforms="$(make echo-platforms)"

# TODO: Remove v1 support after last scheduled v1 release (early 2026)
# Use BUILD_TRACK environment variable, default to both for backward compatibility
track_override=""

while getopts "hk:p:t:" opt; do
  case "${opt}" in
  k)
    gpg_key_id=${OPTARG}
    ;;
  p)
    platforms=${OPTARG}
    ;;
  t)
    track_override=${OPTARG}
    ;;
  ?)
    print_help
    ;;
  esac
done

# Determine which tracks to build
BUILD_TRACK=${track_override:-${BUILD_TRACK:-both}}

case "$BUILD_TRACK" in
  v1)
    build_v1=true
    build_v2=false
    echo "Building v1 packages only (legacy)"
    ;;
  v2)
    build_v1=false
    build_v2=true
    echo "Building v2 packages only (primary)"
    ;;
  both)
    build_v1=true
    build_v2=true
    echo "Building both v1 (legacy) and v2 (primary) packages"
    ;;
  *)
    echo "ERROR: Invalid track '${BUILD_TRACK}'. Must be v1, v2, or both"
    exit 1
    ;;
esac

# stage-platform-files stages the different the platform ($1) into the package
# staging dir ($2). If you pass in a file extension ($3) it will be used when
# copying on the source
function stage-platform-files-v1 {
    local -r PLATFORM=$1
    local -r PACKAGE_STAGING_DIR=$2
    local -r FILE_EXTENSION=${3:-}

    cp "./cmd/all-in-one/all-in-one-${PLATFORM}"  "${PACKAGE_STAGING_DIR}/jaeger-all-in-one${FILE_EXTENSION}"
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

    cp "./cmd/es-index-cleaner/es-index-cleaner-${PLATFORM}"        "${TOOLS_PACKAGE_STAGING_DIR}/jaeger-es-index-cleaner${FILE_EXTENSION}"
    cp "./cmd/es-rollover/es-rollover-${PLATFORM}"                  "${TOOLS_PACKAGE_STAGING_DIR}/jaeger-es-rollover${FILE_EXTENSION}"
    cp "./cmd/esmapping-generator/esmapping-generator-${PLATFORM}"  "${TOOLS_PACKAGE_STAGING_DIR}/jaeger-esmapping-generator${FILE_EXTENSION}"
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

    echo "Packaging binaries for $PLATFORM (track: $BUILD_TRACK)"

    PACKAGES=()
    
    # Add packages based on build track
    if [[ "$build_v1" == "true" ]]; then
        PACKAGES+=("$PACKAGE_NAME_V1" "$TOOLS_PACKAGE_NAME")
    fi
    if [[ "$build_v2" == "true" ]]; then
        PACKAGES+=("$PACKAGE_NAME_V2")
    fi

    for d in "${PACKAGES[@]}"; do
      if [ -d "$d" ]; then
        rm -vrf "$d"
      fi
      mkdir "$d"
    done
    
    if [[ "$build_v1" == "true" ]]; then
        stage-platform-files-v1 "$PLATFORM" "$PACKAGE_NAME_V1" "$FILE_EXTENSION"
        stage-tool-platform-files "$PLATFORM" "$TOOLS_PACKAGE_NAME" "$FILE_EXTENSION"
    fi
    if [[ "$build_v2" == "true" ]]; then
        stage-platform-files-v2 "$PLATFORM" "$PACKAGE_NAME_V2" "$FILE_EXTENSION"
    fi
    
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

# Get versions based on what we're building
if [[ "$build_v1" == "true" ]]; then
    VERSION_V1="$(make echo-v1 | perl -lne 'print $1 if /^v(\d+.\d+.\d+)$/' )"
fi
if [[ "$build_v2" == "true" ]]; then
    VERSION_V2="$(make echo-v2 | perl -lne 'print $1 if /^v(\d+.\d+.\d+(-rc\d+)?)$/' )"
fi

echo "Working on track: $BUILD_TRACK"
if [[ "$build_v1" == "true" ]]; then
    echo "V1 version: $VERSION_V1"
    if [ -z "$VERSION_V1" ]; then
        >&2 echo 'Failed to detect v1 version string'
        exit 1
    fi
fi
if [[ "$build_v2" == "true" ]]; then
    echo "V2 version: $VERSION_V2"
    if [ -z "$VERSION_V2" ]; then
        >&2 echo 'Failed to detect v2 version string'
        exit 1
    fi
fi

# make needed directories
rm -rf deploy
mkdir deploy

# Loop through each platform (separated by commas)
for platform in $(echo "$platforms" | tr ',' ' '); do
  os=${platform%%/*}  # Remove everything after the slash
  arch=${platform##*/}  # Remove everything before the last slash
  if [[ "$os" == "windows" ]]; then
    package tar "${os}-${arch}" .exe
    package zip "${os}-${arch}" .exe
  else
    package tar "${os}-${arch}"
  fi
done

# Create a checksum file for all non-checksum files in the deploy directory. Strips the leading 'deploy/' directory from filepaths. Sort by filename.
CHECKSUM_BASE_FILES=()
if [[ "$build_v1" == "true" ]]; then
    CHECKSUM_BASE_FILES+=("./deploy/jaeger-${VERSION_V1}.sha256sum.txt")
fi
if [[ "$build_v2" == "true" ]]; then
    CHECKSUM_BASE_FILES+=("./deploy/jaeger-${VERSION_V2}.sha256sum.txt")
fi

find deploy \( ! -name '*sha256sum.txt' \) -type f -exec shasum -b -a 256 {} \; \
  | sed -r 's#(\w+\s+\*?)deploy/(.*)#\1\2#' \
  | sort -k2 \
  | tee "${CHECKSUM_BASE_FILES[@]}"

# Use gpg to sign the (g)zip files (excluding checksum files) into .asc files.
if [[ "${gpg_key_id}" == "skip" ]]; then
  echo "Skipping GPG signing as requested"
else
  echo "Signing archives with GPG key ${gpg_key_id}"
  gpg --list-keys "${gpg_key_id}"
  find deploy \( ! -name '*sha256sum.txt' \) -type f -exec gpg -v --local-user "${gpg_key_id}" --armor --detach-sign {} \;
fi

# show your work
ls -lF deploy/
