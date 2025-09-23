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
# Using 'sed -i' without '.bak' to avoid creating backup files.
echo "1. Updating version strings..."

sed -i "s/version: .*/version: ${MAIN_VERSION}/g" charts/jaeger/Chart.yaml
sed -i "s/appVersion: .*/appVersion: ${MAIN_VERSION}/g" charts/jaeger/Chart.yaml
sed -i "s/const Version = .*/const Version = \"${MAIN_VERSION}\"/g" pkg/version/version.go
sed -i "s/\"version\": \".*\"/\"version\": \"${UI_VERSION}\"/g" jaeger-ui/package.json

# --- Task 2: Generate Changelog ---
# Calling the python script directly to filter changes and save to a file.
echo "2. Generating changelog..."

# Find the tag of the most recent release to generate a delta changelog.
PREVIOUS_TAG=$(git describe --tags --abbrev=0)
echo "Generating changelog since previous tag: ${PREVIOUS_TAG}"

# Call the python script directly with arguments to save the filtered output.
python3 ./scripts/release/notes.py \
  --start-tag "${PREVIOUS_TAG}" \
  --output CHANGELOG.md

echo "Changelog successfully written to CHANGELOG.md"

# --- Final Instructions ---
echo
echo "--------------------------------------------------"
echo "✅ Release preparation script finished."
echo "Review the file changes with 'git diff'."
echo "Once you are satisfied, commit the changes and update your Pull Request."
echo "--------------------------------------------------"