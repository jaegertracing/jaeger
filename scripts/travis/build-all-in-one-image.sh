#!/bin/bash

set -exu

BRANCH=${BRANCH:?'missing BRANCH env var'}
# Set default GOARCH variable to the host GOARCH, the target architecture can
# be overrided by passing architecture value to the script:
# `GOARCH=<target arch> ./scripts/travis/build-all-in-one-image.sh`.
GOARCH=${GOARCH:-$(go env GOARCH)}
# local run with `TRAVIS_SECURE_ENV_VARS=true NAMESPACE=$(whoami) BRANCH=master ./scripts/travis/build-all-in-one-image.sh`
NAMESPACE=${NAMESPACE:-jaegertracing}
ARCHS="amd64 arm64 s390x"

source ~/.nvm/nvm.sh
nvm use 10
make build-ui

set +e

run_integration_test() {
  CID=$(docker run -d -p 16686:16686 -p 5778:5778 $1:latest)
  make all-in-one-integration-test
  if [ $? -ne 0 ] ; then
      echo "---- integration test failed unexpectedly ----"
      echo "--- check the docker log below for details ---"
      docker logs $CID
      exit 1
  fi
  docker kill $CID
}

docker_buildx() {
  CMD_ROOT=${1}
  FLAGS=${@:2}

  if [[ "$FLAGS" == *"-push"*  ]]; then
    if [[ ("$BRANCH" == "master" || $BRANCH =~ ^v[0-9]+\.[0-9]+\.[0-9]+$) && "$TRAVIS_SECURE_ENV_VARS" == "true" ]]; then
      echo 'upload to Docker Hub'
    else
      echo 'skip docker upload for PR'
      exit 0
    fi
  fi

  docker buildx build -f ${CMD_ROOT}/Dockerfile ${FLAGS} ${CMD_ROOT}
}

image_tags_for() {
  REPO=${1}

  major=""
  minor=""
  patch=""

  if [[ "$BRANCH" == "master" ]]; then
    TAG="latest"
  elif [[ $BRANCH =~ ^v([0-9]+)\.([0-9]+)\.([0-9]+)$ ]]; then
    major="${BASH_REMATCH[1]}"
    minor="${BASH_REMATCH[2]}"
    patch="${BASH_REMATCH[3]}"
    TAG=${major}.${minor}.${patch}
  else
    TAG="${BRANCH///}"
  fi

  IMAGE_TAGS="--tag ${REPO}:${TAG}"

  # add major, major.minor and major.minor.patch tags
  if [[ -n $major ]]; then
    IMAGE_TAGS="${IMAGE_TAGS} -t ${REPO}:${major}"
    if [[ -n $minor ]]; then
       IMAGE_TAGS="${IMAGE_TAGS} -t ${REPO}:${major}.${minor}"
      if [[ -n $patch ]]; then
          IMAGE_TAGS="${IMAGE_TAGS} -t ${REPO}:${major}.${minor}.${patch}"
      fi
    fi
  fi

  echo "${IMAGE_TAGS}"
}

target_platforms() {
  PLATFORMS=""
  for arch in ${ARCHS}; do
    if [ -n "${PLATFORMS}" ]; then
      PLATFORMS="${PLATFORMS},linux/${arch}"
    else
      PLATFORMS="linux/${arch}"
    fi
  done
  echo ${PLATFORMS}
}


for arch in ${ARCHS}; do
  make build-all-in-one GOOS=linux GOARCH=${arch}
done
repo=${NAMESPACE}/all-in-one
docker_buildx cmd/all-in-one --load --build-arg=TARGET=release --platform=linux/${GOARCH} -t $repo:latest
run_integration_test ${repo}
docker_buildx cmd/all-in-one --push --build-arg=TARGET=release --platform=$(target_platforms) $(image_tags_for ${repo})

for arch in ${ARCHS}; do
  make build-all-in-one-debug GOOS=linux GOARCH=${arch}
done
repo=${NAMESPACE}/all-in-one-debug
docker_buildx cmd/all-in-one --load --build-arg=TARGET=debug --platform=linux/${GOARCH} -t $repo:latest
run_integration_test ${repo}
docker_buildx cmd/all-in-one --push --build-arg=TARGET=debug --platform=$(target_platforms) $(image_tags_for ${repo})

for arch in ${ARCHS}; do
  make build-otel-all-in-one GOOS=linux GOARCH=${arch}
done
repo=${NAMESPACE}/opentelemetry-all-in-one
docker_buildx cmd/opentelemetry/cmd/all-in-one --load --platform=linux/${GOARCH} -t $repo:latest
run_integration_test ${repo}
docker_buildx cmd/opentelemetry/cmd/all-in-one --push --platform=$(target_platforms) $(image_tags_for ${repo})