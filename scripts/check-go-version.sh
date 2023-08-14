#!/bin/bash

version_regex="[0-9]\.[0-9]"
update=false

while getopts "u" opt; do
    case $opt in
        u) update=true ;;
        *) echo "Usage: $0 [-u]" >&2
           exit 1
           ;;
    esac
done

# Fetch latest go release version
go_latest_version=$(curl -s https://go.dev/dl/?mode=json | jq -r '.[0].version' | awk -F'.' '{gsub("go", ""); print $1"."$2}')

# Extract go version from go.mod
go_mod_path="./go.mod"
go_mod_version=$(grep -oP "go\s+\K$version_regex+" $go_mod_path)

# If update, set go.mod version as latest version - 1 (N-1)
if [ "$update" = true ]; then
    go_mod_updated="${go_latest_version%.*}.$((10#${go_latest_version#*.} - 1))"
    if [ "$go_mod_version" != "$go_mod_updated" ]; then
        sed -i "s/$go_mod_version/$go_mod_updated/g" "$go_mod_path"
        {
            printf "Go.mod file: %-60s | Previous Go Version: %s | Updated Go Version: %s\n" "$go_mod_path" "${go_mod_version}" "${go_mod_updated}"
        } | column -t -s '|'
    fi
fi

# Extract go version from GitHub actions scripts
while IFS=: read -r build_script version_string; do
    
    # If update, set latest go version for all GitHub actions scripts
    if [ "$update" = true ]; then
        if [ "${version_string##*: }" != "$go_latest_version" ]; then
            updated_version="${version_string%%:*}: ${go_latest_version}"
            sed -i "s/$version_string/$updated_version/g" "$build_script"
            {
                printf "Build script: %-60s | Previous Go Version: %s | Updated Go Version: %s\n" "$build_script" "${version_string##*: }" "${updated_version##*: }"
            } | column -t -s '|'
        fi
    else
        declare -A go_version_map
        go_version_map["$build_script"]="${version_string##*: }"
    fi
done < <(find . -type f -name "*.yml" -exec grep -o -H "go-version: $version_regex\+" {} +)

# Extract go version from Alpine Image makefile
alpine_makefile_path="./docker/Makefile"
go_alpine_version=$(grep -oP "golang:$version_regex+" "$alpine_makefile_path")

# If update, set latest go version for Alpine Image makefile 
if [ "$update" = true ]; then
    if [ "${go_alpine_version##*:}" != "$go_latest_version" ]; then
        sed -i "s/${go_alpine_version}/golang:${go_latest_version}/" "$alpine_makefile_path"
        {
            printf "Build script: %-60s | Previous Go Version: %s | Updated Go Version: %s\n" "$alpine_makefile_path" "${go_alpine_version##*:}" "${go_latest_version}"
        } | column -t -s '|'
    fi
else
    go_version_map["$alpine_makefile_path"]="${go_alpine_version##*:}"
fi

# Ensure N-1 support policy
if [ "$update" != true ]; then
    go_versions=($(printf "%s\n" "${go_version_map[@]}" | sort -u))

    # Ensure all build scripts use the same Go version {go_latest_version}
    if [ ${#go_versions[@]} -gt 1 ]; then
    echo "Error: go version mismatch detected in build scripts."
    {
        for key in "${!go_version_map[@]}"; do
            printf "Build script: %-50s | Go Version: %s | Go.mod Version: %s\n" "$key" "${go_version_map[$key]}" "$go_mod_version"
        done
    } | column -t -s '|'
    echo "Run ./scripts/update-go-version.sh -u to update build scripts to latest go version."
    exit 1
    fi

    # Ensure go.mod uses go version one release behind build script
    if [ "${go_mod_version%.*}" != "${go_versions%.*}" ] || [ $((10#${go_versions#*.} - 10#${go_mod_version#*.})) != 1 ]; then
        echo "Error: go.mod must be one release behind build scripts."
        {
        printf "Source file: %-50s | Go.mod version: %s | Build scripts version: %s\n" "./go.mod" "$go_mod_version" "$go_versions"
        } | column -t -s '|'
        echo "Run ./scripts/update-go-version.sh -u to update go.mod to correct go version."
        exit 1
    fi

    {
        printf "Go.mod version: %-10s | Build scripts version: %-10s | Latest version: %-10s\n" "$go_mod_version" "$go_versions" "$go_latest_version"
    } | column -t -s '|'
fi