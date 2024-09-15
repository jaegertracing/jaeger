#!/bin/bash

# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

set -euf -o pipefail

print_help() {
  echo "Usage: $0 [-c] [-D] [-h] [-l] [-o] [-p platforms]"
  echo "-h: Print help"
  echo "-b: add base_image and debug_image arguments to the build command"
  echo "-c: name of the component to build"
  echo "-d: directory for the Dockerfile"
  echo "-f: override the name of the Dockerfile (-d still respected)"
  echo "-o: overwrite image in the target remote repository even if the semver tag already exists"
  echo "-p: Comma-separated list of platforms to build for (default: all supported)"
  echo "-t: Release target (release|debug) if required by the Dockerfile"
  exit 1
}

echo "BRANCH=${BRANCH:?'expecting BRANCH env var'}"
base_debug_img_arg=""
docker_file_arg="Dockerfile"
target_arg=""
local_test_only='N'
platforms="linux/amd64"
namespace="jaegertracing"
overwrite='N'

while getopts "bc:d:f:hlop:t:" opt; do
	# shellcheck disable=SC2220 # we don't need a *) case
	case "${opt}" in
	b)
		base_debug_img_arg="--build-arg base_image=localhost:5000/baseimg_alpine:latest --build-arg debug_image=localhost:5000/debugimg_alpine:latest "
		;;
	c)
		component_name=${OPTARG}
		;;
	d)
		dir_arg=${OPTARG}
		;;
	f)
		docker_file_arg=${OPTARG}
		;;
	l)
		local_test_only='Y'
		;;
	o)
		overwrite='Y'
		;;
	p)
		platforms=${OPTARG}
		;;
	t)
		target_arg=${OPTARG}
		;;
	?)
		print_help
		;;
	esac
done

set -x

if [ -n "${target_arg}" ]; then
    target_arg="--target ${target_arg}"
fi

docker_file_arg="${dir_arg}/${docker_file_arg}"

check_overwrite() {
  for image in "$@"; do
    if [[ "$image" == "--tag" ]]; then
      continue
    fi
    if [[ $image =~ -snapshot ]]; then
      continue
    fi
    tag=${image#*:}
    if [[ $tag =~ ^[0-9]+\.[0-9]+\.[0-9]+(-rc[0-9]+)?$ ]]; then
      echo "Checking if image $image already exists"
      if docker manifest inspect "$image" >/dev/null 2>&1; then
        echo "❌ ERROR: Image $image already exists and overwrite=$overwrite"
        exit 1
      fi
    fi
  done
}

upload_comment=""

if [[ "${local_test_only}" = "Y" ]]; then
    IMAGE_TAGS=("--tag" "localhost:5000/${namespace}/${component_name}:${GITHUB_SHA}")
    PUSHTAG="type=image,push=true"
else
    echo "::group:: compute tags ${component_name}"
    # shellcheck disable=SC2086
    IFS=" " read -r -a IMAGE_TAGS <<< "$(bash scripts/compute-tags.sh ${namespace}/${component_name})"
    echo "::endgroup::"

    # Only push multi-arch images to dockerhub/quay.io for main branch or for release tags vM.N.P{-rcX}
    if [[ "$BRANCH" == "main" || $BRANCH =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-rc[0-9]+)?$ ]]; then
	    echo "will build docker images and upload to dockerhub/quay.io, BRANCH=$BRANCH"
	    bash scripts/docker-login.sh
	    PUSHTAG="type=image,push=true"
	    upload_comment=" and uploading"
	    if [[ "$overwrite" == 'N' ]]; then
	      check_overwrite "${IMAGE_TAGS[@]}"
	    fi
    else
	    echo 'skipping docker images upload, because not on tagged release or main branch'
	    PUSHTAG="type=image,push=false"
    fi
fi

echo "::group:: docker build ${component_name}"
# Some of the variables can be blank and should not produce extra arguments,
# so we need to disable the linter checks for quoting.
# TODO: collect arguments into an array and add optional once conditionally
# shellcheck disable=SC2086
docker buildx build --output "${PUSHTAG}" ${target_arg} ${base_debug_img_arg} \
	--progress=plain \
	--platform="${platforms}" \
	--file "${docker_file_arg}" \
	"${IMAGE_TAGS[@]}" \
	"${dir_arg}"
echo "::endgroup::"
echo "Finished building${upload_comment} ${component_name} =============="

echo "::group:: docker prune"
df -h /
docker buildx prune --all --force
docker system prune --force
df -h /
echo "::endgroup::"
