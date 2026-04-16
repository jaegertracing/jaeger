#!/bin/bash

# Copyright (c) 2025 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

set -euxf -o pipefail

success="false"
timeout=600
end_time=$((SECONDS + timeout))
compose_file="docker-compose/clickhouse/docker-compose.yml"
container_name="clickhouse"

setup_clickhouse() {
    echo "Starting ClickHouse with $compose_file"
    docker compose -f "$compose_file" up -d
}

healthcheck_clickhouse() {
    local wait_seconds=10

    while [ $SECONDS -lt $end_time ]; do
        status=$(docker inspect -f '{{ .State.Health.Status }}' "${container_name}")
        if [[ ${status} == "healthy" ]]; then
            echo "✅ $container_name is healthy"
            return 0
        fi
        echo "Waiting for $container_name to be healthy. Current status: $status"
        sleep $wait_seconds
    done

    echo "❌ ERROR: $container_name did not become healthy in time"
    exit 1
}

dump_logs() {
    echo "::group::🚧 🚧 🚧 Clickhouse logs"
    docker compose -f "${compose_file}" logs
    echo "::endgroup::"
}

teardown_clickhouse() {
    if [[ "$success" == "false" ]]; then
        dump_logs "${compose_file}"
    fi
    docker compose -f "$compose_file" down
}

run_integration_test() {
    local storage_test=${1:-e2e}
    setup_clickhouse
    trap teardown_clickhouse EXIT
    healthcheck_clickhouse
    if [[ "${storage_test}" == "e2e" ]]; then
        STORAGE=clickhouse make jaeger-v2-storage-integration-test
    elif [[ "${storage_test}" == "direct" ]]; then
        STORAGE=clickhouse make storage-integration-test
    else
        echo "ERROR: Invalid argument value storage_test=${storage_test}, expecting direct or e2e"
        exit 1
    fi
    success="true"
}

main() {
    local storage_test=${1:-e2e}
    echo "Executing ClickHouse ${storage_test} integration tests"
    run_integration_test "${storage_test}"
}

main "$@"