#!/bin/bash

# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

set -euf -o pipefail

compose_file="docker-compose/kafka-integration-test/docker-compose.yml"
echo "docker_compose_file=${compose_file}" >> "${GITHUB_OUTPUT:-/dev/null}"

jaeger_version=""
manage_kafka="true"

print_help() {
  echo "Usage: $0 [-K] -j <jaeger_version>"
  echo "  -K: do not start or stop Kafka container (useful for local testing)"
  echo "  -j: major version of Jaeger to test (v1|v2)"
  exit 1
}

parse_args() {
  while getopts "j:Kh" opt; do
    case "${opt}" in
    j)
      jaeger_version=${OPTARG}
      ;;
    K)
      manage_kafka="false"
      ;;
    *)
      print_help
      ;;
    esac
  done
  if [ "$jaeger_version" != "v1" ] && [ "$jaeger_version" != "v2" ]; then
    echo "Error: Invalid Jaeger version. Valid options are v1 or v2"
    print_help
  fi
}

setup_kafka() {
  echo "Starting Kafka using Docker Compose..."
  docker compose -f "${compose_file}" up -d kafka
}

teardown_kafka() {
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
    print_help
  fi
}

main() {
  parse_args "$@"

  echo "Executing Kafka integration test for version $2"
  set -x

  if [[ "$manage_kafka" == "true" ]]; then
    setup_kafka
    trap 'teardown_kafka' EXIT
  fi
  wait_for_kafka

  run_integration_test
}

main "$@"
