#!/bin/bash

# Builds all platform binaries.
set -e

BRANCH=${BRANCH:?'missing BRANCH env var'}

set -x

if [[ $BRANCH == "master" ]]; then
  # Only build the UI on master branch.
  source ~/.nvm/nvm.sh
  nvm use 10
  make build-ui
  make build-platform-binaries
else
  echo "Skipping UI build because current branch $BRANCH is a PR branch"
  make build-platform-binaries-without-ui
fi

