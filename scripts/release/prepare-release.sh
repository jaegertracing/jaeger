#!/bin/bash
set -e # Exit if any command fails

MAIN_VERSION=$1
UI_VERSION=$2

if [ -z "$MAIN_VERSION" ] || [ -z "$UI_VERSION" ]; then
  echo "Error: Missing version arguments."
  echo "Usage: ./scripts/release/prepare-release.sh <main-version> <ui-version>"
  echo "Example: ./scripts/release/prepare-release.sh 1.73.0 v2.10.0" # Note the 'v' for UI version
  exit 1
fi

BRANCH_NAME="release-${MAIN_VERSION}"
COMMIT_MSG="chore(release): Prepare release ${MAIN_VERSION}"

echo "--- Creating new release branch: ${BRANCH_NAME} ---"
git checkout -b "${BRANCH_NAME}"

echo "--- Preparing release for main version: $MAIN_VERSION and UI version: $UI_VERSION ---"

# --- Task 1: Update Version Strings in the Codebase ---
echo "1. Updating version strings in main repository..."
sed -i "s/version: .*/version: ${MAIN_VERSION}/g" charts/jaeger/Chart.yaml
sed -i "s/appVersion: .*/appVersion: ${MAIN_VERSION}/g" charts/jaeger/Chart.yaml
sed -i "s/const Version = .*/const Version = \"${MAIN_VERSION}\"/g" pkg/version/version.go

# --- Task 2: Update Jaeger UI Submodule ---
echo "2. Updating jaeger-ui submodule to version ${UI_VERSION}..."
# Initialize and update the submodule to the latest from its main branch
git submodule update --init --recursive
pushd jaeger-ui
git checkout main
git pull
# Check out the specific version tag for the new UI release
git checkout "${UI_VERSION}"
popd

# --- Task 3: Generate Changelog ---
echo "3. Generating changelog..."
PREVIOUS_TAG=$(git describe --tags --abbrev=0)
python3 ./scripts/release/notes.py --start-tag "${PREVIOUS_TAG}" --output CHANGELOG.md

# --- Task 4: Commit and Push Changes ---
echo "4. Committing all changes..."
git add .
git commit -m "${COMMIT_MSG}"

echo "5. Pushing new branch to your fork..."
# The user will need to have their fork set up as 'origin'
git push --set-upstream origin "${BRANCH_NAME}"

# --- Final Instructions ---
echo
echo "--------------------------------------------------"
echo "✅ Release branch '${BRANCH_NAME}' has been created and pushed."
echo "You can now go to GitHub to open a Pull Request."
echo "--------------------------------------------------"