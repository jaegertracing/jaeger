#!/bin/bash
#
# Build UI and all Docker images

set -e

# TODO avoid building the UI when on a PR branch: https://github.com/jaegertracing/jaeger/issues/1908
source ~/.nvm/nvm.sh
nvm use 10

export DOCKER_NAMESPACE=jaegertracing
make docker
