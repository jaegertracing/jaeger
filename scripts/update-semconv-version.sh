#!/bin/bash

package_name="go.opentelemetry.io/otel/semconv"
version_regex="v[0-9]\.[0-9]\+\.[0-9]\+"

latest_semconv_version=$(
    curl -s https://pkg.go.dev/$package_name \
    | grep -oP 'data-id="v\d+\.\d+\.\d+"' \
    | sed -E "s/\"($version_regex)\"/v\1/" \
    | sort -Vr \
    | head -n 1 \
    | awk -F'"' '{print $2}'
)

latest_package_string="$package_name/$latest_semconv_version"

while IFS=: read -r file_name package_string; do
    version_number=${package_string##*/}

    if [ "$version_number" != "$latest_semconv_version" ]; then
	sed -i "s#$package_name/$version_regex#$latest_package_string#g" "$file_name"
	{
            printf "Source File: %-60s | Previous Semconv Version: %s | Updated Semconv Version: %s\n" "$file_name" "$version_number" "$latest_semconv_version"
        } | column -t -s '|'
    fi
done < <(find . -type f -name "*.go" -exec grep -o -H "$package_name/$version_regex" {} +)
