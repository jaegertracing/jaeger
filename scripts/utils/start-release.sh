#!/usr/bin/env bash

# Copyright (c) 2025 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0
#Requires bash version to be >=4. Will add alternative for lower versions
set -euo pipefail


if ! current_version_v1=$(make "echo-v1"); then
  echo "Error: Failed to fetch current version from make echo-v1."
  exit 1
fi

# removing the v so that in the line "New version: v1.66.1", v cannot be removed with backspace
clean_version="${current_version_v1#v}" 

IFS='.' read -r major minor patch <<< "$clean_version"

patch=$((patch + 1))
suggested_version="${major}.${minor}.${patch}"
echo "Current v1 version: ${current_version_v1}"
read -e -p "New version: v" -i "${suggested_version}" user_version_v1

if ! current_version_v2=$(make "echo-v2"); then
  echo "Error: Failed to fetch current version from make echo-v2."
  exit 1
fi

# removing the v so that in the line "New version: v1.66.1", v cannot be removed with backspace
clean_version="${current_version_v2#v}" 

IFS='.' read -r major minor patch <<< "$clean_version"

patch=$((patch + 1))
suggested_version="${major}.${minor}.${patch}"
echo "Current v2 version: ${current_version_v2}"
read -e -p "New version: v" -i "${suggested_version}" user_version_v2

new_version="v${user_version_v1} / v${user_version_v2}"
echo "Using new version: ${new_version}"


if [ ! -f "scripts/utils/formatter.go" ]; then
  echo "Error: scripts/utils/formatter.go not found."
  exit 1
fi

wget -O DOC_RELEASE.md https://raw.githubusercontent.com/jaegertracing/documentation/main/RELEASE.md

issue_body=$(go run scripts/utils/formatter.go)

gh issue create --title "Checklist ${new_version}" --body "$issue_body"


if ! current_version_v1=$(make "echo-v2"); then
  echo "Error: Failed to fetch current version from make echo-v2."
  exit 1
fi


rm DOC_RELEASE.md

exit 1;


