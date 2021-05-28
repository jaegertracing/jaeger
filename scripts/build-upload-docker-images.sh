#!/bin/bash

set -euxf -o pipefail

docker_buildx_build(){
	local component_name=$1
	local dir_arg=$2
	local docker_file_arg=$3

	if [[ "$4" == "N" ]]; then
		local target_arg=""
	else
		local target_arg="--target $4 "
	fi

	if [[ "$5" == "N" ]]; then
		local base_debug_img_arg=""
	else
		local base_debug_img_arg="--build-arg base_image=localhost:5000/baseimg:1.0.0-alpine-3.13 --build-arg debug_image=golang:1.15-alpine "
	fi

	docker buildx build --output "${PUSHTAG}" \
		--progress=plain ${target_arg} ${base_debug_img_arg}\
		--platform=${PLATFORMS} \
		${docker_file_arg} \
		$(echo ${IMAGE_TAGS} | sed "s/JAEGERCOMP/${component_name}/g") \
		${dir_arg}

	echo "Finished building multiarch jager-${component_name} =============="
}

build_upload_multiarch_images(){
	PLATFORMS="linux/amd64,linux/s390x"
	IMAGE_TAGS=$(bash scripts/compute-tags.sh "jaegertracing/jaeger-JAEGERCOMP")

	# build/upload images for Jaeger backend components
	for component in agent collector query ingester
	do
		docker_buildx_build "${component}" "cmd/${component}" "--file cmd/${component}/Dockerfile" "release" "Y" 
	done

	# build/upload images for jaeger-es-index-cleaner and jaeger-es-rollover
	docker_buildx_build "es-index-cleaner" "plugin/storage/es" "--file plugin/storage/es/Dockerfile" "N" "N"
	docker_buildx_build "es-rollover" "plugin/storage/es" "--file plugin/storage/es/Dockerfile.rollover" "N" "N"

	# build/upload images for jaeger-tracegen and jaeger-anonymizer
	for component in tracegen anonymizer
	do
		docker_buildx_build "${component}" "cmd/${component}" "--file cmd/${component}/Dockerfile" "N" "N" 
	done 
}

upload_docker_images(){
	# upload amd64 docker images
	jaeger_components=(
		agent-debug
		cassandra-schema
		collector-debug
		query-debug
		ingester-debug
	)

	for component in "${jaeger_components[@]}"
	do
		REPO="jaegertracing/jaeger-${component}"
		bash scripts/upload-to-registry.sh $REPO
	done
}

# build multi-arch binaries
make build-binaries-linux
make build-binaries-s390x
# build amd64 docker images
make docker-images-jaeger-backend-debug
make docker-images-cassandra

# Only push multi-arch images to dockerhub/quay.io for master branch or for release tags vM.N.P
if [[ "$BRANCH" == "master" || $BRANCH =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
	echo "build docker images and upload to dockerhub/quay.io, BRANCH=$BRANCH"
	bash scripts/docker-login.sh
	upload_docker_images
	PUSHTAG="type=image, push=true"
else
	echo 'skip docker images upload, only allowed for tagged releases or master (latest tag)'
	PUSHTAG="type=image, push=false"
fi
build_upload_multiarch_images



