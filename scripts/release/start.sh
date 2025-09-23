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



cmd_v1="v${user_version_v1}"
cmd_v2="v${user_version_v2}"

issue_body=$(cat << EOF
## Prepare Jaeger Release ${cmd_v1} / ${cmd_v2}

This is the tracking issue for preparing the Jaeger release.

Next step (automated):
Run: \`bash ./scripts/release/prepare-release.sh ${cmd_v1} ${cmd_v2}\`

After merging the PR, create signed tags and push:
\`\`\`
git checkout main
git pull --ff-only upstream main
git tag ${cmd_v1} -s -m "${cmd_v1}"
git tag ${cmd_v2} -s -m "${cmd_v2}"
git push upstream ${cmd_v1} ${cmd_v2}
\`\`\`

References: #7496
EOF
)

if $dry_run; then
  printf "%s\n" "${issue_body}"
  exit 0
else
  gh issue create -R jaegertracing/jaeger --title "Prepare Jaeger Release ${new_version}" --body "$issue_body"
fi


