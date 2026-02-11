#!/bin/bash
# Verify PR trailers for AI policy compliance.
# Called from GitHub Actions workflow.

set -euo pipefail

ASSOCIATION="${1:-}"
COMMIT_MSG=$(git log -1 --pretty=%B)

# Check if trusted contributor
is_trusted() {
    [[ "$ASSOCIATION" == "OWNER" || "$ASSOCIATION" == "MEMBER" || "$ASSOCIATION" == "COLLABORATOR" ]]
}

# Check for Tested-By trailer
if ! echo "$COMMIT_MSG" | grep -q "Tested-By:"; then
    echo "❌ Missing Tested-By trailer."
    exit 1
fi

echo "✅ Found Tested-By trailer"

# Trusted contributors don't need Test-Gist
if is_trusted; then
    echo "✅ Trusted contributor - Test-Gist not required"
    exit 0
fi

# Check for Test-Gist trailer
GIST_URL=$(echo "$COMMIT_MSG" | grep "Test-Gist:" | awk '{print $2}' | head -1)
if [ -z "$GIST_URL" ]; then
    echo "❌ Missing Test-Gist trailer."
    exit 1
fi

echo "Found Test-Gist: $GIST_URL"

# Extract and validate Gist
GIST_ID=$(echo "$GIST_URL" | grep -oE '[0-9a-f]{20,32}' | tail -1)
if [ -z "$GIST_ID" ]; then
    echo "❌ Invalid Gist URL"
    exit 1
fi

GIST_CONTENT=$(curl -s "https://api.github.com/gists/$GIST_ID" | jq -r '.files | to_entries[0].value.content // empty')
if [ -z "$GIST_CONTENT" ]; then
    echo "❌ Could not fetch Gist"
    exit 1
fi

GIST_TREE_SHA=$(echo "$GIST_CONTENT" | grep -oE "Tree SHA: [0-9a-f]{40}" | head -1 | awk '{print $3}')
PR_TREE_SHA=$(git rev-parse HEAD'^{tree}')

if [ -z "$GIST_TREE_SHA" ] || [ "$GIST_TREE_SHA" != "$PR_TREE_SHA" ]; then
    echo "❌ Tree SHA mismatch. Re-run 'make verify-with-proof'."
    exit 1
fi

echo "✅ Verification passed"
