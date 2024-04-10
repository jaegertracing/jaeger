#!/bin/bash

PS4='T$(date "+%H:%M:%S") '
set -euxf -o pipefail

# use global variables to reflect status of db
db_is_up=

usage() {
  echo $"Usage: $0 <elasticsearch|opensearch> <version>"
  exit 1
}

check_arg() {
  if [ ! $# -eq 2 ]; then
    echo "ERROR: need exactly two arguments, <elasticsearch|opensearch> <image>"
    usage
  fi
}

setup_es() {
  local tag=$1
  local image=docker.elastic.co/elasticsearch/elasticsearch
  local params=(
    --detach
    --publish 9200:9200
    --env "http.host=0.0.0.0"
    --env "transport.host=127.0.0.1"
    --env "xpack.security.enabled=false"
  )
  local major_version=${tag%%.*}
  if (( major_version < 8 )); then
    params+=(--env "xpack.monitoring.enabled=false")
  else
    params+=(--env "xpack.monitoring.collection.enabled=false")
  fi
  if (( major_version > 7 )); then
    params+=(
      --env "action.destructive_requires_name=false"
    )
  fi

  local cid
  cid=$(docker run "${params[@]}" "${image}:${tag}")
  echo "${cid}"
}

setup_opensearch() {
  local image=opensearchproject/opensearch
  local tag=$1
  local params=(
    --detach
    --publish 9200:9200
    --env "http.host=0.0.0.0"
    --env "transport.host=127.0.0.1"
    --env "plugins.security.disabled=true"
  )
  local cid
  cid=$(docker run "${params[@]}" "${image}:${tag}")
  echo "${cid}"
}

wait_for_storage() {
  local distro=$1
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
    echo "ERROR: ${distro} is not ready"
    docker logs "${cid}"
    docker kill "${cid}"
    db_is_up=0
  else
    echo "SUCCESS: ${distro} is ready"
    db_is_up=1
  fi
}

bring_up_storage() {
  local distro=$1
  local version=$2
  local cid

  echo "starting ${distro} ${version}"
  for retry in 1 2 3
  do
    echo "attempt $retry"
    if [ "${distro}" = "elasticsearch" ]; then
      cid=$(setup_es "${version}")
    elif [ "${distro}" == "opensearch" ]; then
      cid=$(setup_opensearch "${version}")
    else
      echo "Unknown distribution $distro. Valid options are opensearch or elasticsearch"
      usage
    fi
    wait_for_storage "${distro}" "http://localhost:9200" "${cid}"
    if [ ${db_is_up} = "1" ]; then
      break
    fi
  done
  if [ ${db_is_up} = "1" ]; then
  # shellcheck disable=SC2064
    trap "teardown_storage ${cid}" EXIT
  else
    echo "ERROR: unable to start ${distro}"
    exit 1
  fi
}

teardown_storage() {
  local cid=$1
  docker kill "${cid}"
}

main() {
  check_arg "$@"
  local distro=$1
  local version=$2

  bring_up_storage "${distro}" "${version}"
  STORAGE=${distro} make storage-integration-test
  STORAGE=${distro} SPAN_STORAGE_TYPE=${distro} make jaeger-v2-storage-integration-test
  make index-cleaner-integration-test
  make index-rollover-integration-test
}

main "$@"
