#!/usr/bin/env bash

# Copyright (c) 2025 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

#Requires bash version to be >=4. Will add alternative for lower versions
set -euo pipefail

dry_run=false

while getopts "d" opt; do
    case "${opt}" in
        d)
            dry_run=true
            ;;
        *)
            echo "Usage: $0 [-d]"
            exit 1
            ;;
    esac
done
if ! current_version_v1=$(make "echo-v1"); then
  echo "Error: Failed to fetch current version from make echo-v1."
  exit 1
fi

# removing the v so that in the line "New version: v1.66.1", v cannot be removed with backspace
clean_version="${current_version_v1#v}" 

IFS='.' read -r major minor patch <<< "$clean_version"

minor=$((minor + 1))
patch=0
suggested_version="${major}.${minor}.${patch}"
echo "Current v1 version: ${current_version_v1}"
read -r -e -p "New version: v" -i "${suggested_version}" user_version_v1

if ! current_version_v2=$(make "echo-v2"); then
  echo "Error: Failed to fetch current version from make echo-v2."
  exit 1
fi

# removing the v so that in the line "New version: v1.66.1", v cannot be removed with backspace
clean_version="${current_version_v2#v}" 

IFS='.' read -r major minor patch <<< "$clean_version"

minor=$((minor + 1))
patch=0
suggested_version="${major}.${minor}.${patch}"
echo "Current v2 version: ${current_version_v2}"
read -r -e -p "New version: v" -i "${suggested_version}" user_version_v2

new_version="v${user_version_v1} / v${user_version_v2}"
echo "Using new version: ${new_version}"



TMPFILE=$(mktemp "/tmp/DOC_RELEASE.XXXXXX") 
wget -O "$TMPFILE" https://raw.githubusercontent.com/jaegertracing/documentation/main/RELEASE.md

# Ensure the UI Release checklist is up to date.
make init-submodules

issue_body=$(python scripts/release/formatter.py "${TMPFILE}" "${user_version_v1}" "${user_version_v2}")

if $dry_run; then
  echo "${issue_body}"
else
  gh issue create -R jaegertracing/jaeger --title "Prepare Jaeger Release ${new_version}" --body "$issue_body"
fi

rm "${TMPFILE}"

exit 1;


