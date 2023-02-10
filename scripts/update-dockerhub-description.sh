#!/bin/bash

set -euxf -o pipefail

if [ "${WORKFLOW}" == "ci-docker-build.yml" ]; then
  binary="docker"
elif [ "${WORKFLOW}" == "ci-all-in-one-build.yml" ]; then
  binary="all-in-one"
else
  echo "Error: unknown workflow ${WORKFLOW}"
  exit 1
fi

# Check for a more specific README file
binary_readme_path="./cmd/${binary}/README.md"
if [ -f "$binary_readme_path" ]; then
  readme_path="$binary_readme_path"
else
  readme_path="./README.md"
fi

image_name="$binary"
dockerhub_repository="jaegertracing/$image_name"
dockerhub_url="https://hub.docker.com/v2/repositories/$dockerhub_repository/"

quay_repository="quay.io/jaegertracing/$image_name"
quay_url="https://quay.io/api/v1/repository/$quay_repository/description"

tempfile=$(mktemp)
cat $readme_path > $tempfile

set +x

dockerhub_response=$(curl -s -X PATCH \
  -H "Authorization: JWT $DOCKERHUB_TOKEN" \
  -H "Content-Type: multipart/form-data" \
  -F "full_description=@$tempfile" \
  $dockerhub_url)

dockerhub_status_code=$(echo "$dockerhub_response" | jq -r .status_code)

if [ $dockerhub_status_code -eq 200 ]; then
  echo "Successfully updated Docker Hub README for $dockerhub_repository"
else
  echo "Failed to update Docker Hub README for $dockerhub_repository with status code $dockerhub_status_code"
  echo "Full response: $dockerhub_response"
fi

quay_response=$(curl -s -X PUT \
  -H "Authorization: Bearer $QUAY_TOKEN" \
  -H "Content-Type: multipart/form-data" \
  -F "description=@$tempfile" \
  $quay_url)

quay_status_code=$(echo "$quay_response" | jq -r .status)

if [ $quay_status_code -eq 200 ]; then
  echo "Successfully updated Quay.io README for $quay_repository"
else
  echo "Failed to update Quay.io README for $quay_repository with status code $quay_status_code"
  echo "Full response: $quay_response"
fi

set -x

rm $tempfile