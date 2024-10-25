#!/bin/bash

# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0
set -euxf -o pipefail

usage() {
  echo "Usage: $0 <repository_name> <file_path>"
  exit 1
}

if [ "$#" -ne 2 ]; then
  echo "Error: Missing arguments."
  usage
fi

repo="$1"
readme_path="$2"
abs_readme_path=$(realpath "$readme_path")

DOCKERHUB_TOKEN=${DOCKERHUB_TOKEN:?'missing Docker Hub token'}
dockerhub_repository="jaegertracing/$repo"
dockerhub_url="https://hub.docker.com/v2/repositories/$dockerhub_repository/"

if [ ! -f "$abs_readme_path" ]; then
  echo "Warning: the dedicated README file for Docker image $repo was not found at path $abs_readme_path"
  echo "It is recommended to have a dedicated README file for each Docker image"
  exit 0
fi

readme_content=$(<"$abs_readme_path")

set +x

body=$(jq -n \
  --arg full_desc "$readme_content" \
  '{full_description: $full_desc}')

dockerhub_response=$(curl -s -w "%{http_code}" -X PATCH "$dockerhub_url" \
    -H "Content-Type: application/json" \
    -H "Authorization: JWT $DOCKERHUB_TOKEN" \
    -d "$body")

http_code="${dockerhub_response: -3}"
response_body="${dockerhub_response:0:${#dockerhub_response}-3}"

if [ "$http_code" -eq 200 ]; then
  echo "Successfully updated Docker Hub README for $dockerhub_repository"
else
  echo "Failed to update Docker Hub README for $dockerhub_repository with status code $http_code"
  echo "Full response: $response_body"
fi
