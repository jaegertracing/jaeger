#!/bin/bash

# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

set -euf -o pipefail

compose_file=docker-compose/adaptive-sampling/docker-compose.yml

set -x

timeout=600
end_time=$((SECONDS + timeout))
success="false"

threshold=0.5

dump_logs() {
    echo "::group:: docker logs"
    docker compose -f $compose_file logs
    echo "::endgroup::"
}

teardown_services() {
    if [[ "$success" == "false" ]]; then
        dump_logs
    fi
    docker compose -f $compose_file down
}

check_service_health() {
    local service_name=$1
    local url=$2
    echo "Checking health of service: $service_name at $url"

    local wait_seconds=3
    local curl_params=(
        --silent
        --output
        /dev/null
        --write-out
        "%{http_code}"
    )
    while [ $SECONDS -lt $end_time ]; do
        if [[ "$(curl "${curl_params[@]}" "${url}")" == "200" ]]; then
            echo "✅ $service_name is healthy"
            return 0
        fi
        echo "Waiting for $service_name to be healthy..."
        sleep $wait_seconds
    done

    echo "❌ ERROR: $service_name did not become healthy in time"
    return 1
}

wait_for_services() {
    echo "Waiting for services to be up and running..."
    check_service_health "Jaeger" "http://localhost:16686"
}

check_tracegen_probability() {
    local url="http://localhost:5778/api/sampling?service=tracegen"
    response=$(curl -s "$url")
    probability=$(echo "$response" | jq .operationSampling | jq -r '.perOperationStrategies[] | select(.operation=="lets-go")' | jq .probabilisticSampling.samplingRate)
    if [ -n "$probability" ]; then
        if (( $(echo "$probability < $threshold" |bc -l) )); then
            return 0
        fi
    fi
    return 1
}

check_adaptive_sampling() {
    local wait_seconds=10
    while [ $SECONDS -lt $end_time ]; do
        if check_tracegen_probability; then
            success="true"
            break
        fi
        sleep $wait_seconds
    done
      if [[ "$success" == "false" ]]; then
        echo "❌ ERROR: Adaptive sampling probability did not drop below $threshold."
        exit 1
      else
        echo "✅ Adaptive sampling probability integration test passed"
      fi
}

main() {
    (cd docker-compose/adaptive-sampling && make build && make dev DOCKER_COMPOSE_ARGS="-d")
    wait_for_services
    check_adaptive_sampling
}

trap teardown_services EXIT INT

main
