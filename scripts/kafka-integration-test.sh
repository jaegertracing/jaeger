#!/bin/bash

set -euxf -o pipefail

usage() {
  echo $"Usage: $0 -k <test_version>"
  exit 1
}

check_args() {
  if [ $# -ne 2 ]; then
    echo "ERROR: need exactly two arguments, -k and <test_version>"
    usage
  fi
}

setup_kafka() {
  local compose_file=$1
  echo "Starting Kafka using Docker Compose..."
  docker compose -f "${compose_file}" up -d kafka
  echo "docker_compose_file=${compose_file}" >> "${GITHUB_OUTPUT:-/dev/null}"
}

teardown_kafka() {
  local compose_file=$1
  echo "Stopping Kafka..."
  docker compose -f "${compose_file}" down
}

is_kafka_ready() {
  local compose_file=$1
  docker compose -f "${compose_file}" \
    exec kafka /opt/bitnami/kafka/bin/kafka-topics.sh \
    --list \
    --bootstrap-server localhost:9092 \
    >/dev/null 2>&1
}

wait_for_kafka() {
  local compose_file=$1
  local timeout=180
  local interval=5
  local end_time=$((SECONDS + timeout))

  while [ $SECONDS -lt $end_time ]; do
    if is_kafka_ready "$compose_file"; then
      echo "Kafka is ready."
      return
    fi
    echo "Kafka broker not ready, waiting ${interval} seconds"
    sleep $interval
  done

  echo "Timed out waiting for Kafka to start"
  exit 1
}

run_integration_test() {
  local version=$1

  if [ "${version}" = "v1" ]; then
    STORAGE=kafka make storage-integration-test
    exit_status=$?
  elif [ "${version}" = "v2" ]; then
    STORAGE=kafka make jaeger-v2-storage-integration-test
    exit_status=$?
  else
    echo "Unknown test version ${version}. Valid options are v1 or v2"
    exit 1
  fi

  if [ $exit_status -ne 0 ]; then
    echo "Integration tests failed."
    exit $exit_status
  fi
}

main() {
  check_args "$@"

  local compose_file="docker-compose/kafka-integration-test/docker-compose.yml"

  echo "Executing Kafka integration test for version $2"

  if [ "$1" == "-k" ]; then
    setup_kafka "${compose_file}"
    wait_for_kafka "${compose_file}"
    trap "teardown_kafka ${compose_file}" EXIT
  fi

  run_integration_test "$2"
}

main "$@"
