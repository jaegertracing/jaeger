#!/bin/bash

set -euxf -o pipefail

cd jaeger-ui

LAST_TAG=$(git describe --abbrev=0 --tags 2>/dev/null)
BRANCH_HASH=$(git rev-parse HEAD)
LAST_TAG_HASH=$(git rev-parse $LAST_TAG)

if [[ "$BRANCH_HASH" == "$LAST_TAG_HASH" ]]; then
    
    if [[ "$LAST_TAG" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]];  then
        temp_file=$(mktemp)
        trap "rm -f ${temp_file}" EXIT
        release_url="https://github.com/jaegertracing/jaeger-ui/releases/download/${LAST_TAG}/assets.tar.gz"
        if curl --silent --fail --location --output "$temp_file" "$release_url"; then

            mkdir -p packages/jaeger-ui/build/
            rm -r -f packages/jaeger-ui/build/
            tar -zxvf "$temp_file" packages/jaeger-ui/build/
            exit 0
            
        fi
    fi

fi

# do a regular full build
yarn install --frozen-lockfile && cd packages/jaeger-ui && yarn build





