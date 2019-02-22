#!/bin/bash

set -e

# Repo for newer Node.js versions
curl -sL https://deb.nodesource.com/setup_8.x | sudo -E bash -

# Repo for Yarn (https://yarnpkg.com/en/docs/install-ci#travis-tab)
sudo apt-key adv --fetch-keys https://dl.yarnpkg.com/debian/pubkey.gpg
echo "deb https://dl.yarnpkg.com/debian/ stable main" | sudo tee /etc/apt/sources.list.d/yarn.list
sudo apt-get update -qq
sudo apt-get install -y -qq yarn

source ~/.nvm/nvm.sh
nvm install 8

yarn --version
