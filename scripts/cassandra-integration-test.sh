#!/bin/bash

set -uxf -o pipefail

usage() {
  echo $"Usage: $0 <cassandra_version> <schema_version>"
  exit 1
}

check_arg() {
  if [ ! $# -eq 3 ]; then
    echo "ERROR: need exactly three arguments, <cassandra_version> <schema_version> <jaeger_version>"
    usage
  fi
}

setup_cassandra() {
  local tag=$1
  local image=cassandra
  local params=(
    --detach
    --publish 9042:9042
    --publish 9160:9160
  )
  local cid
  cid=$(docker run "${params[@]}" "${image}:${tag}")
  echo "cid=${cid}" >> "$GITHUB_OUTPUT"
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
  local keyspace=$2
  local params=(
    --rm
    --env CQLSH_HOST=localhost
    --env CQLSH_PORT=9042
    --env "TEMPLATE=/cassandra-schema/${schema_version}.cql.tmpl"
    --env "KEYSPACE=${keyspace}"
    --network host
  )
  docker build -t ${image} ${schema_dir}
  docker run "${params[@]}" ${image}
}

run_integration_test() {
  local version=$1
  local schema_version=$2
  local jaegerVersion=$3
  local primaryKeyspace="jaeger_v1_dc1"
  local archiveKeyspace="jaeger_v1_dc1_archive"

  local cid
  cid=$(setup_cassandra "${version}")

  apply_schema "$schema_version" "$primaryKeyspace"
  apply_schema "$schema_version" "$archiveKeyspace"

  if [ "${jaegerVersion}" = "v1" ]; then
    STORAGE=cassandra make storage-integration-test
    exit_status=$?
  elif [ "${jaegerVersion}" == "v2" ]; then
    STORAGE=cassandra make jaeger-v2-storage-integration-test
    exit_status=$?
  else
    echo "Unknown jaeger version $jaegerVersion. Valid options are v1 or v2"
    exit 1
  fi

  # shellcheck disable=SC2064
  trap "teardown_cassandra ${cid}" EXIT
}


main() {
  check_arg "$@"

  echo "Executing integration test for $1 with schema $2.cql.tmpl"
  run_integration_test "$1" "$2" "$3"
}

main "$@"
