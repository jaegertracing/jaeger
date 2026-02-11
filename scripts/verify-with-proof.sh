#!/bin/bash
# Copyright (c) 2025 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

# Verify PR with proof for AI policy compliance.
# Uploads test logs to a Gist and adds trailers to the commit.

set -euo pipefail

# Check that gh CLI is available
if ! command -v gh >/dev/null 2>&1; then
    echo "❌ GitHub CLI (gh) not found. Install from: https://cli.github.com/"
    exit 1
fi

# Check that there's a commit to amend
if ! git rev-parse --verify HEAD >/dev/null 2>&1; then
    echo "❌ No commit to amend. Please commit your changes first."
    exit 1
fi

# Check that .test.log exists
if [ ! -f .test.log ]; then
    echo "❌ .test.log not found. Run 'make test-with-log' first."
    exit 1
fi

echo "Uploading proof to Gist..."

TREE_SHA=$(git rev-parse HEAD'^{tree}')
echo "Tree SHA: $TREE_SHA" > .test.log.tmp
echo "---" >> .test.log.tmp
cat .test.log >> .test.log.tmp
mv .test.log.tmp .test.log

GIST_URL=$(gh gist create .test.log -d "Test logs for Jaeger tree $TREE_SHA")
if [ -z "$GIST_URL" ]; then
    echo "❌ Failed to create Gist. Make sure 'gh' CLI is authenticated."
    exit 1
fi

NAME=$(git config user.name)
EMAIL=$(git config user.email)
git commit --amend --no-edit \
    --trailer "Tested-By: $NAME <$EMAIL>" \
    --trailer "Test-Gist: $GIST_URL"

rm -f .test.log
echo "✅ Done. Gist: $GIST_URL"
echo "Now run: git push --force-with-lease"
