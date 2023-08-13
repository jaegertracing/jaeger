#!/bin/bash

declare -A go_version_map
version_regex="[0-9]\.[0-9]"

# Fetch latest go release version
go_latest_version=$(curl -s https://go.dev/dl/?mode=json | jq -r '.[0].version' | awk -F'.' '{gsub("go", ""); print $1"."$2}')

# Extract go version from go.mod
go_mod_path="./go.mod"
go_mod_version=$(grep -oP "go\s+\K$version_regex+" $go_mod_path)

# Ensure go.mod uses go_latest_version - 1
if [ "${go_mod_version%.*}" != "${go_latest_version%.*}" ] || [ $((10#${go_latest_version#*.} - 10#${go_mod_version#*.})) != 1 ]; then
    echo "Error: go version mismatch detected in go.mod."
    {
	printf "Source file: %-50s | Go version: %s | Latest Version: %s\n" "./go.mod" "$go_mod_version" "$go_latest_version"
    } | column -t -s '|'
    echo "Run ./scripts/update-go-version.sh to update go.mod to correct go version."
    exit 1
fi

# Extract go version from GitHub actions scripts
while IFS=: read -r build_script version_string; do
    go_version_map["$build_script"]="${version_string##*: }"
done < <(find . -type f -name "*.yml" -exec grep -o -H "go-version: $version_regex\+" {} +)

# Extract go version from Alpine Image makefile
alpine_makefile_path="./docker/Makefile"
go_alpine_version=$(grep -oP "golang:$version_regex+" "$alpine_makefile_path")
go_version_map["$alpine_makefile_path"]="${go_alpine_version##*:}"

go_versions=($(printf "%s\n" "${go_version_map[@]}" | sort -u))

# Ensure all build scripts use the same Go version {go_latest_version}
if [ ${#go_versions[@]} -gt 1 ] || [ "${go_versions[0]}" != "$go_latest_version" ]; then
   echo "Error: go version mismatch detected in build scripts."
   {
       for key in "${!go_version_map[@]}"; do
           printf "Build script: %-50s | Go Version: %s | Latest Version: %s\n" "$key" "${go_version_map[$key]}" "$go_latest_version"
       done
   } | column -t -s '|'
   echo "Run ./scripts/update-go-version.sh to update build scripts to latest go version."
   exit 1
fi