#!/usr/bin/env bash

# Copyright (c) 2025 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

#Requires bash version to be >=4. Will add alternative for lower versions
set -euo pipefail

dry_run=false

while getopts "dh" opt; do
    case "${opt}" in
        d)
            dry_run=true
            ;;
        h)
            echo "Usage: $0 [-d]"
            exit 0
            ;;
        *)
            echo "Usage: $0 [-d]"
            exit 1
            ;;
    esac
done
if ! current_version=$(make "echo-version"); then
  echo "Error: Failed to fetch current version from make echo-version."
  exit 1
fi

# removing the v so that in the line "New version: v2.13.0", v cannot be removed with backspace
clean_version="${current_version#v}" 

IFS='.' read -r major minor patch <<< "$clean_version"

minor=$((minor + 1))
patch=0
suggested_version="${major}.${minor}.${patch}"
echo "Current version: ${current_version}"
read -r -e -p "New version: v" -i "${suggested_version}" user_version

new_version="v${user_version}"
echo "Using new version: ${new_version}"



DOCS_TEMPLATE=$(mktemp "/tmp/DOC_RELEASE.XXXXXX")
wget -O "$DOCS_TEMPLATE" https://raw.githubusercontent.com/jaegertracing/documentation/main/RELEASE.md

UI_TMPFILE=$(mktemp "/tmp/UI_RELEASE.XXXXXX")
wget -O "$UI_TMPFILE" https://raw.githubusercontent.com/jaegertracing/jaeger-ui/main/RELEASE.md

issue_body=$(python3 scripts/release/formatter.py "${user_version}" "${UI_TMPFILE}" "${DOCS_TEMPLATE}")

if $dry_run; then
  echo "${issue_body}"
else
  gh issue create -R jaegertracing/jaeger --title "Prepare Jaeger Release ${new_version}" --body "$issue_body"
fi

rm "${DOCS_TEMPLATE}" "${UI_TMPFILE}"
