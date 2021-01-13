#!/bin/bash

set -euxf -o pipefail

make docker-hotrod
export REPO=jaegertracing/example-hotrod
export CID=$(docker run -d -p 8080:8080 $REPO:latest)
i=0
while [[ "$(curl -s -o /dev/null -w ''%{http_code}'' localhost:8080)" != "200" && ${i} < 5 ]]; do
  sleep 1
  i=$((i+1))
done
body=$(curl localhost:8080)
if [[ $body != *"Rides On Demand"* ]]; then
  echo "String \"Rides On Demand\" is not present on the index page"
  exit 1
fi
docker rm -f $CID

BRANCH=${BRANCH:?'missing BRANCH env var'}
DOCKERHUB_LOGIN=${DOCKERHUB_LOGIN:-false}

# Only push images to Docker Hub for master branch or for release tags vM.N.P and when dockerhub login is done
if [[ ("$BRANCH" == "master" || $BRANCH =~ ^v[0-9]+\.[0-9]+\.[0-9]+$) && "$DOCKERHUB_LOGIN" == "true" ]]; then
  echo "upload to Docker Hub, BRANCH=$BRANCH"
else
  echo "skip Docker upload, only allowed for tagged releases or master (latest tag)"
  exit 0
fi

bash ./scripts/travis/upload-to-docker.sh
