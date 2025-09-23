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
# These are the commands suggested by the review bot and our investigation.
echo "1. Updating version strings..."

sed -i.bak "s/version: .*/version: ${MAIN_VERSION}/g" charts/jaeger/Chart.yaml
sed -i.bak "s/appVersion: .*/appVersion: ${MAIN_VERSION}/g" charts/jaeger/Chart.yaml
sed -i.bak "s/const Version = .*/const Version = \"${MAIN_VERSION}\"/g" pkg/version/version.go
# This is for the UI version, as per the "two-version system" requirement.
sed -i.bak "s/\"version\": \".*\"/\"version\": \"${UI_VERSION}\"/g" jaeger-ui/package.json

# --- Task 2: Generate Changelog ---
echo "2. Generating changelog..."
make changelog

# --- Final Instructions ---
echo
echo "--------------------------------------------------"
echo "✅ Release preparation script finished."
echo "Review the file changes with 'git diff'."
echo "Once you are satisfied, commit the changes and update your Pull Request."
echo "--------------------------------------------------"