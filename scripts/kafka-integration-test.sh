#!/bin/bash

set -e

export STORAGE=kafka
compose_file="docker-compose/kafka-integration-test/docker-compose.yml"

usage() {
  echo $"Usage: $0 -k <jaeger_version>"
  exit 1
}

check_args() {
  if [ $# -ne 2 ]; then
    echo "ERROR: need exactly two arguments, -k and <jaeger_version>"
    usage
  fi
}

setup_kafka() {
  echo "Starting Kafka using Docker Compose..."
  docker compose -f "${compose_file}" up -d kafka
  echo "docker_compose_file=${compose_file}" >> "${GITHUB_OUTPUT:-/dev/null}"
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
      break
    fi
    echo "Kafka broker not ready, waiting ${interval} seconds"
    sleep $interval
  done

  if ! is_kafka_ready; then
    echo "Timed out waiting for Kafka to start"
    exit 1
  fi
}

run_integration_test() {
  local version=$1

  if [ "${version}" = "v1" ]; then
    STORAGE=kafka make storage-integration-test
  elif [ "${version}" = "v2" ]; then
    STORAGE=kafka make jaeger-v2-storage-integration-test
  else
    echo "Unknown test version ${version}. Valid options are v1 or v2"
    exit 1
  fi
}

main() {
  check_args "$@"

  echo "Executing Kafka integration test for version $2"

  if [ "$1" == "-k" ]; then
    setup_kafka
    trap 'teardown_kafka' EXIT
    wait_for_kafka
  fi

  run_integration_test "$2"
}

main "$@"
