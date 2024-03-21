#!/bin/bash

PS4='T$(date "+%H:%M:%S") '
set -euxf -o pipefail

# use global variables to reflect status of db
db_is_up=
badger_data=/badger

usage() {
  echo $"Usage: $0 <image_version>"
  exit 1
}

check_arg() {
  if [ ! $# -eq 1 ]; then
    echo "ERROR: need exactly one argument, <image_version>"
    usage
  fi
}

setup_remote_storage() {
  local image=$1
  local tag=$2
  local params=(
    --rm
    --detach
    --publish 17271:17271
    --publish 17270:17270
    --env SPAN_STORAGE_TYPE=badger
    --env BADGER_EPHEMERAL=false
    --env BADGER_DIRECTORY_VALUE="$badger_data/values"
    --env BADGER_DIRECTORY_KEY="$badger_data/keys"
    -v test:"$badger_data"
  )
  local cid
  cid=$(docker run "${params[@]}" "${image}:${tag}")
  echo "${cid}"
}

wait_for_storage() {
  local image=$1
  local url=$2
  local cid=$3
  local params=(
    --silent
    --output
    /dev/null
    --write-out
    "%{http_code}"
  )
  local counter=0
  local max_counter=60
  while [[ "$(curl "${params[@]}" "${url}")" != "200" && ${counter} -le ${max_counter} ]]; do
    docker inspect "${cid}" | jq '.[].State'
    echo "waiting for ${url} to be up..."
    sleep 10
    counter=$((counter+1))
  done
  # after the loop, do final verification and set status as global var
  if [[ "$(curl "${params[@]}" "${url}")" != "200" ]]; then
    echo "ERROR: ${image} is not ready"
    docker logs "${cid}"
    docker kill "${cid}"
    db_is_up=0
  else
    echo "SUCCESS: ${image} is ready"
    db_is_up=1
  fi
}

bring_up_storage() {
  local version=$1
  local image="jaegertracing/jaeger-remote-storage"
  local cid

  # create a dir 
  docker volume create test
  docker run --rm -v test:"$badger_data" -it busybox sh -c '
    mkdir -p '"$badger_data"' && \
    touch '"$badger_data"'/.initialized && \
    chown -R 10001:10001 '"$badger_data"'
  '

  echo "starting ${image} ${version}"
  for retry in 1 2 3
  do
    echo "attempt $retry"
    cid=$(setup_remote_storage "${image}" "${version}")

    wait_for_storage "${image}" "http://localhost:17270" "${cid}"
    if [ ${db_is_up} = "1" ]; then
      break
    fi
  done
  if [ ${db_is_up} = "1" ]; then
  # shellcheck disable=SC2064
    trap "teardown_storage ${cid}" EXIT
  else
    echo "ERROR: unable to start ${image}"
    exit 1
  fi
}

teardown_storage() {
  local cid=$1
  docker kill "${cid}"
  docker volume rm test
}

main() {
  check_arg "$@"
  local version=$1

  bring_up_storage "${version}"
  STORAGE="badger" make jaeger-storage-integration-test
}

main "$@"
