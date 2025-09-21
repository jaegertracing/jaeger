#!/bin/bash
set -e # Exit if any command fails

# This script expects two version numbers as arguments
MAIN_VERSION=$1
UI_VERSION=$2

# Check if both version arguments were provided.
if [ -z "$MAIN_VERSION" ] || [ -z "$UI_VERSION" ]; then
  echo "Error: Missing version arguments."
  echo "Usage: ./scripts/release/prepare-release.sh <main-version> <ui-version>"
  echo "Example: ./scripts/release/prepare-release.sh 1.56.0 4.10.0"
  exit 1
fi

echo "--- Preparing release for main version: $MAIN_VERSION and UI version: $UI_VERSION ---"

# --- Task 1: Update Version Strings in the Codebase ---
echo "1. Updating version strings..."

# Update the main Jaeger version in the Makefile
# Note: The 'v' is intentionally left out here as per the file's format.
sed -i.bak "s/JAEGER_VERSION ?= .*/JAEGER_VERSION ?= ${MAIN_VERSION}/g" Makefile

# Update the main Jaeger version in the Go version file
sed -i.bak "s/const Version = \".*\"/const Version = \"${MAIN_VERSION}\"/g" pkg/version/version.go

# Update the Jaeger UI version in its package.json file
sed -i.bak "s/\"version\": \".*\"/\"version\": \"${UI_VERSION}\"/g" jaeger-ui/package.json

# --- Task 2: Generate Changelog ---
echo "2. Generating changelog..."
make changelog

# --- Final Instructions ---
echo
echo "--------------------------------------------------"
echo "✅ Release preparation script finished."
echo "Review the file changes with 'git diff'."
echo "Once you are satisfied, commit the changes and open a Pull Request."
echo "--------------------------------------------------"