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
    echo "ERROR: need exactly three arguments, <elasticsearch|opensearch> <image> <jaeger-version>"
    usage
  fi
}

# start the elasticsearch/opensearch container
setup_db() {
  local compose_file=$1
  docker compose -f "${compose_file}" up -d
  echo "docker_compose_file=${compose_file}" >> "${GITHUB_OUTPUT:-/dev/null}"
}

# check if the storage is up and running
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
    attempt=$(( attempt + 1 ))
    echo "Attempt: ${attempt} ${distro} is not yet available at ${url}..."
    sleep 10
  done

  # if after all the attempts the storage is not accessible, terminate it and exit
  if [[ "$(curl "${params[@]}" "${url}")" != "200" ]]; then
    echo "ERROR: ${distro} is not ready at ${url} after $(( attempt * 10 )) seconds"
    echo "::group::${distro} logs"
    docker compose -f "${compose_file}" logs
    echo "::endgroup::"
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
  local compose_file="docker-compose/${distro}/v${major_version}/docker-compose.yml"

  echo "starting ${distro} ${major_version}"
  for retry in 1 2 3
  do
    echo "attempt $retry"
    if [ "${distro}" = "elasticsearch" ] || [ "${distro}" = "opensearch" ]; then
        setup_db "${compose_file}"
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

# terminate the elasticsearch/opensearch container
teardown_storage() {
  local compose_file=$1
  docker compose -f "${compose_file}" down
}

build_local_img(){
    local LOCAL_FLAG=''
    local platforms="linux/amd64"
    # Loop through each platform (separated by commas)
    for platform in $(echo "$platforms" | tr ',' ' '); do
      # Extract the architecture from the platform string
      arch=${platform##*/}  # Remove everything before the last slash
      make "build-binaries-$arch"
    done
    export GITHUB_SHA="local-test"
    make create-baseimg
    bash scripts/build-upload-a-docker-image.sh ${LOCAL_FLAG} -b -c jaeger-es-index-cleaner -d cmd/es-index-cleaner -p "${platforms}" -t release -l Y
}

main() {
  check_arg "$@"
  local distro=$1
  local es_version=$2
  local j_version=$2

  bring_up_storage "${distro}" "${es_version}"
  build_local_img
  if [[ "${j_version}" == "v2" ]]; then
    STORAGE=${distro} SPAN_STORAGE_TYPE=${distro} make jaeger-v2-storage-integration-test
  else
    STORAGE=${distro} make storage-integration-test
    make index-cleaner-integration-test
    make index-rollover-integration-test
  fi
}

main "$@"
