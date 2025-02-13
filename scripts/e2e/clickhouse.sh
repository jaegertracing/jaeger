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

apply_schema() {
  docker exec -i clickhouse clickhouse-client --host=127.0.0.1 --port=9000 \
    --user="$CLICKHOUSE_USERNAME" \
    --password="$CLICKHOUSE_PASSWORD" \
    --queries-file="docker-entrypoint-initdb.d/init.sql"
}

dump_logs() {
  echo "::group::🚧 🚧 🚧 clickhouse logs"
  docker compose -f "${compose_file}" logs
  echo "::endgroup::"
}

main() {
  dump_logs
  setup_clickhouse
  wait_for_clickhouse
  apply_schema
}

main