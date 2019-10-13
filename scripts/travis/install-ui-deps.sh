#!/bin/bash

set -e

# Repo for newer Node.js versions
curl -sL https://deb.nodesource.com/setup_10.x | sudo -E bash -

curl -o- -L https://yarnpkg.com/install.sh | bash

source ~/.nvm/nvm.sh
nvm install 10

yarn --version
