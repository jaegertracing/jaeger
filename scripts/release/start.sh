#!/usr/bin/env bash

# Copyright (c) 2025 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

# Requires bash version to be >=4. Will add alternative for lower versions
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

# Fetch v2 version first (primary focus)
if ! current_version_v2=$(make "echo-v2"); then
  echo "Error: Failed to fetch current v2 version from make echo-v2."
  exit 1
fi

# TODO: Remove v1 support after last scheduled v1 release (early 2026)
if ! current_version_v1=$(make "echo-v1"); then
  echo "Warning: Failed to fetch current v1 version from make echo-v1."
fi

echo "Preparing Jaeger release"
echo "v2 version (recommended): ${current_version_v2}"
echo "v1 version (legacy, 3 releases remaining): ${current_version_v1:-N/A}"

# Calculate suggested v2 version
clean_version_v2="${current_version_v2#v}"
IFS='.' read -r major_v2 minor_v2 patch_v2 <<< "$clean_version_v2"
minor_v2=$((minor_v2 + 1))
patch_v2=0
suggested_v2_version="${major_v2}.${minor_v2}.${patch_v2}"

# Calculate suggested v1 version if available
if [[ -n "${current_version_v1:-}" ]]; then
    clean_version_v1="${current_version_v1#v}"
    IFS='.' read -r major_v1 minor_v1 patch_v1 <<< "$clean_version_v1"
    minor_v1=$((minor_v1 + 1))
    patch_v1=0
    suggested_v1_version="${major_v1}.${minor_v1}.${patch_v1}"
fi

# Prompt for v2 version first (primary)
read -r -e -p "New v2 version: v" -i "${suggested_v2_version}" user_version_v2

# Prompt for v1 version (optional)
if [[ -n "${current_version_v1:-}" ]]; then
    read -r -e -p "New v1 version (leave blank to skip): v" -i "${suggested_v1_version:-}" user_version_v1 || true
fi

# Format version display
new_version="v${user_version_v2}"
if [[ -n "${user_version_v1:-}" ]]; then
    new_version+=" / v${user_version_v1}"
fi

echo "Using new version(s): ${new_version}"

TMPFILE=$(mktemp "/tmp/DOC_RELEASE.XXXXXX")
wget -O "$TMPFILE" https://raw.githubusercontent.com/jaegertracing/documentation/main/RELEASE.md

make init-submodules

# Pass versions to formatter
if [[ -n "${user_version_v1:-}" ]]; then
    issue_body=$(python3 scripts/release/formatter.py "$TMPFILE" "$user_version_v1" "$user_version_v2")
else
    issue_body=$(python3 scripts/release/formatter.py "$TMPFILE" "" "$user_version_v2")
fi

if $dry_run; then
  echo "${issue_body}"
else
  gh issue create -R jaegertracing/jaeger --title "Prepare Jaeger Release ${new_version}" --body "$issue_body"
fi

rm "$TMPFILE"

exit 0
