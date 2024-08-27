#!/bin/bash

# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

# Extract and parse Jaeger release version from the closest Git tag.

set -ef -o pipefail

if [[ -z $QUIET ]]; then
  set -x
fi
SED=${SED:-sed}

case $1 in
  v1)
    JAEGER_MAJOR=v1
    ;;
  v2)
    JAEGER_MAJOR=v2
    ;;
  *)
    echo "Jaeger major version is required as argument: v1|v2"
    exit 1
esac

# strict mode for variables
set -u

# GIT_SHA=$(git rev-parse HEAD)

# Some of GitHub Actions workflows do a shallow checkout without tags. This avoids logging warnings from git.
if [[ $(git rev-parse --is-shallow-repository) == "false" ]]; then
  GIT_CLOSEST_TAG=$(git describe --abbrev=0 --tags)
else
  echo "The repository is a shallow clone, cannot determine most recent tag"
  exit 1
fi

MATCHING_TAG=''
for tag in $(git tag --list --contains "$(git rev-parse "$GIT_CLOSEST_TAG")"); do
  echo "found tag $tag" >&2
  if [[ "${tag:0:2}" == "$JAEGER_MAJOR" ]]; then
    MATCHING_TAG="$tag"
    break
  fi
done
if [[ "$MATCHING_TAG" == "" ]]; then
  echo "Did not find a tag matching major version $JAEGER_MAJOR"
  exit 1
else
  echo "Using tag $MATCHING_TAG" >&2
fi

if [[ $MATCHING_TAG =~ ^v([0-9]+)\.([0-9]+)\.([0-9]+) ]]; then
    MAJOR="${BASH_REMATCH[1]}"
    MINOR="${BASH_REMATCH[2]}"
    PATCH="${BASH_REMATCH[3]}"
else
    echo "Invalid semver format: $MATCHING_TAG"
    exit 1
fi

echo $MATCHING_TAG $MAJOR $MINOR $PATCH
