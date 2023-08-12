#!/bin/bash

declare -A semconv_map

while IFS=: read -r file_name package_string; do
    version_number=$(echo "$package_string" | grep -o -E "v1\.[0-9]+\.[0-9]+")
    semconv_map["$file_name"]=$version_number
done < <(find . -type f -name "*.go" -exec grep -o -H "go.opentelemetry.io/otel/semconv/v1\.[0-9]\+\.[0-9]\+" {} +)

semconv_versions=($(printf "%s\n" "${semconv_map[@]}" | sort -u))

if [ ${#semconv_versions[@]} -gt 1 ]; then
    echo "Error: semconv version mismatch detected"
    {
        for key in "${!semconv_map[@]}"; do
            printf "Source File: %-30s | Semconv Version: %s\n" "$key" "${semconv_map[$key]}"
        done
    } | column -t -s '|'

    exit 1
fi