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
  if [ ! $# -eq 3 ]; then
    echo "ERROR: need exactly two arguments, <elasticsearch|opensearch> <image> <jaeger-version>"
    usage
  fi
}

setup_es() {
  local tag=$1
  local major_version=${tag%%.*}
  local compose_file
  compose_file="docker-compose/elasticsearch/v${major_verison}.yml"
  docker-compose -f ${compose_file} up -d
  echo "${compose_file}"
}

setup_opensearch() {
  local tag=$1
  local major_version=${tag%%.*}
  local compose_file
  compose_file="docker-compose/opensearch/v${major_version}.yml"
  docker-compose -f ${compose_file} up -d
  echo "${compose_file}"
}

wait_for_storage() {
  local distro=$1
  local url=$2
  local compose_file=$3
  local params=(
    --silent
    --output
    /dev/null
    --write-out
    "%{http_code}"
  )
  local max_attempts=60
  local attempt=0
  echo "Waiting for ${distro} to be available at ${url}..."
  until [[ "$(curl "${params[@]}" "${url}")" == "200" ]] || (( attempt >= max_attempts )); do
    echo "Attempt $(( attempt + 1 )): ${distro} is not yet available at ${url}..."
    sleep 10
    attempt=$(( attempt + 1 ))
  done

  if [[ "$(curl "${params[@]}" "${url}")" != "200" ]]; then
    echo "ERROR: ${distro} is not ready at ${url} after $(( attempt * 10 )) seconds"
    docker-compose -f ${compose_file} logs
    docker-compose -f ${compose_file} down
    db_is_up=0
  else
    echo "SUCCESS: ${distro} is available at ${url}"
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
      compose_file=$(setup_es "${version}")
    elif [ "${distro}" == "opensearch" ]; then
      compose_file=$(setup_opensearch "${version}")
    else
      echo "Unknown distribution $distro. Valid options are opensearch or elasticsearch"
      usage
    fi
    wait_for_storage "${distro}" "http://localhost:9200" "${compose_file}"
    if [ ${db_is_up} = "1" ]; then
      break
    fi
  done
  if [ ${db_is_up} = "1" ]; then
  # shellcheck disable=SC2064
    trap "teardown_storage ${compose_file}" EXIT
  else
    echo "ERROR: unable to start ${distro}"
    exit 1
  fi
}

teardown_storage() {
  local compose_file=$1
  docker-compose -f ${compose_file} down
}

main() {
  check_arg "$@"
  local distro=$1
  local es_version=$2
  local j_version=$2

  bring_up_storage "${distro}" "${es_version}"

  if [[ "${j_version}" == "v2" ]]; then
    STORAGE=${distro} SPAN_STORAGE_TYPE=${distro} make jaeger-v2-storage-integration-test
  else
    STORAGE=${distro} make storage-integration-test
    make index-cleaner-integration-test
    make index-rollover-integration-test
  fi
}

main "$@"