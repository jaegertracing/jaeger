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

setup_db() {
  local distro=$1
  local compose_file=$2
  docker compose -f "${compose_file}" up -d
  local cid 
  cid=$(docker compose -f "${compose_file}" ps -q "${distro}")
  echo "cid=${cid}" >> "$GITHUB_OUTPUT"
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
  echo "::group::Waiting for ${distro} availability"
  until [[ "$(curl "${params[@]}" "${url}")" == "200" ]] || (( attempt >= max_attempts )); do
    attempt=$(( attempt + 1 ))
    echo "Attempt: ${attempt} ${distro} is not yet available at ${url}..."
    sleep 10
  done
  echo "::endgroup::"

  if [[ "$(curl "${params[@]}" "${url}")" != "200" ]]; then
    echo "ERROR: ${distro} is not ready at ${url} after $(( attempt * 10 )) seconds"
    docker compose -f "${compose_file}" logs
    docker compose -f "${compose_file}" down
    db_is_up=0
  else
    echo "SUCCESS: ${distro} is available at ${url}"
    db_is_up=1
  fi
}

bring_up_storage() {
  local distro=$1
  local version=$2
  local major_version=${version%%.*}
  local compose_file="docker-compose/${distro}/v${major_version}.yml"

  echo "starting ${distro} ${version}"
  for retry in 1 2 3
  do
    echo "attempt $retry"
    if [ "${distro}" = "elasticsearch" ] || [ "${distro}" = "opensearch" ]; then
        setup_db "${distro}" "${compose_file}"
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
  docker compose -f "${compose_file}" down
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