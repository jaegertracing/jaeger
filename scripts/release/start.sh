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

This issue tracks the release of Jaeger ${cmd_v1} / ${cmd_v2}.

**Automated option**: Run \`bash ./scripts/release/prepare.sh ${cmd_v1} ${cmd_v2}\` to automatically create the PR with changelog updates.

**Manual option**: Follow the [manual release preparation steps](https://github.com/jaegertracing/jaeger/blob/main/RELEASE.md#manual-release-preparation-steps) in \`RELEASE.md\`.

---

### Tagging

After merging the PR, create signed tags and push them:

```bash
bash ./scripts/release/prepare.sh ${cmd_v1} ${cmd_v2} --create-tags
EOF
)

if $dry_run; then
  printf "%s\n" "${issue_body}"
  exit 0
else
  issue_output=$(gh issue create -R jaegertracing/jaeger --title "Prepare Jaeger Release ${new_version}" --body "$issue_body")
  issue_number=$(echo "$issue_output" | grep -o '#[0-9]*' | head -1)
  echo "Created tracking issue: $issue_output"
  echo ""
  echo "Next step: Run the following command:"
  echo "bash ./scripts/release/prepare.sh ${cmd_v1} ${cmd_v2}"
fi


