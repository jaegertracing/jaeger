#!/bin/bash

# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

set -euxf -o pipefail

success="false"
timeout=180
compose_file="docker-compose/jaeger-docker-compose.yml"

setup_services() {
    echo "Starting Jaeger with adaptive sampling and Cassandra"
    docker compose -f "$compose_file" up -d
}

wait_for_cassandra() {
    local end_time=$((SECONDS + timeout))
    while [ $SECONDS -lt $end_time ]; do
        if docker compose -f "$compose_file" ps | grep cassandra | grep -q "healthy"; then
            echo "Cassandra is healthy"
            return 0
        fi
        echo "Waiting for Cassandra..."
        sleep 3
    done
    echo "ERROR: Cassandra did not become healthy"
    exit 1
}

verify_services() {
    echo "Verifying all containers are running"
    if docker compose -f "$compose_file" ps | grep -i "restarting"; then
        echo "ERROR: Some containers are restarting"
        return 1
    fi
    
    if docker compose -f "$compose_file" logs hotrod | grep -q "connection refused"; then
        echo "ERROR: HotROD has connection errors"
        return 1
    fi
    echo "✅ All services running correctly"
}

generate_traces() {
    echo "Generating test traces via HotROD"
    for i in {1..20}; do
        curl -s "http://localhost:8080/dispatch?customer=123&nonse=$i" > /dev/null || true
        if [ $((i % 5)) -eq 0 ]; then
            echo "  Sent $i requests..."
        fi
    done
    echo "Waiting for traces to be written to Cassandra..."
    sleep 20
}

verify_traces() {
    echo "Verifying traces in Cassandra"
    local span_count
    local max_attempts=3
    local attempt=1
    
    while [ $attempt -le $max_attempts ]; do
        echo "Attempt $attempt/$max_attempts: Checking Cassandra..."
        span_count=$(docker compose -f "$compose_file" exec -T cassandra \
            cqlsh -e "USE jaeger_v1_dc1; SELECT COUNT(*) FROM traces;" 2>/dev/null | \
            grep -o '[0-9]\+' | tail -1 || echo "0")
        
        echo "Found $span_count spans"
        
        if [ "$span_count" -ge 5 ]; then
            echo "Found $span_count spans in storage"
            return 0
        fi
        
        if [ $attempt -lt $max_attempts ]; then
            echo "Not enough spans yet, waiting 10 seconds..."
            sleep 10
        fi
        attempt=$((attempt + 1))
    done
    
    echo "ERROR: Expected at least 5 spans, found $span_count after $max_attempts attempts"
    echo "Checking collector logs for errors..."
    docker compose -f "$compose_file" logs --tail=50 jaeger-collector
    return 1
}

dump_logs() {
    echo "::group::🚧 Container logs"
    docker compose -f "$compose_file" logs
    echo "::endgroup::"
}

teardown() {
    if [[ "$success" == "false" ]]; then
        dump_logs
    fi
    docker compose -f "$compose_file" down -v
}

main() {
    setup_services
    trap teardown EXIT
    
    wait_for_cassandra
    sleep 30  # Wait for collector to start
    
    verify_services
    generate_traces
    verify_traces
    
    success="true"
    echo "All tests passed"
}

main

