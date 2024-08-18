#!/bin/bash

# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

set -euf -o pipefail

base_debug_img_arg=""
docker_file_arg="Dockerfile"
target_arg=""
local_test_only='N'
platforms="linux/amd64"
namespace="jaegertracing"

while getopts "lbc:d:f:p:t:" opt; do
	# shellcheck disable=SC2220 # we don't need a *) case
	case "${opt}" in
	c)
		component_name=${OPTARG}
		;;
	b)
		base_debug_img_arg="--build-arg base_image=localhost:5000/baseimg_alpine:latest --build-arg debug_image=localhost:5000/debugimg_alpine:latest "
		;;
	d)
		dir_arg=${OPTARG}
		;;
	f)
		docker_file_arg=${OPTARG}
		;;
	p)
		platforms=${OPTARG}
		;;
	t)
		target_arg=${OPTARG}
		;;
	l)
		local_test_only='Y'
		;;
	esac
done

set -x

if [ -n "${target_arg}" ]; then
    target_arg="--target ${target_arg}"
fi

docker_file_arg="${dir_arg}/${docker_file_arg}"

# shellcheck disable=SC2086
IFS=" " read -r -a IMAGE_TAGS <<< "$(bash scripts/compute-tags.sh ${namespace}/${component_name})"
upload_flag=""

if [[ "${local_test_only}" = "Y" ]]; then
    IMAGE_TAGS=("--tag" "localhost:5000/${namespace}/${component_name}:${GITHUB_SHA}")
    PUSHTAG="type=image, push=true"
else
    # Only push multi-arch images to dockerhub/quay.io for main branch or for release tags vM.N.P
    if [[ "$BRANCH" == "main" || $BRANCH =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
	    echo "build docker images and upload to dockerhub/quay.io, BRANCH=$BRANCH"
	    bash scripts/docker-login.sh
	    PUSHTAG="type=image, push=true"
	    upload_flag=" and uploading"
    else
	    echo 'skipping docker images upload, because not on tagged release or main branch'
	    PUSHTAG="type=image, push=false"
    fi
fi

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

echo "Finished building${upload_flag} ${component_name} =============="

df -h /
docker buildx prune --all --force
docker system prune --force
df -h /
