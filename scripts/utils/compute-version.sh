#!/bin/bash

# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

# Extract and parse Jaeger release version from the closest Git tag.

set -euf -o pipefail
SED=${SED:-sed}

usage() {
  echo "Usage: $0 [-s] [-v] <jaeger_version>"
  echo "  -s  split semver into 4 parts: semver major minor patch"
  echo "  -v  verbose"
  echo "  jaeger_version:  major version, v1 | v2"
  exit 1
}

verbose="false"
split="false"

while getopts "sv" opt; do
	# shellcheck disable=SC2220 # we don't need a *) case
	case "${opt}" in
	s)
		split="true"
		;;
	v)
		verbose="true"
		;;
	esac
done

shift $((OPTIND - 1))

case $1 in
  v1)
    JAEGER_MAJOR=v1
    ;;
  v2)
    JAEGER_MAJOR=v2
    ;;
  *)
    echo "Jaeger major version is required as argument"
    usage
esac

print_result() {
  if [[ "$split" == "true" ]]; then
    echo "$1" "$2" "$3" "$4"
  else
    echo "$1"
  fi
}

if [[ "$verbose" == "true" ]]; then
  set -x
fi

# Some of GitHub Actions workflows do a shallow checkout without tags. This avoids logging warnings from git.
if [[ $(git rev-parse --is-shallow-repository) == "false" ]]; then
  GIT_CLOSEST_TAG=$(git describe --abbrev=0 --tags)
else
  if [[ "$verbose" == "true" ]]; then
    echo "The repository is a shallow clone, cannot determine most recent tag" >&2
  fi
  print_result 0.0.0 0 0 0
  exit
fi

MATCHING_TAG=''
for tag in $(git tag --list --contains "$(git rev-parse "$GIT_CLOSEST_TAG")"); do
  if [[ "${tag:0:2}" == "$JAEGER_MAJOR" ]]; then
    MATCHING_TAG="$tag"
    break
  fi
done
if [[ "$MATCHING_TAG" == "" ]]; then
  if [[ "$verbose" == "true" ]]; then
    echo "Did not find a tag matching major version $JAEGER_MAJOR" >&2
  fi
  print_result 0.0.0 0 0 0
  exit
fi

if [[ $MATCHING_TAG =~ ^v([0-9]+)\.([0-9]+)\.([0-9]+) ]]; then
    MAJOR="${BASH_REMATCH[1]}"
    MINOR="${BASH_REMATCH[2]}"
    PATCH="${BASH_REMATCH[3]}"
else
    echo "Invalid semver format: $MATCHING_TAG"
    exit 1
fi

print_result "$MATCHING_TAG" "$MAJOR" "$MINOR" "$PATCH"
