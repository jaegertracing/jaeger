#!/bin/bash

latest_semconv_version=$(
    curl -s https://pkg.go.dev/go.opentelemetry.io/otel/semconv \
    | grep -oP 'data-id="v\d+\.\d+\.\d+"' \
    | sed -E 's/data-id="v([0-9]+\.[0-9]+\.[0-9]+)"/v\1/' \
    | sort -Vr \
    | head -n 1
)

latest_package_string="go.opentelemetry.io/otel/semconv/$latest_semconv_version"

while IFS=: read -r file_name package_string; do
    version_number=$(echo "$package_string" | grep -o -E "v1\.[0-9]+\.[0-9]+")

    if [ "$version_number" != "$latest_semconv_version" ]; then
	sed -i "s#go\.opentelemetry\.io\/otel\/semconv\/v[0-9]\+\.[0-9]\+\.[0-9]\+#$latest_package_string#g" "$file_name"
	{
            printf "Source File: %-60s | Previous Semconv Version: %s | Updated Semconv Version: %s\n" "$file_name" "$version_number" "$latest_semconv_version"
        } | column -t -s '|'
    fi
done < <(find . -type f -name "*.go" -exec grep -o -H "go.opentelemetry.io/otel/semconv/v1\.[0-9]\+\.[0-9]\+" {} +)
