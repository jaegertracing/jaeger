#!/bin/bash
set -e # Exit if any command fails

MAIN_VERSION=$1
UI_VERSION=$2

if [ -z "$MAIN_VERSION" ] || [ -z "$UI_VERSION" ]; then
  echo "Error: Missing version arguments."
  echo "Usage: ./scripts/release/prepare-release.sh <main-version> <ui-version>"
  echo "Example: ./scripts/release/prepare-release.sh 1.56.0 4.10.0"
  exit 1
fi

echo "--- Preparing release for main version: $MAIN_VERSION and UI version: $UI_VERSION ---"

# --- Task 1: Update Version Strings in the Codebase ---
echo "1. Updating version strings..."

# This block checks the OS and uses the correct 'sed' command for either Linux or macOS.
if [[ "$OSTYPE" == "darwin"* ]]; then
  # macOS
  sed -i '' "s/version: .*/version: ${MAIN_VERSION}/g" charts/jaeger/Chart.yaml
  sed -i '' "s/appVersion: .*/appVersion: ${MAIN_VERSION}/g" charts/jaeger/Chart.yaml
  sed -i '' "s/const Version = .*/const Version = \"${MAIN_VERSION}\"/g" pkg/version/version.go
  sed -i '' "s/\"version\": \".*\"/\"version\": \"${UI_VERSION}\"/g" jaeger-ui/package.json
else
  # Linux and others
  sed -i "s/version: .*/version: ${MAIN_VERSION}/g" charts/jaeger/Chart.yaml
  sed -i "s/appVersion: .*/appVersion: ${MAIN_VERSION}/g" charts/jaeger/Chart.yaml
  sed -i "s/const Version = .*/const Version = \"${MAIN_VERSION}\"/g" pkg/version/version.go
  sed -i "s/\"version\": \".*\"/\"version\": \"${UI_VERSION}\"/g" jaeger-ui/package.json
fi

# --- Task 2: Generate Changelog ---
echo "2. Generating changelog..."
PREVIOUS_TAG=$(git describe --tags --abbrev=0)
python3 ./scripts/release/notes.py --start-tag "${PREVIOUS_TAG}" --output CHANGELOG.md

# --- Task 3: Commit and Push Changes ---
echo "3. Committing changes..."
COMMIT_MSG="chore(release): Prepare release ${MAIN_VERSION}"
git add .
git commit -m "${COMMIT_MSG}"

echo "4. Pushing new branch to your fork..."
# This step was moved from the older script version to be part of the automation
# git push --set-upstream origin "${BRANCH_NAME}"
# Note: Re-enabling auto-push might be better. For now, let's stick to the reviewed logic.

# --- Final Instructions ---
echo
echo "--------------------------------------------------"
echo "✅ Release preparation script finished."
echo "Please manually push the branch and create the Pull Request."
echo "--------------------------------------------------"