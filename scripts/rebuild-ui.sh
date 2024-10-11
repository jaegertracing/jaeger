#!/bin/bash

# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

set -euxf -o pipefail

cd jaeger-ui

git fetch --all --unshallow --tags
git log --oneline --decorate=full -n 10 | cat

last_tag=$(git describe --tags --dirty 2>/dev/null)

if [[ "$last_tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]];  then
    branch_hash=$(git rev-parse HEAD)
    last_tag_hash=$(git rev-parse "$last_tag")

    if [[ "$branch_hash" == "$last_tag_hash" ]]; then
        temp_file=$(mktemp)
        # shellcheck disable=SC2064
        trap "rm -f ${temp_file}" EXIT
        release_url="https://github.com/jaegertracing/jaeger-ui/releases/download/${last_tag}/assets.tar.gz"
        if curl --silent --fail --location --output "$temp_file" "$release_url"; then

            mkdir -p packages/jaeger-ui/build/
            rm -r -f packages/jaeger-ui/build/
            tar -zxvf "$temp_file" packages/jaeger-ui/build/
            exit 0
        fi
    fi
fi

# do a regular full build
npm ci && cd packages/jaeger-ui && npm run build
