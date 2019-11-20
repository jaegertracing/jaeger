#!/bin/bash

# Builds all platform binaries.
set -e

if [[ -z "$BRANCH" ]]; then
  BRANCH=$(git rev-parse --abbrev-ref HEAD)
  echo "BRANCH env var not defined, using current branch $BRANCH instead ..."
fi

set -x

if [[ (-z "$SKIP_UI") || ($SKIP_UI == false) ]]; then
  # Only build the UI on master branch.
  source ~/.nvm/nvm.sh
  nvm use 10
  make build-ui
  make build-platform-binaries
else
  echo "Skipping UI build because current branch $BRANCH is a PR branch"
  make build-platform-binaries-without-ui
fi

