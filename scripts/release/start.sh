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
if $dry_run; then
  echo "DRY RUN: Skipping download of RELEASE.md. Using minimal fallback template."
  cat > "$TMPFILE" << 'EOF'
<!-- BEGIN_CHECKLIST -->
* Update documentation for the release
* Verify release artifacts
<!-- END_CHECKLIST -->
EOF
else
  wget -O "$TMPFILE" https://raw.githubusercontent.com/jaegertracing/documentation/main/RELEASE.md
fi

# Ensure the UI Release checklist is up to date.
if $dry_run; then
  echo "DRY RUN: Skipping submodule init/update"
else
  make init-submodules
fi

# Extract sections between markers from UI, Backend, and Docs READMEs
START_MARKER='<!-- BEGIN_CHECKLIST -->'
END_MARKER='<!-- END_CHECKLIST -->'

# Read UI checklist from submodule
if [[ -f "jaeger-ui/RELEASE.md" ]]; then
  ui_section=$(awk "/${START_MARKER}/{flag=1;next}/$END_MARKER/{flag=0}flag" jaeger-ui/RELEASE.md)
else
  ui_section=""
fi

# Read Backend checklist from local RELEASE.md
backend_section=$(awk "/${START_MARKER}/{flag=1;next}/$END_MARKER/{flag=0}flag" RELEASE.md)

# Read Docs checklist from downloaded template
doc_section=$(awk "/${START_MARKER}/{flag=1;next}/$END_MARKER/{flag=0}flag" "$TMPFILE")

# Substitute versions (v1 placeholders like 1.x.x/X.Y.Z, and v2 placeholders 2.x.x)
v1_replacement="${user_version_v1}"
v2_replacement="${user_version_v2}"
ui_section=$(echo "$ui_section" | sed -E "s/(X\\.Y\\.Z|1\\.[0-9]+\\.[0-9]+|1\\.x\\.x)/$v1_replacement/g" | sed -E "s/2\\.x\\.x/$v2_replacement/g")
backend_section=$(echo "$backend_section" | sed -E "s/(X\\.Y\\.Z|1\\.[0-9]+\\.[0-9]+|1\\.x\\.x)/$v1_replacement/g" | sed -E "s/2\\.x\\.x/$v2_replacement/g")
doc_section=$(echo "$doc_section" | sed -E "s/(X\\.Y\\.Z|1\\.[0-9]+\\.[0-9]+|1\\.x\\.x)/$v1_replacement/g" | sed -E "s/2\\.x\\.x/$v2_replacement/g")

# Build issue body
issue_body="# UI Release
$ui_section
# Backend Release
$backend_section
# Doc Release
$doc_section"

# Append automated command with concrete versions
cmd_v1="v${user_version_v1}"
cmd_v2="v${user_version_v2}"
issue_body+=$'\n\n**Automated option**: Run `bash ./scripts/release/prepare-release.sh '
issue_body+="$cmd_v1"
issue_body+=$' '
issue_body+="$cmd_v2"
issue_body+=$'` to automatically create the PR with changelog updates. Add `--auto-tag` to create and push tags after merging the PR.'

if $dry_run; then
  echo "${issue_body}"
  rm -f "${TMPFILE}"
  exit 0
else
  gh issue create -R jaegertracing/jaeger --title "Prepare Jaeger Release ${new_version}" --body "$issue_body"
fi

rm -f "${TMPFILE}"


