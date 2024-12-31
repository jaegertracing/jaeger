#!/bin/bash
#
# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

platforms=$(make echo-platforms)
for main in ./cmd/*/main.go; do
    dir=$(dirname "$main")
    bin=$(basename "$dir")
    rm -rf "${dir:?}/$bin"
    for platform in $(echo "$platforms" | tr ',' ' ' | tr '/' '-'); do
      b="${dir:?}/$bin-$platform"
      echo "$b"
      rm -f "$b"
      b="${dir:?}/$bin-debug-$platform"
      echo "$b"
      rm -f "$b"
    done
done
