#!/bin/bash

set -euxf -o pipefail

make create-baseimg-debugimg

# build multi-arch binaries
make build-binaries-linux
make build-binaries-s390x

platforms="linux/amd64,linux/s390x"
base_debug_img_arg="--build-arg base_image=localhost:5000/baseimg_alpine:latest --build-arg debug_image=golang:1.15-alpine "

# build/upload images for release version of Jaeger backend components
for component in agent collector query ingester
do
	bash scripts/build-upload-a-docker-image.sh -c "jaeger-${component}" -b "${base_debug_img_arg}" -d "cmd/${component}" -p "${platforms}" -t release
done

# build/upload images for jaeger-es-index-cleaner and jaeger-es-rollover
bash scripts/build-upload-a-docker-image.sh -c jaeger-es-index-cleaner -d plugin/storage/es -p "${platforms}"
bash scripts/build-upload-a-docker-image.sh -c jaeger-es-rollover -d plugin/storage/es -f Dockerfile.rollover -p "${platforms}"

# build/upload images for jaeger-tracegen and jaeger-anonymizer
for component in tracegen anonymizer
do
	bash scripts/build-upload-a-docker-image.sh -c "jaeger-${component}" -d "cmd/${component}" -p "${platforms}"
done 


# build amd64 docker images
platforms="linux/amd64"
base_debug_img_arg="--build-arg base_image=localhost:5000/baseimg_alpine:latest --build-arg debug_image=localhost:5000/debugimg_alpine:latest "

# build/upload images for debug version of Jaeger backend components
for component in agent collector query ingester
do
	bash scripts/build-upload-a-docker-image.sh -c "jaeger-${component}-debug" -b "${base_debug_img_arg}" -d "cmd/${component}" -p "${platforms}" -t debug
done

# build/upload images for jaeger-cassandra-schema
bash scripts/build-upload-a-docker-image.sh -c jaeger-cassandra-schema -d plugin/storage/cassandra/ -p "${platforms}"
