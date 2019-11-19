#!/bin/bash
#
# Build UI and all Docker images

set -e

BRANCH=${BRANCH:?'missing BRANCH env var'}
export DOCKER_NAMESPACE=jaegertracing
if [[ $BRANCH == "master" ]]; then
  source ~/.nvm/nvm.sh
  nvm use 10
  make docker
else
  echo "Skipping UI build because the current branch $BRANCH is a PR branch"
  make docker-without-ui
fi


