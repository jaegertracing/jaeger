#!/bin/bash

image_name="all-in-one"
readme_path="./README.md"
token="$DOCKERHUB_TOKEN"
repository="jaegertracing/$image_name"
url="https://hub.docker.com/v2/repositories/$repository/"
readme_content=$(cat $readme_path)
payload="{\"full_description\":\"$readme_content\"}"

response=$(curl -s -X PATCH \
  -H "Authorization: JWT $token" \
  -H "Content-Type: application/json" \
  -d "$payload" \
  $url)

status_code=$(echo "$response" | jq -r .status_code)

if [ $status_code -eq 200 ]; then
  echo "Successfully updated description for $repository"
else
  echo "Failed to update description for $repository with status code $status_code"
fi