#!/bin/bash

# this script expects all docker images to be already built, it only uploads them to Docker Hub

set -euxf -o pipefail

BRANCH=${BRANCH:?'missing BRANCH env var'}
DOCKERHUB_LOGIN=${DOCKERHUB_LOGIN:-false}

# Only push images to Docker Hub for master branch or for release tags vM.N.P and when dockerhub login is done
if [[ ("$BRANCH" == "master" || $BRANCH =~ ^v[0-9]+\.[0-9]+\.[0-9]+$) && "$DOCKERHUB_LOGIN" == "true" ]]; then
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
