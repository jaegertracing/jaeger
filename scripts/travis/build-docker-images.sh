#!/bin/bash
#
# Build UI and all Docker images

set -e

export DOCKER_NAMESPACE=jaegertracing
if [[ "$SKIP_UI" == "true" ]]; then
  echo "Skipping UI build because \$SKIP_UI is set to true"
  make docker-without-ui
else
  source ~/.nvm/nvm.sh
  nvm use 10
  make docker
fi
