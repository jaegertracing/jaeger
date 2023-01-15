#!/bin/bash

PS4='T$(date "+%H:%M:%S") '
set -euxf -o pipefail

# use global variables to reflect status of db
db_is_up=
db_cid=

exit_trap_command="echo executing exit traps"
function cleanup {
    eval "$exit_trap_command"
}
trap cleanup EXIT

function add_exit_trap {
    local to_add=$1
    exit_trap_command="$exit_trap_command; $to_add"
}

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
    --env "xpack.monitoring.enabled=false"
  )
  local cid=$(docker run ${params[@]} ${image}:${tag})
  echo ${cid}
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
  local cid=$(docker run ${params[@]} ${image}:${tag})
  echo ${cid}
}

# bearer token propagaton test uses real query service, but starts a fake DB at 19200
setup_query() {
  local distro=$1
  local os=$(go env GOOS)
  local arch=$(go env GOARCH)
  local params=(
    --es.tls.enabled=false
    --es.version=7
    --es.server-urls=http://127.0.0.1:19200
    --query.bearer-token-propagation=true
  )
  SPAN_STORAGE_TYPE=${distro} ./cmd/query/query-${os}-${arch} ${params[@]}
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
    ''%{http_code}''
  )
  local counter=0
  local max_counter=60
  while [[ "$(curl ${params[@]} ${url})" != "200" && ${counter} -le ${max_counter} ]]; do
    docker inspect ${cid} | jq '.[].State'
    echo "waiting for ${url} to be up..."
    sleep 10
    counter=$((counter+1))
  done
  # after the loop, do final verification and set status as global var
  if [[ "$(curl ${params[@]} ${url})" != "200" ]]; then
    echo "ERROR: ${distro} is not ready"
    docker logs ${cid}
    docker kill ${cid}
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
  for retry in 1 2 3
  do
    if [ ${distro} = "elasticsearch" ]; then
      cid=$(setup_es ${version})
    elif [ ${distro} == "opensearch" ]; then
      cid=$(setup_opensearch ${version})
    else
      echo "Unknown distribution $distro. Valid options are opensearch or elasticsearch"
      usage
    fi
    wait_for_storage ${distro} "http://localhost:9200" ${cid}
    if [ ${db_is_up} = "1" ]; then
      break
    else
      echo "ERROR: unable to start ${distro}"
      exit 1
    fi
  done
  db_cid=${cid}
}

teardown_storage() {
  local cid=$1
  docker kill ${cid}
}

teardown_query() {
  local pid=$1
  kill -9 ${pid}
}

build_query() {
  make build-crossdock-ui-placeholder
  make build-query
}

run_integration_test() {
  local distro=$1
  STORAGE=${distro} make storage-integration-test
  make index-cleaner-integration-test
  make index-rollover-integration-test
}

run_token_propagation_test() {
  local distro=$1
  build_query
  setup_query ${distro} &
  local pid=$!
  add_exit_trap "teardown_query ${pid}"
  make token-propagation-integration-test
}

main() {
  check_arg "$@"

  echo "Preparing $1 $2"
  bring_up_storage "$1" "$2"
  add_exit_trap "teardown_storage ${db_cid}"

  echo "Executing main integration tests"
  run_integration_test "$1"

  echo "Executing token propagation test"
  run_token_propagation_test "$1"
}

main "$@"
