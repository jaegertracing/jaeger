#!/bin/bash

set -euxf -o pipefail

build_upload_multiarch_images(){
	for component in agent collector query ingester
	do
		docker buildx build --output "${PUSHTAG}" \
			--progress=plain --target release \
			--build-arg base_image="localhost:5000/baseimg:1.0.0-alpine-3.12" \
			--build-arg debug_image="golang:1.15-alpine" \
			--platform=${PLATFORMS} \
			--file cmd/${component}/Dockerfile \
			$(echo ${IMAGE_TAGS} | sed "s/JAEGERCOMP/${component}/g") \
			cmd/${component}
		echo "Finished building multiarch jager-${component} =============="
	done

	for component in es-index-cleaner es-rollover tracegen anonymizer
	do
		if [[ "${component}" == "es-rollover" ]]; then
			docker_file_arg="--file plugin/storage/es/Dockerfile.rollover"
		else
			docker_file_arg=""
		fi
		if [[ "${component}" =~ ^es-.* ]]; then
			dir_arg="plugin/storage/es"
		else 
			dir_arg="cmd/${component}"
		fi
		docker buildx build --output "${PUSHTAG}" \
			--progress=plain \
			--platform=${PLATFORMS} \
			$(echo ${IMAGE_TAGS} | sed "s/JAEGERCOMP/${component}/g") \
			${dir_arg} \
			${docker_file_arg}
		echo "Finished building multiarch jaeger-${component} =============="
	done 
}

#Step 1: build and upload multiarch docker images
make build-binaries-linux
make build-binaries-s390x

PLATFORMS="linux/amd64,linux/s390x"
bash scripts/build-multiarch-baseimg.sh

IMAGE_TAGS=$(bash scripts/compute-tags.sh "jaegertracing/jaeger-JAEGERCOMP")

# Only push multi-arch images to dockerhub/quay.io for master branch or for release tags vM.N.P
if [[ "$BRANCH" == "master" || $BRANCH =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "build multiarch images and upload to dockerhub/quay.io, BRANCH=$BRANCH"
  bash scripts/docker-login.sh
  PUSHTAG="type=image, push=true"
else
  echo 'skip multiarch docker images upload, only allowed for tagged releases or master (latest tag)'
  PUSHTAG="type=image, push=false"
fi
build_upload_multiarch_images

#Step 2: build and upload amd64 docker images
make docker-images-jaeger-backend-debug
make docker-images-cassandra
# Only push amd64 specific images to dockerhub/quay.io for master branch or for release tags vM.N.P
if [[ "$BRANCH" == "master" || $BRANCH =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "upload to dockerhub/quay.io, BRANCH=$BRANCH"
else
  echo 'skip docker images upload, only allowed for tagged releases or master (latest tag)'
  exit 0
fi

export DOCKER_NAMESPACE=jaegertracing

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

