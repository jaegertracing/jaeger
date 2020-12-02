#!/bin/bash

set -e


REQUIRED_BUILDX_VERSION="0.4.2";
BUILDER_NAME=jaeger-builder

# Ensure "docker buildx" is available and enabled. For more details, see: https://github.com/docker/buildx/blob/master/README.md

# This does a few things:
#  1. Makes sure docker is in PATH
#  2. Downloads and installs buildx if no version of buildx is installed yet
#  3. Makes sure any installed buildx is a required version or newer
#  4. Makes sure the user has enabled buildx (either by default or by setting DOCKER_CLI_EXPERIMENTAL env var to 'enabled')
#  Thus, this target will only ever succeed if a required (or newer) version of 'docker buildx' is available and enabled.

if ! which docker > /dev/null 2>&1; then echo "'docker' is not in your PATH."; exit 1; fi

if ! DOCKER_CLI_EXPERIMENTAL="enabled" docker buildx version > /dev/null 2>&1 ; then
  buildx_download_url="https://github.com/docker/buildx/releases/download/v${REQUIRED_BUILDX_VERSION}/buildx-v${REQUIRED_BUILDX_VERSION}.$(go env GOOS)-$(go env GOARCH)";

  echo "You do not have 'docker buildx' installed. Will now download from [${buildx_download_url}] and install it to [${HOME}/.docker/cli-plugins].";

  mkdir -p ${HOME}/.docker/cli-plugins;
  curl -L --output ${HOME}/.docker/cli-plugins/docker-buildx "${buildx_download_url}";
  chmod a+x ${HOME}/.docker/cli-plugins/docker-buildx;

  installed_version="$(DOCKER_CLI_EXPERIMENTAL="enabled" docker buildx version || echo "unknown")";

  if docker buildx version > /dev/null 2>&1; then
    echo "'docker buildx' has been installed and is enabled [version=${installed_version}]";
  else
    echo "An attempt to install 'docker buildx' has been made but it either failed or is not enabled by default. [version=${installed_version}]";
    echo "Set DOCKER_CLI_EXPERIMENTAL=enabled to enable it.";
    exit 1;
  fi
fi;

current_buildx_version="$(DOCKER_CLI_EXPERIMENTAL=enabled docker buildx version 2>/dev/null | sed -E 's/.*v([0-9]+\.[0-9]+\.[0-9]+).*/\1/g')";
is_valid_buildx_version="$(if [ "$(printf ${REQUIRED_BUILDX_VERSION}\\n${current_buildx_version} | sort -V | head -n1)" == "${REQUIRED_BUILDX_VERSION}" ]; then echo "true"; else echo "false"; fi)";

if [ "${is_valid_buildx_version}" == "true" ]; then
  echo "A valid version of 'docker buildx' is available: ${current_buildx_version}";
else
  echo "You have an older version of 'docker buildx' that is not compatible. Please upgrade to at least v${REQUIRED_BUILDX_VERSION}";
  exit 1;
fi;

if docker buildx version > /dev/null 2>&1; then
  echo "'docker buildx' is enabled";
else
  echo "'docker buildx' is not enabled. Set DOCKER_CLI_EXPERIMENTAL=enabled if you want to use it.";
  exit 1;
fi

# Ensure a local builder for multi-arch build. For more details, see: https://github.com/docker/buildx/blob/master/README.md#building-multi-platform-images
if ! docker buildx inspect ${BUILDER_NAME} > /dev/null 2>&1; then
  echo "The buildx builder instance named '${BUILDER_NAME}' does not exist. Creating one now.";
  if ! docker buildx create --name=${BUILDER_NAME} --driver-opt=image=moby/buildkit:v0.8.0; then
    echo "Failed to create the buildx builder '${BUILDER_NAME}'";
    exit 1;
  fi
fi;

if [[ $(uname -s) == "Linux" ]]; then
  echo "Ensuring QEMU is set up for this Linux host";
  if ! docker run --privileged --rm tonistiigi/binfmt --install all; then
    echo "Failed to ensure QEMU is set up. This build will be allowed to continue, but it may fail at a later step.";
  fi
fi