#!/bin/bash

declare -A go_version_map
version_regex="[0-9]\.[0-9]"

# Fetch latest go release version
go_latest_version=$(curl -s https://go.dev/dl/?mode=json | jq -r '.[0].version' | awk -F'.' '{gsub("go", ""); print $1"."$2}')

# Set go.mod go version as latest version - 1 (N-1)
go_mod_path="./go.mod"
go_mod_version=$(grep -oP "go\s+\K$version_regex+" $go_mod_path)
go_mod_updated="${go_latest_version%.*}.$((10#${go_latest_version#*.} - 1))"

if [ "$go_mod_version" != "$go_mod_updated" ]; then
    sed -i "s/$go_mod_version/$go_mod_updated/g" "$go_mod_path"
    {
        printf "Go.mod file: %-60s | Previous Go Version: %s | Updated Go Version: %s\n" "$go_mod_path" "${go_mod_version}" "${go_mod_updated}"
    } | column -t -s '|'
fi

# Set latest go version for all GitHub actions scripts
while IFS=: read -r build_script version_string; do

    if [ "${version_string##*: }" != "$go_latest_version" ]; then
        updated_version="${version_string%%:*}: ${go_latest_version}"
        sed -i "s/$version_string/$updated_version/g" "$build_script"
        {
            printf "Build script: %-60s | Previous Go Version: %s | Updated Go Version: %s\n" "$build_script" "${version_string##*: }" "${updated_version##*: }"
        } | column -t -s '|'
    fi
done < <(find . -type f -name "*.yml" -exec grep -o -H "go-version: $version_regex\+" {} +)

# Set latest go version for Alpine Image makefile 
alpine_makefile_path="./docker/Makefile"
go_alpine_version=$(grep -oP "golang:$version_regex+" "$alpine_makefile_path")

if [ "${go_alpine_version##*:}" != "$go_latest_version" ]; then
    sed -i "s/${go_alpine_version}/golang:${go_latest_version}/" "$alpine_makefile_path"
    {
        printf "Build script: %-60s | Previous Go Version: %s | Updated Go Version: %s\n" "$alpine_makefile_path" "${go_alpine_version##*:}" "${go_latest_version}"
    } | column -t -s '|'
fi
