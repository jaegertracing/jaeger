#!/bin/bash

# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

set -euf -o pipefail

package_name="go.opentelemetry.io/otel/semconv"
version_regex="v[0-9]\.[0-9]\+\.[0-9]\+"

function find_files() {
    find . -type f -name "*.go" -exec grep -o -H "$package_name/$version_regex" {} + \
    | tr ':' ' ' \
    | sed "s|$package_name/||g"
}
count=$(find_files | awk '{print $2}' | sort -u | wc -l)

if [ "$count" -gt 1 ]; then
    printf "%-70s | %s\n" "Source File" "Semconv Version"
    printf "%-70s | %s\n" "================" "================"
    while IFS=' ' read -r file_name version; do
        printf "%-70s | %s\n" "$file_name" "$version"
    done < <(find_files)
    printf "Error: %d different semconv versions detected.\n" "$count"
    echo "Run ./scripts/update-semconv-version.sh to update semconv to latest version."
    exit 1
fi
