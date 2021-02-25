#!/bin/bash

set -euxf -o pipefail

DOCKERHUB_USERNAME=${DOCKERHUB_USERNAME:-"jaegertracingbot"}
DOCKERHUB_TOKEN=${DOCKERHUB_TOKEN:-}
QUAY_USERNAME=${QUAY_USERNAME:-"jaegertracing+github_workflows"}
QUAY_TOKEN=${QUAY_TOKEN:-}

usage() {
  echo $"Usage: $0 <image>"
  exit 1
}

check_args() {
  if [ ! $# -eq 1 ]; then
    echo "ERROR: need exactly one argument"
    usage
  fi
}

compute_image_tag() {
  local branch=$1

  if [[ "${branch}" == "master" ]]; then
    local tag="latest"
    echo ${tag}
  elif [[ "${branch}" =~ ^v([0-9]+)\.([0-9]+)\.([0-9]+)$ ]]; then
    local major="${BASH_REMATCH[1]}"
    local minor="${BASH_REMATCH[2]}"
    local patch="${BASH_REMATCH[3]}"
    local tag=${major}.${minor}.${patch}
    echo "${tag} ${major} ${minor} ${patch}"
  else
    local tag="${branch}"
    echo ${tag}
  fi
}

label_release_tag() {
  local registry=$1
  local image=$2
  local major=${3:-}
  local minor=${4:-}
  local patch=${5:-}

  docker tag ${image} ${registry}/${image}
  if [[ -n ${major} ]]; then
    docker tag ${image} ${registry}/${image}:${major}
    if [[ -n ${minor} ]]; then
      docker tag ${image} ${registry}/${image}:${major}.${minor}
      if [[ -n ${patch} ]]; then
        docker tag ${image} ${registry}/${image}:${major}.${minor}.${patch}
      fi
    fi
  fi
}

try_login() {
  local registry=$1
  local user=$2
  local token=$3
  local marker=$4

  if [ ! -f ${marker} ] && [ -v ${token} ]; then
    printenv ${token} | docker login ${registry} --username ${user} --password-stdin
    touch ${marker}
  fi
}

upload_images() {
  local registry=$1
  local image=$2
  local user=$3
  local token=$4
  local marker=.${registry}.login

  try_login ${registry} ${user} ${token} ${marker}

  if [ ! -f ${marker} ]; then
    echo "skipping upload to ${registry}, not logged in!"
  else
    docker push --all-tags ${registry}/${image}
    local snapshot_image="${image}-snapshot:${GITHUB_SHA}"
    echo "pushing snapshot image ${snapshot_image}"
    docker tag ${image} ${registry}/${snapshot_image}
    docker push ${registry}/${snapshot_image}
  fi
}

main() {
  local registry=$1
  local image=$2
  local user=$3
  local token=$4

  read -r tag major minor patch <<< "$(compute_image_tag ${BRANCH})"
  label_release_tag ${registry} ${image} ${major} ${minor} ${patch}
  upload_images ${registry} ${image} ${user} ${token}
}

check_args "$@"

main "docker.io" $1 ${DOCKERHUB_USERNAME} DOCKERHUB_TOKEN
main "quay.io" $1 ${QUAY_USERNAME} QUAY_TOKEN
