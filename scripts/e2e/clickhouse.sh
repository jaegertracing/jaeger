#!/bin/bash

# Copyright (c) 2025 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

export CLICKHOUSE_USERNAME="default"
export CLICKHOUSE_PASSWORD="default"
compose_file="docker-compose/clickhouse/docker-compose.yml"
SKIP_APPLY_SCHEMA=${SKIP_APPLY_SCHEMA:-"false"}
export CLICKHOUSE_CREATE_SCHEMA=${SKIP_APPLY_SCHEMA}

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

apply_schema() {
  local image=clickhouse-schema
  local schema_dir=internal/storage/v2/clickhouse/
  local params=(
    --rm
    --env "CLICKHOUSE_HOST=localhost"
    --env "QUERY_FILE=/clickhouse-schema/init.tmpl"
    --env "CLICKHOUSE_USERNAME=${CLICKHOUSE_USERNAME}"
    --env "CLICKHOUSE_PASSWORD=${CLICKHOUSE_PASSWORD}"
    --network host
  )
  docker build -t ${image} ${schema_dir}
  docker run "${params[@]}" ${image}

  if ! docker run "${params[@]}" ${image}; then
    echo "Schema application failed. Exiting..."
    exit 1
  fi
}

dump_logs() {
  echo "::group::🚧 🚧 🚧 clickhouse logs"
  docker compose -f "${compose_file}" logs
  echo "::endgroup::"
}

run_integration_test() {
  apply_schema
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