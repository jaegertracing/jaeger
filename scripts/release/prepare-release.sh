#!/bin/bash
set -e # Exit immediately if a command exits with a non-zero status.

# This script expects two arguments: the main Jaeger version and the UI version.
MAIN_VERSION=$1
UI_VERSION=$2

# Check if both version arguments were provided.
if [ -z "$MAIN_VERSION" ] || [ -z "$UI_VERSION" ]; then
  echo "Error: Missing version arguments."
  echo "Usage: ./scripts/release/prepare-release.sh <main-version> <ui-version>"
  echo "Example: ./scripts/release/prepare-release.sh v1.56.0 v4.10.0"
  exit 1
fi

echo "--- Preparing release for main version: $MAIN_VERSION and UI version: $UI_VERSION ---"

# --- Task 1: Update Version Strings in the Codebase ---
# You must investigate the codebase to find the correct files and patterns to replace.
# Use 'grep' to find where the old version numbers are located.
# The 'sed' command is used to replace them. The '-i.bak' flag creates a backup.
echo "1. Updating version strings..."

# EXAMPLE ONLY - Replace with the actual files and patterns you find.
# sed -i.bak "s/JAEGER_VERSION := .*/JAEGER_VERSION := ${MAIN_VERSION}/g" Makefile
# sed -i.bak "s|github.com/jaegertracing/jaeger-ui v.*|github.com/jaegertracing/jaeger-ui ${UI_VERSION}|g" go.mod


# --- Task 2: Generate Changelog ---
echo "2. Generating changelog..."
# This command updates the CHANGELOG.md file.
make changelog


# --- Final Instructions ---
echo
echo "--------------------------------------------------"
echo "✅ Release preparation script finished."
echo "Please review the file changes with 'git diff'."
echo "Once you are satisfied, commit the changes and open a Pull Request."
echo "--------------------------------------------------"