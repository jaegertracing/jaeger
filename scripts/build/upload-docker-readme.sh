#!/bin/bash
# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

set -euf -o pipefail

usage() {
  echo "Usage: $0 <repository_name> <file_path>"
  exit 1
}

if [ "$#" -ne 2 ]; then
  echo "🛑 Error: Missing arguments."
  usage
fi

repo="$1"
readme_path="$2"

# Check if README file exists before calling realpath
if [ ! -f "$readme_path" ]; then
  echo "🟡 Warning: no README file found at path $readme_path"
  exit 0
fi

abs_readme_path=$(realpath "$readme_path")
repository="jaegertracing/$repo"

DOCKERHUB_TOKEN=${DOCKERHUB_TOKEN:?'missing Docker Hub token'}
DOCKERHUB_USERNAME=${DOCKERHUB_USERNAME:?'missing Docker Hub user name'}
QUAY_TOKEN=${QUAY_TOKEN:?'missing Quay token'}

dockerhub_url="https://hub.docker.com/v2/repositories/$repository/"
quay_url="https://quay.io/api/v1/repository/${repository}"

readme_content=$(<"$abs_readme_path")

# 🛑 IMPORTANT: do not echo commands as they contain tokens
set +x

# Handle DockerHUB upload

# Get Docker Hub JWT token from PAT
dockerhub_credentials=$(jq -n \
  --arg pwd "$DOCKERHUB_TOKEN" \
  --arg user "$DOCKERHUB_USERNAME" \
  '{username: $user, password: $pwd}')
dockerhub_jwt=$(curl -s -H "Content-Type: application/json" \
  -X POST -d "$dockerhub_credentials" \
  https://hub.docker.com/v2/users/login/ | jq -r .token)

if [ "$dockerhub_jwt" = "null" ] || [ -z "$dockerhub_jwt" ]; then
  echo "🛑 Failed to get Docker Hub JWT token"
  exit 1
fi

# encode readme as properly escaped JSON
body=$(jq -n \
  --arg full_desc "$readme_content" \
  '{full_description: $full_desc}')

dockerhub_response=$(curl -s -w "%{http_code}" -X PATCH "$dockerhub_url" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $dockerhub_jwt" \
    -d "$body")

http_code="${dockerhub_response: -3}"
response_body="${dockerhub_response:0:${#dockerhub_response}-3}"

if [ "$http_code" -eq 200 ]; then
  echo "✅ Successfully updated Docker Hub README for $repository"
else
  echo "🛑 Failed to update Docker Hub README for $repository with status code $http_code"
  echo "🛑 Full response: $response_body"
fi

# Handle Quay upload

# encode readme as properly escaped JSON
quay_body=$(jq -n \
  --arg full_desc "$readme_content" \
  '{description: $full_desc}')

quay_response=$(curl -s -w "%{http_code}" -X PUT "$quay_url" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $QUAY_TOKEN" \
    -d "$quay_body")

quay_http_code="${quay_response: -3}"
quay_response_body="${quay_response:0:${#quay_response}-3}"

if [ "$quay_http_code" -eq 200 ]; then
  echo "✅ Successfully updated Quay.io README for $repository"
else
  echo "🛑 Failed to update Quay.io README for $repository with status code $quay_http_code"
  echo "🛑 Full response: $quay_response_body"
fi
