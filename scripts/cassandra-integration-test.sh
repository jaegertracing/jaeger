#!/bin/bash

set -uxf -o pipefail

usage() {
  echo $"Usage: $0 <cassandra_version> <schema_version>"
  exit 1
}

check_arg() {
  if [ ! $# -eq 2 ]; then
    echo "ERROR: need exactly two arguments, <cassandra_version> <schema_version>"
    usage
  fi
}

setup_cassandra() {
  local tag=$1
  local image=cassandra
  local params=(
    --rm
    --detach
    --publish 9042:9042
    --publish 9160:9160
  )
  local cid
  cid=$(docker run "${params[@]}" "${image}:${tag}")
  echo "${cid}"
}

teardown_cassandra() {
  local cid=$1
  docker kill "${cid}"
  exit "${exit_status}"
}

apply_schema() {
  local image=cassandra-schema
  local schema_dir=plugin/storage/cassandra/
  local schema_version=$1
  local params=(
    --rm
    --env CQLSH_HOST=localhost
    --env CQLSH_PORT=9042
    --env "TEMPLATE=/cassandra-schema/${schema_version}.cql.tmpl"
    --network host
  )
  docker build -t ${image} ${schema_dir}
  docker run "${params[@]}" ${image}
}

run_integration_test() {
  local version=$1
  local schema_version=$2
  local cid
  cid=$(setup_cassandra "${version}")
  apply_schema "$2"
  STORAGE=cassandra make storage-integration-test
  exit_status=$?
  # shellcheck disable=SC2064
  trap "teardown_cassandra ${cid}" EXIT
}

main() {
  check_arg "$@"

  echo "Executing integration test for $1 with schema $2.cql.tmpl"
  run_integration_test "$1" "$2"
}

main "$@"
