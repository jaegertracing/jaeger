#!/bin/bash

# Copyright (c) 2025 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

export CLICKHOUSE_USERNAME="default"
export CLICKHOUSE_PASSWORD="default"
compose_file="docker-compose/clickhouse/docker-compose.yml"

setup_clickhouse() {
  docker compose -f "$compose_file" up -d
}

wait_for_clickhouse() {
  echo "Waiting for ClickHouse to start..."
  while ! docker exec clickhouse clickhouse-client --host=127.0.0.1 --port=9000 \
      --user="$CLICKHOUSE_USERNAME" \
      --password="$CLICKHOUSE_PASSWORD" \
      --query="SELECT 1" > /dev/null 2>&1; do
    echo "Waiting for ClickHouse to be ready..."
    sleep 2
  done
  echo "ClickHouse is ready!"
}

dump_logs() {
  echo "::group::🚧 🚧 🚧 clickhouse logs"
  docker compose -f "${compose_file}" logs
  echo "::endgroup::"
}

run_integration_test() {
  # Wait for the schema to be applied before starting the integration test
  echo "Schema applied, starting integration tests..."
  STORAGE=clickhouse make storage-integration-test
}

main() {
  dump_logs
  setup_clickhouse
  wait_for_clickhouse
  run_integration_test
}

main