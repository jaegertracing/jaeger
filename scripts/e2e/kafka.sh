#!/bin/bash

# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

set -euf -o pipefail

compose_file=""
# Elasticsearch is started alongside Kafka so the synchronous-write ingester e2e
# (TestKafkaStorage_SyncElasticsearch) can exercise Collector -> Kafka -> Ingester
# -> Elasticsearch end-to-end (RFC 0007). The plain TestKafkaStorage still uses the
# in-process memory backend and ignores it.
es_compose_file="docker-compose/elasticsearch/v8/docker-compose.yml"
kafka_version="v3"
manage_kafka="true"
success="false"

usage() {
  echo "Usage: $0 [-S] [-v <kafka_version>]"
  echo "  -S: 'no storage' - do not start or stop Kafka container (useful for local testing)"
  echo "  -v: kafka major version (3.x); default: 3.x"
  exit 1
}

parse_args() {
  while getopts "v:Sh" opt; do
    case "${opt}" in
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
  compose_file="docker-compose/kafka/${kafka_version}/docker-compose.yml"
}

setup_kafka() {
  echo "Starting Kafka and Elasticsearch using Docker Compose..."
  bash scripts/utils/retry.sh docker compose -f "${compose_file}" pull kafka
  bash scripts/utils/retry.sh docker compose -f "${es_compose_file}" pull elasticsearch
  docker compose -f "${compose_file}" up -d kafka
  docker compose -f "${es_compose_file}" up -d elasticsearch
}

dump_logs() {
  echo "::group::🚧 🚧 🚧 Kafka logs"
  docker compose -f "${compose_file}" logs
  echo "::endgroup::"
  echo "::group::🚧 🚧 🚧 Elasticsearch logs"
  docker compose -f "${es_compose_file}" logs
  echo "::endgroup::"
}

teardown_kafka() {
   if [[ "$success" == "false" ]]; then
    dump_logs
  fi
  echo "Stopping Kafka and Elasticsearch..."
  docker compose -f "${compose_file}" down
  docker compose -f "${es_compose_file}" down
}

is_kafka_ready() {
  docker compose -f "${compose_file}" \
    exec kafka /opt/kafka/bin/kafka-topics.sh \
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

wait_for_es() {
  local timeout=180
  local interval=5
  local end_time=$((SECONDS + timeout))

  while [ $SECONDS -lt $end_time ]; do
    if curl -s http://localhost:9200 >/dev/null 2>&1; then
      return
    fi
    echo "Elasticsearch not ready, waiting ${interval} seconds"
    sleep $interval
  done

  echo "Timed out waiting for Elasticsearch to start"
  exit 1
}

run_integration_test() {
  export STORAGE=kafka
  make jaeger-v2-storage-integration-test
}

main() {
  parse_args "$@"

  echo "Executing Kafka integration test."
  echo "Kafka version ${kafka_version}."
  set -x

  if [[ "$manage_kafka" == "true" ]]; then
    setup_kafka
    trap 'teardown_kafka' EXIT
  fi
  wait_for_kafka
  wait_for_es

  run_integration_test

  success="true"
}

main "$@"
