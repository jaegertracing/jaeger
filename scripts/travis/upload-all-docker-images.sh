#!/bin/bash

# this script should only be run after build-docker-images.sh

set -e

BRANCH=${BRANCH:?'missing BRANCH env var'}

if [[ "$TRAVIS_SECURE_ENV_VARS" == "false" ]]; then
  echo "skip docker upload, TRAVIS_SECURE_ENV_VARS=$TRAVIS_SECURE_ENV_VARS"
  exit 0
fi

# Only push images to Docker Hub for master branch or for release tags vM.N.P
if [[ "$BRANCH" == "master" || $BRANCH =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "upload to Docker Hub, BRANCH=$BRANCH"
else
  echo 'skip Docker upload, only allowed for tagged releases or master (latest tag)'
  exit 0
fi

export DOCKER_NAMESPACE=jaegertracing

jaeger_components=(
	agent
	agent-debug
	cassandra-schema
	es-index-cleaner
	es-rollover
	collector
	collector-debug
	query
	query-debug
	ingester
	ingester-debug
	tracegen
	anonymizer
	opentelemetry-collector
	opentelemetry-agent
	opentelemetry-ingester
)

for component in "${jaeger_components[@]}"
do
  export REPO="jaegertracing/jaeger-${component}"
  bash ./scripts/travis/upload-to-docker.sh
done
