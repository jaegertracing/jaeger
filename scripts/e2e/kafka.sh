#!/bin/bash

# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

set -euf -o pipefail

compose_file=""
jaeger_version="v1"
kafka_version="v3"
manage_kafka="true"
success="false"

usage() {
  echo "Usage: $0 [-S] [-j <jaeger_version>] [-v <kafka_version>]"
  echo "  -S: 'no storage' - do not start or stop Kafka container (useful for local testing)"
  echo "  -j: major version of Jaeger to test (v1|v2); default: v2"
  echo "  -v: kafka major version (3.x); default: 3.x"
  exit 1
}

parse_args() {
  while getopts "j:v:Sh" opt; do
    case "${opt}" in
    j)
      jaeger_version=${OPTARG}
      ;;
    v)
      case ${OPTARG} in
      3.x)
        kafka_version="v3"
        ;;
      2.x)
        kafka_version="v2"
        ;;
      *)
        echo "Error: Invalid Kafka version. Valid options are 3.x or 2.x"
        usage
        ;;
      esac
      ;;
    S)
      manage_kafka="false"
      ;;
    *)
      usage
      ;;
    esac
  done
  if [[ "$jaeger_version" != "v1" && "$jaeger_version" != "v2" ]]; then
    echo "Error: Invalid Jaeger version. Valid options are v1 or v2"
    usage
  fi
  compose_file="docker-compose/kafka/${kafka_version}/docker-compose.yml"
}

setup_kafka() {
  echo "Starting Kafka using Docker Compose..."
  docker compose -f "${compose_file}" up -d kafka
}

dump_logs() {
  echo "::group::🚧 🚧 🚧 Kafka logs"
  docker compose -f "${compose_file}" logs
  echo "::endgroup::"
}

teardown_kafka() {
   if [[ "$success" == "false" ]]; then
    dump_logs
  fi
  echo "Stopping Kafka..."
  docker compose -f "${compose_file}" down
}

is_kafka_ready() {
  docker compose -f "${compose_file}" \
    exec kafka /opt/bitnami/kafka/bin/kafka-topics.sh \
    --list \
    --bootstrap-server localhost:9092 \
    >/dev/null 2>&1
}

wait_for_kafka() {
  local timeout=180
  local interval=5
  local end_time=$((SECONDS + timeout))

  while [ $SECONDS -lt $end_time ]; do
    if is_kafka_ready; then
      return
    fi
    echo "Kafka broker not ready, waiting ${interval} seconds"
    sleep $interval
  done

  echo "Timed out waiting for Kafka to start"
  exit 1
}

run_integration_test() {
  export STORAGE=kafka
  if [ "${jaeger_version}" = "v1" ]; then
    make storage-integration-test
  elif [ "${jaeger_version}" = "v2" ]; then
    make jaeger-v2-storage-integration-test
  else
    echo "Unknown Jaeger version ${jaeger_version}."
    usage
  fi
}

main() {
  parse_args "$@"

  echo "Executing Kafka integration test."
  echo "Kafka version ${kafka_version}."
  echo "Jaeger version ${jaeger_version}."
  set -x

  if [[ "$manage_kafka" == "true" ]]; then
    setup_kafka
    trap 'teardown_kafka' EXIT
  fi
  wait_for_kafka

  run_integration_test

  success="true"
}

main "$@"