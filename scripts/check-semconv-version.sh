#!/bin/bash

declare -A semconv_map

package_name="go.opentelemetry.io/otel/semconv"
version_regex="v[0-9]\.[0-9]\+\.[0-9]\+"

while IFS=: read -r file_name package_string; do
    semconv_map["$file_name"]="${package_string##*/}"
done < <(find . -type f -name "*.go" -exec grep -o -H "$package_name/$version_regex" {} +)

semconv_versions=($(printf "%s\n" "${semconv_map[@]}" | sort -u))

if [ ${#semconv_versions[@]} -gt 1 ]; then
    echo "Error: semconv version mismatch detected."
    {
        for key in "${!semconv_map[@]}"; do
            printf "Source File: %-50s | Semconv Version: %s\n" "$key" "${semconv_map[$key]}"
        done
    } | column -t -s '|'
    echo "Run ./scripts/update-semconv-version.sh to update semconv to latest version."
    exit 1
fi
