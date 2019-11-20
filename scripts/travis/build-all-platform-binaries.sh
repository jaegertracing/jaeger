#!/bin/bash

set -x

if [[ "$SKIP_UI" == "true" ]]; then
  echo "Skipping UI build because \$SKIP_UI is set to true"
  make build-platform-binaries-without-ui
else
  source ~/.nvm/nvm.sh
  nvm use 10
  make build-ui
  make build-platform-binaries
fi

