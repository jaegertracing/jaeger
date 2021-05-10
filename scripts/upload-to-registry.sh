#!/bin/bash

set -euxf -o pipefail

IMAGE_TAGS=$(bash scripts/compute-tags.sh $1)
IMAGE_TAGS=$(echo ${IMAGE_TAGS} | sed "s/--tag //g")
for image_tag in $IMAGE_TAGS
do
  docker tag $1 $image_tag
done

docker push --all-tags docker.io/$1
docker push --all-tags docker.io/$1-snapshot
docker push --all-tags quay.io/$1
docker push --all-tags quay.io/$1-snapshot
