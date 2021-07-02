#!/bin/bash

set -exu

DOCKERHUB_USERNAME=${DOCKERHUB_USERNAME:-"jaegertracingbot"}
DOCKERHUB_TOKEN=${DOCKERHUB_TOKEN:-}
QUAY_USERNAME=${QUAY_USERNAME:-"jaegertracing+github_workflows"}
QUAY_TOKEN=${QUAY_TOKEN:-}

echo "Performing a 'docker login' for DockerHub"
echo "${DOCKERHUB_TOKEN}" | docker login -u "${DOCKERHUB_USERNAME}" docker.io --password-stdin

echo "Performing a 'docker login' for Quay"
echo "${QUAY_TOKEN}" | docker login -u "${QUAY_USERNAME}" quay.io --password-stdin
