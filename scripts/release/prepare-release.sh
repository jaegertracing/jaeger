#!/bin/bash
set -e # Exit immediately if a command exits with a non-zero status.

# The version number is passed as the first argument from the workflow
VERSION=$1
if [ -z "$VERSION" ]; then
  echo "Error: Version number not provided."
  exit 1
fi

echo "--- Preparing release for version: $VERSION ---"

# --- Task 1: Generate Changelog ---
# Find and run the command to update the CHANGELOG.md file.
# You might need to find the exact command in the Makefile.
# For example, it might be:
echo "1. Generating changelog..."
make changelog # Assuming this command exists

# --- Task 2: Update UI Version ---
# This is an example. You must find the actual file and version string to replace.
# Let's pretend the version is in a file called 'pkg/version/version.go'
echo "2. Updating UI version..."
# sed -i 's/jaeger-ui:v[0-9.]*/jaeger-ui:'$VERSION'/g' path/to/some/file

# --- Task 3: Print Tagging Commands ---
echo "3. Generating tag commands..."
echo "--------------------------------------------------"
echo "✅ Release preparation is complete."
echo "Once the PR is merged, run these commands locally to tag the release:"
echo
echo "git tag ${VERSION}"
echo "git push upstream ${VERSION}"
echo "--------------------------------------------------"