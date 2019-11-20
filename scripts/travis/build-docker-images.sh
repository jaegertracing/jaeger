#!/bin/bash
#
# Build UI and all Docker images

set -e

if [[ -z "$BRANCH" ]]; then
  BRANCH=$(git rev-parse --abbrev-ref HEAD)
  echo "BRANCH env var not defined, using current branch $BRANCH instead ..."
fi

export DOCKER_NAMESPACE=jaegertracing
if [[ (-z "$SKIP_UI") || ($SKIP_UI == false) ]]; then
  source ~/.nvm/nvm.sh
  nvm use 10
  make docker
else
  echo "Skipping UI build because the current branch $BRANCH is a PR branch"
  make docker-without-ui
fi
