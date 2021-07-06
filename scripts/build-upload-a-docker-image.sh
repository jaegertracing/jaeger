#!/bin/bash

set -exu

base_debug_img_arg=""
docker_file_arg="Dockerfile"
target_arg=""
local_test_only='N'
platforms="linux/amd64"
name_space="jaegertracing"

while getopts "lbc:d:f:p:t:" opt; do
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

if [ ! -z ${target_arg} ]; then
    target_arg="--target ${target_arg}"
fi

docker_file_arg="${dir_arg}/${docker_file_arg}"

IMAGE_TAGS=$(bash scripts/compute-tags.sh "${name_space}/${component_name}")
upload_flag=""

if [[ "${local_test_only}" = "Y" ]]; then
    IMAGE_TAGS="--tag localhost:5000/${name_space}/${component_name}:latest"
    PUSHTAG="type=image, push=true"
else
    # Only push multi-arch images to dockerhub/quay.io for master branch or for release tags vM.N.P
    if [[ "$BRANCH" == "master" || $BRANCH =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
	    echo "build docker images and upload to dockerhub/quay.io, BRANCH=$BRANCH"
	    bash scripts/docker-login.sh
	    PUSHTAG="type=image, push=true"
	    upload_flag=" and uploading"
    else
	    echo 'skip docker images upload, only allowed for tagged releases or master (latest tag)'
	    PUSHTAG="type=image, push=false"
    fi
fi

docker buildx build --output "${PUSHTAG}" \
	--progress=plain ${target_arg} ${base_debug_img_arg}\
	--platform=${platforms} \
	--file ${docker_file_arg} \
	${IMAGE_TAGS} \
	${dir_arg}

echo "Finished building${upload_flag} ${component_name} =============="
