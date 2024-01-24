#!/bin/bash

set -euxf -o pipefail

bring_up_kafka() {
  local tag=$1
  local image="bitnami/kafka"

  local cid
  cid=$(docker run --detach \
    --name kafka \
    --publish 9092:9092 \
    --env KAFKA_CFG_NODE_ID=0 \
    --env KAFKA_CFG_PROCESS_ROLES=controller,broker \
    --env KAFKA_CFG_CONTROLLER_QUORUM_VOTERS=0@localhost:9093 \
    --env KAFKA_CFG_LISTENERS=PLAINTEXT://:9092,CONTROLLER://:9093 \
    --env KAFKA_CFG_ADVERTISED_LISTENERS=PLAINTEXT://localhost:9092 \
    --env KAFKA_CFG_LISTENER_SECURITY_PROTOCOL_MAP=CONTROLLER:PLAINTEXT,PLAINTEXT:PLAINTEXT \
    --env KAFKA_CFG_CONTROLLER_LISTENER_NAMES=CONTROLLER \
    --env KAFKA_CFG_INTER_BROKER_LISTENER_NAME=PLAINTEXT \
    "${image}:${tag}")

  wait_for_storage "9092" "${cid}"

  echo "${cid}"
}

bring_up_remote_storage() {
  local tag=$1
  local image="jaegertracing/jaeger-remote-storage"

  local cid
  cid=$(docker run --detach \
    --publish 17271:17271 \
    --publish 17270:17270 \
    --env SPAN_STORAGE_TYPE=memory \
    "${image}:${tag}")

  wait_for_storage "17271" "${cid}"

  echo "${cid}"
}

teardown_storage() {
  for cid in "$@"
  do
    docker kill "${cid}"
  done
}

wait_for_storage() {
  local port=$1
  local cid=$2

  local counter=0
  local max_counter=30
  local interval=10
  while [[ $(! nc -z localhost "${port}") && ${counter} -lt ${max_counter} ]]; do
    docker inspect "${cid}" | jq '.[].State'
    echo "waiting for localhost:${port} to be up..."
    sleep "${interval}"
    counter=$((counter+1))
  done

  if ! nc -z localhost "${port}"; then
    docker inspect "${cid}" | jq '.[].State'
    echo "timed out waiting storage to start"
    exit 1
  fi
}

main() {
  local kafka_version="${1:-"latest"}"
  local remote_storage_version="${2:-"latest"}"

  kafka_cid=$(bring_up_kafka "${kafka_version}")
  remote_storage_cid=$(bring_up_remote_storage "${remote_storage_version}")

  trap 'teardown_storage "${kafka_cid}" "${remote_storage_cid}"' EXIT

  STORAGE="otel_kafka" make otel-integration-test
}

main "$@"
