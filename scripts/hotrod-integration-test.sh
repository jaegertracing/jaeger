#!/bin/bash

set -euxf -o pipefail

make docker-hotrod
REPO=jaegertracing/example-hotrod
export CID=$(docker run -d -p 8080:8080 $REPO:latest)
i=0
while [[ "$(curl -s -o /dev/null -w ''%{http_code}'' localhost:8080)" != "200" && ${i} < 30 ]]; do
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

# Only push images to dockerhub/quay.io for master branch or for release tags vM.N.P
if [[ "$BRANCH" == "master" || $BRANCH =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "upload to dockerhub/quay.io, BRANCH=$BRANCH"
else
  echo "skip docker images upload, only allowed for tagged releases or master (latest tag)"
  exit 0
fi

bash scripts/upload-to-registry.sh $REPO
