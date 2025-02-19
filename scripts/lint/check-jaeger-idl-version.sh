#!/bin/bash

# Copyright (c) 2025 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

dependency="github.com/jaegertracing/jaeger-idl"

get_gomod_version() {
  gomod_dep=$(grep $dependency <go.mod)
  if [ ! "$gomod_dep" ]; then
    printf "Error: jaeger-idl dependency not found in go mod\n" >&2
    exit 1
  fi
  gomod_version=$(echo "$gomod_dep" | awk '{print $2}')
  echo "$gomod_version"
}

get_submodule_version() {
  cd idl
  commit_version=$(git rev-parse HEAD)
  semver=$(git describe --tags --exact-match "$commit_version")
  if [ ! "$semver" ]; then
    printf "Error: failed getting version from submodule\n" >&2
    exit 1
  fi
  echo "$semver"
}

gomod_semver=$(get_gomod_version) || exit 1
submod_semver=$(get_submodule_version) || exit 1
if [[ "$gomod_semver" != "$submod_semver" ]]; then
  printf "Error: jaeger-idl version mismatch: go.mod %s != submodule %s\n" "$gomod_semver" "$submod_semver" >&2
  exit 1
fi
echo "jaeger-idl version match: OK"
