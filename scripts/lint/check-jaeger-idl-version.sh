#!/usr/bin/env bash
set -euo pipefail

# Fetch the version from go.mod, stripping any '+incompatible' suffix
GO_MOD_VERSION=$(grep 'github.com/jaegertracing/jaeger-idl' go.mod | awk '{print $2}' | sed 's/+incompatible//')

if [ -z "$GO_MOD_VERSION" ]; then
    echo "Error: jaeger-idl version not found in go.mod"
    exit 1
fi

# Check submodule directory exists
if [ ! -d "idl" ]; then
    echo "Error: 'idl' submodule directory not found. Initialize submodules first."
    exit 1
fi

cd idl

# Ensure tags are fetched to find the latest semver tag
git fetch --tags --quiet

# Get the latest semver tag at the current submodule commit
SUBMODULE_TAG=$(git tag --points-at HEAD | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' | sort --version-sort | tail -n1)

if [ -z "$SUBMODULE_TAG" ]; then
    echo "No semver tag found at HEAD. Attempting to find the latest tag..."

    # Find the latest tag and check it out
    LATEST_TAG=$(git tag | sort --version-sort | tail -n1)

    if [ -n "$LATEST_TAG" ]; then
        echo "Checking out latest tag: $LATEST_TAG"
        git checkout "$LATEST_TAG"
        SUBMODULE_TAG="$LATEST_TAG"
    else
        echo "Error: No valid semver tag found for jaeger-idl submodule"
        exit 1
    fi
fi

cd ..

# Compare versions
if [ "$GO_MOD_VERSION" != "$SUBMODULE_TAG" ]; then
    echo "Error: Version mismatch between go.mod ($GO_MOD_VERSION) and jaeger-idl submodule ($SUBMODULE_TAG)"
    exit 1
fi

echo "Versions are in sync: go.mod ($GO_MOD_VERSION) and submodule ($SUBMODULE_TAG)"
exit 0