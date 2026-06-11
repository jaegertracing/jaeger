#!/bin/bash

# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

set -euxf -o pipefail

# JAEGER_UI_DIR can be overridden to use a local checkout instead of the submodule:
#   JAEGER_UI_DIR=/path/to/jaeger-ui make rebuild-ui
JAEGER_UI_DIR="${JAEGER_UI_DIR:-jaeger-ui}"

cd "${JAEGER_UI_DIR}"

# When JAEGER_UI_SKIP_RELEASE_CHECK is set (e.g. snapshot builds pointing at a
# non-release commit), skip the tag lookup and go straight to the npm build.
# This avoids a costly git fetch --unshallow + fetch --all --tags on a shallow clone.
if [[ -z "${JAEGER_UI_SKIP_RELEASE_CHECK:-}" ]]; then
    if [[ "$(git rev-parse --is-shallow-repository)" == "true" ]]; then
        git fetch --unshallow
    fi
    git fetch --all --tags
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
fi

# do a regular full build
# nvm is only available in local environments; CI uses actions/setup-node instead.
if type nvm &>/dev/null 2>&1; then nvm use; fi
npm ci && cd packages/jaeger-ui && npm run build
