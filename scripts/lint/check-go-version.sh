#!/bin/bash

# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

version_regex='[0-9]\.[0-9][0-9]'
update=false
verbose=false

while getopts "uvdx" opt; do
    case $opt in
        u) update=true ;;
        v) verbose=true ;;
        x) set -x ;;
        *) echo "Usage: $0 [-u] [-v] [-d]" >&2
           exit 1
           ;;
    esac
done

# Fetch latest go release version
# go_latest_version=$(curl -s https://go.dev/dl/?mode=json | jq -r '.[0].version' | awk -F'.' '{gsub("go", ""); print $1"."$2}')
#
# UPDATE: we don't use the logic above because it causes CI to fail when new version of Go is released,
# which may create circular dependencies when other utilities need to be upgraded. Instead use the go
# version declared in the main go.mod. Updates to that version will be handled by the bots.
go_latest_version=$(grep "^go " go.mod | sed 's/^go \([0-9]\.[0-9]*\).*/\1/')

files_to_update=0

function update() {
    local file=$1
    local pattern=$2
    local current=$3
    local target=$4

    newfile=$(mktemp)
    old_IFS=$IFS
    IFS=''
    while read -r line; do
        match=$(echo "$line" | grep -e "$pattern")
        if [[ "$match" != "" ]]; then
            line=${line//${current}/${target}}
        fi
        echo "$line" >> "$newfile"
    done < "$file"
    IFS=$old_IFS

    if [ $verbose = true ]; then
        diff "$file" "$newfile"
    fi

    mv "$newfile" "$file"
}

function check() {
    local file=$1
    local pattern=$2
    local target=$3

    go_version=$(grep -e "$pattern" "$file" | head -1 | sed "s/^.*\($version_regex\).*$/\1/")

    if [ "$go_version" = "$target" ]; then
        mismatch=''
    else
        mismatch="*** needs update to $target ***"
        files_to_update=$((files_to_update+1))
    fi

    if [[ $update = true && "$mismatch" != "" ]]; then
        # Detect if the line includes a patch version
        if [[ "$go_version" =~ $version_regex\.[0-9]+ ]]; then
            echo "Patch version detected in $file. Manual update required."
            exit 1
        fi
        update "$file" "$pattern" "$go_version" "$target"
        mismatch="*** => $target ***"
    fi

    printf "%-50s Go version: %s %s\n" "$file" "$go_version" "$mismatch"
}

# In the main go.mod file (and linter config) we want the same Go version N.
# All importable code has been moved to internal packages, so there's no need
# to maintain backward compatibility with older compilers.
check go.mod "^go\s\+$version_regex" "$go_latest_version"
check .golangci.yml "go:\s\+\"$version_regex\"" "$go_latest_version"

# find all other go.mod files in the repository and check for latest Go version
for file in $(find . -type f -name go.mod | grep -v '^./go.mod'); do
    if [[ $file == "./idl/go.mod" ]]; then
        continue
    fi
    if [[ $file == "./idl/internal/tools/go.mod" ]]; then
        continue
    fi
    check "$file" "^go\s\+$version_regex" "$go_latest_version"
done

IFS='|' read -r -a gha_workflows <<< "$(grep -rl go-version .github/workflows | tr '\n' '|')"
for gha_workflow in "${gha_workflows[@]}"; do
    check "$gha_workflow" "^\s*go-version:\s\+$version_regex" "$go_latest_version"
done

if [ $files_to_update -eq 0 ]; then
    echo "All files are up to date."
else
    if [[ $update = true ]]; then
        echo "$files_to_update file(s) updated."
    else
        echo "$files_to_update file(s) must be updated. Rerun this script with -u argument."
        exit 1
    fi
fi
