#!/bin/bash

set -e

# Repo for newer Node.js versions
curl -sL https://deb.nodesource.com/setup_8.x | sudo -E bash -

curl -o- -L https://yarnpkg.com/install.sh | bash
PATH=$HOME/.yarn/bin:$PATH

source ~/.nvm/nvm.sh
nvm install 8

yarn --version
