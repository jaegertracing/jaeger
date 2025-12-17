#!/usr/bin/env bash

# Copyright (c) 2025 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

for tool in gh git python3; do
    if ! command -v "$tool" &> /dev/null; then
        echo "Error: $tool is not installed or not in PATH"
        exit 1
    fi
done

if [ "$#" -ne 1 ]; then
    echo "Usage: $0 <version>"
    echo "Example: $0 2.14.0"
    exit 1
fi

VERSION="$1"
VERSION="${VERSION#v}"

echo "Preparing release for v${VERSION}"

CURRENT_BRANCH=$(git rev-parse --abbrev-ref HEAD)
if [ "$CURRENT_BRANCH" != "main" ]; then
    echo "Warning: Not on main branch (current: ${CURRENT_BRANCH})"
    read -p "Continue anyway? (y/n) " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        exit 1
    fi
fi

git fetch origin

BRANCH_NAME="prepare-release-v${VERSION}"
git checkout -b "${BRANCH_NAME}"

echo "Updating UI submodule..."
git submodule init
git submodule update

UI_VERSION="v${VERSION}"
if [ -d "jaeger-ui" ] && [ "$(ls -A jaeger-ui)" ]; then
    pushd jaeger-ui > /dev/null
    git fetch origin
    if git rev-parse "${UI_VERSION}" >/dev/null 2>&1; then
        git checkout "${UI_VERSION}"
    else
        echo "Warning: UI version ${UI_VERSION} not found"
        read -r -p "Enter UI version to use (or Enter to skip): " UI_INPUT
        if [ -n "$UI_INPUT" ]; then
            git checkout "$UI_INPUT"
        else
            git checkout main && git pull
        fi
    fi
    popd > /dev/null
    git add jaeger-ui
fi

echo "Updating CHANGELOG.md..."
RELEASE_DATE=$(date +%Y-%m-%d)
CHANGELOG_CONTENT=$(python3 scripts/release/notes.py --exclude-dependabot 2>/dev/null || echo "")

python3 - <<EOF "$VERSION" "$RELEASE_DATE" "$CHANGELOG_CONTENT"
import sys

version = sys.argv[1]
release_date = sys.argv[2]
changelog_content = sys.argv[3] if len(sys.argv) > 3 else ""

with open('CHANGELOG.md', 'r') as f:
    lines = f.readlines()

# Find the template section end
template_end = -1
for i, line in enumerate(lines):
    if '</details>' in line:
        template_end = i + 1
        break

if template_end == -1:
    print("Error: Could not find template end marker")
    sys.exit(1)

# Create the new changelog section
new_section = []
new_section.append(f"\nv{version} ({release_date})\n")
new_section.append("-" * 31 + "\n")
new_section.append("\n")

if changelog_content:
    new_section.append(changelog_content)
    if not changelog_content.endswith('\n'):
        new_section.append("\n")
else:
    new_section.append("### Backend Changes\n\n")
    new_section.append("run \`make changelog\` to generate content\n\n")
    new_section.append("### UI Changes\n\n")
    new_section.append("copy from UI changelog\n\n")

# Write the updated CHANGELOG.md
with open('CHANGELOG.md', 'w') as f:
    f.writelines(lines[:template_end])
    f.writelines(new_section)
    f.writelines(lines[template_end:])

print(f"Updated CHANGELOG.md with v{version}")
EOF

git add CHANGELOG.md

echo "Rotating release managers table..."
python3 - <<EOF
import re

with open('RELEASE.md', 'r') as f:
    content = f.read()

# Find the release managers table
table_pattern = r'(\|\| Version \| Release Manager \| Tentative release date \|\n\|\|---------|-----------------|------------------------\|(?:\n\|\|[^\n]+\|)*)'
match = re.search(table_pattern, content)

if match:
    table = match.group(0)
    lines = table.strip().split('\n')
    
    # Skip header and separator
    data_lines = lines[2:]
    
    if data_lines:
        # Move first line to the end
        rotated = data_lines[1:] + [data_lines[0]]
        
        # Reconstruct table
        new_table = '\n'.join(lines[:2] + rotated)
        
        # Replace in content
        content = content[:match.start()] + new_table + content[match.end():]
        
        with open('RELEASE.md', 'w') as f:
            f.write(content)
        
        print("Rotated release managers table")
EOF

git diff --quiet RELEASE.md || git add RELEASE.md

git commit -m "Prepare release v${VERSION}

- Updated CHANGELOG.md with release notes
- Updated jaeger-ui submodule
- Rotated release managers table"

git push origin "${BRANCH_NAME}"

PR_BODY="This PR prepares the release for v${VERSION}.

## Changes
- [x] Updated CHANGELOG.md with release notes
- [x] Updated jaeger-ui submodule to v${VERSION}
- [x] Rotated release managers table in RELEASE.md

## After this PR is merged
Run the following commands to create and push the release tag:

\`\`\`bash
git checkout main
git pull
git tag v${VERSION} -s -m \"Release v${VERSION}\"
git push upstream v${VERSION}
\`\`\`

Then create the release on GitHub:

\`\`\`bash
make draft-release
\`\`\`

Trigger the [Publish Release](https://github.com/jaegertracing/jaeger/actions/workflows/ci-release.yml) workflow on GitHub."

gh pr create \
    --title "Prepare release v${VERSION}" \
    --body "$PR_BODY" \
    --label "changelog:skip" \
    --base main

echo "Done. Review and merge the PR, then follow the instructions in the PR description."
