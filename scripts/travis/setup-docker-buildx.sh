#!/bin/bash

set -eux

# update docker
# for setup buildx https://travis-ci.community/t/installing-docker-19-03/8077/2
sudo apt update

sudo systemctl stop docker
sudo apt install -y docker.io

sudo systemctl unmask docker.service
sudo systemctl unmask docker.socket
sudo systemctl start docker
sudo systemctl status docker.socket

docker version

DOCKER_BUILDX_VERSION=v0.4.2

LOCAL_OS=$(uname -s | tr '[A-Z]' '[a-z]')

case $(uname -m) in
x86_64)
  LOCAL_ARCH=amd64
  ;;
aarch64)
  LOCAL_ARCH=arm64
  ;;
*)
  echo "unsupported architecture"
  exit 1
  ;;
esac

if [[ ! -f ~/.docker/cli-plugins/docker-buildx ]]; then
  DOCKER_BUILDX_DOWNLOAD_URL=https://github.com/docker/buildx/releases/download/${DOCKER_BUILDX_VERSION}/buildx-${DOCKER_BUILDX_VERSION}.${LOCAL_OS}-${LOCAL_ARCH}
  mkdir -p ~/.docker/cli-plugins
  echo "downloading from ${DOCKER_BUILDX_DOWNLOAD_URL}"
  curl -sL --output ~/.docker/cli-plugins/docker-buildx "${DOCKER_BUILDX_DOWNLOAD_URL}"
  chmod a+x ~/.docker/cli-plugins/docker-buildx
fi

# enable buildx
export DOCKER_CLI_EXPERIMENTAL=enabled

# checkout buildx available
docker buildx version

# enabled qemu if needed
if [[ ! $(docker buildx inspect default | grep Platforms) == *arm64* ]]; then
  docker run --privileged --rm tonistiigi/binfmt --install all
fi

# setup builder
docker buildx create --use
