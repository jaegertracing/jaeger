#!/bin/bash

# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

set -euxf -o pipefail

success="false"
timeout=600
end_time=$((SECONDS + timeout))
container_names=("clickhouse")
schema_file=""

setup_clickhouse() {
    echo "Starting ClickHouse with $compose_file"
    bash scripts/utils/retry.sh docker compose -f "$compose_file" pull
    docker compose -f "$compose_file" up -d
}

healthcheck_clickhouse() {
    local wait_seconds=10

    for container_name in "${container_names[@]}"; do
        while [ $SECONDS -lt $end_time ]; do
            status=$(docker inspect -f '{{ .State.Health.Status }}' "${container_name}")
            if [[ ${status} == "healthy" ]]; then
                echo "✅ $container_name is healthy"
                break
            fi
            echo "Waiting for $container_name to be healthy. Current status: $status"
            sleep $wait_seconds
        done
        if [[ ${status} != "healthy" ]]; then
            echo "❌ ERROR: $container_name did not become healthy in time"
            exit 1
        fi
    done
}

apply_schema() {
    if [[ -z "$schema_file" ]]; then
        return
    fi
    # The initiator sends every ON CLUSTER DDL to both servers.
    docker compose -f "$compose_file" exec -T clickhouse-01 clickhouse-client \
        --user default --password password --multiquery < "$schema_file"
    sleep 2
}

dump_logs() {
    local compose_file=$1
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
    local storage_test=$1
    local clickhouse_version=$2
    local cluster_mode=$3
    if [[ "${cluster_mode}" == "multi-node" ]]; then
        compose_file="docker-compose/clickhouse/docker-compose-multi-node-${clickhouse_version}.yml"
        container_names=("clickhouse-keeper" "clickhouse-01" "clickhouse-02")
        schema_file="docker-compose/clickhouse/schema-multi-node.sql"
        export CLICKHOUSE_MULTI_NODE="true"
    else
        compose_file="docker-compose/clickhouse/docker-compose-${clickhouse_version}.yml"
        export CLICKHOUSE_MULTI_NODE="false"
    fi
    if [ ! -f "$compose_file" ]; then
        echo "ERROR: Compose file not found: $compose_file"
        exit 1
    fi
    setup_clickhouse
    trap teardown_clickhouse EXIT
    healthcheck_clickhouse
    apply_schema
    if [[ "${storage_test}" == "e2e" ]]; then
        STORAGE=clickhouse make jaeger-v2-storage-integration-test
    elif [[ "${storage_test}" == "compat" ]]; then
        STORAGE=clickhouse make jaeger-v2-backward-compatibility-test
    elif [[ "${storage_test}" == "direct" ]]; then
        STORAGE=clickhouse make storage-integration-test
    else
        echo "ERROR: Invalid argument value storage_test=${storage_test}, expecting direct, e2e or compat"
        exit 1
    fi
    success="true"
}

main() {
    if [ $# -lt 2 ] || [ $# -gt 3 ]; then
        echo "Usage: $0 <e2e|direct|compat> <lts-march|lts-august> [single-node|multi-node]"
        exit 1
    fi
    local storage_test=$1
    local clickhouse_version=$2
    local cluster_mode=${3:-single-node}
    if [[ "${cluster_mode}" != "single-node" && "${cluster_mode}" != "multi-node" ]]; then
        echo "ERROR: Invalid argument value cluster_mode=${cluster_mode}, expecting single-node or multi-node"
        exit 1
    fi
    echo "Executing ClickHouse ${storage_test} integration tests (${clickhouse_version}, ${cluster_mode})"
    run_integration_test "${storage_test}" "${clickhouse_version}" "${cluster_mode}"
}

main "$@"
