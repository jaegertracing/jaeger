#!/bin/bash

set -e

# Repo for newer Node.js versions
curl -sL https://deb.nodesource.com/setup_6.x | sudo -E bash -

# Repo for Yarn (https://yarnpkg.com/en/docs/install-ci#travis-tab)
sudo apt-key adv --keyserver pgp.mit.edu --recv D101F7899D41F3C3
echo "deb http://dl.yarnpkg.com/debian/ stable main" | sudo tee /etc/apt/sources.list.d/yarn.list
sudo apt-get update -qq
sudo apt-get install -y -qq yarn

source ~/.nvm/nvm.sh
nvm install 6

yarn --version
nvm use 6
node -v
