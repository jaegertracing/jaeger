#!/bin/bash

# Copyright (c) 2025 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

set -euxf -o pipefail

success="false"
timeout=360
compose_file="docker-compose/jaeger-docker-compose.yml"

setup_services() {
    echo "Starting Jaeger with adaptive sampling and Cassandra"
    docker compose -f "$compose_file" up -d
}

wait_for_cassandra() {
    local end_time=$((SECONDS + timeout))
    local count=0
    echo "Waiting for Cassandra to become healthy (timeout: ${timeout}s)..."
    while [ $SECONDS -lt $end_time ]; do
        if docker compose -f "$compose_file" ps | grep cassandra | grep -q "healthy"; then
            echo "Cassandra is healthy"
            # Wait a bit more for Cassandra to fully initialize
            echo "Waiting additional 15 seconds for Cassandra to fully stabilize..."
            sleep 15
            return 0
        fi
        count=$((count + 1))
        if [ $((count % 10)) -eq 0 ]; then
            elapsed=$((SECONDS))
            echo "Still waiting for Cassandra... (${elapsed}s elapsed)"
        fi
        sleep 3
    done
    echo "ERROR: Cassandra did not become healthy after ${timeout}s"
    docker compose -f "$compose_file" logs cassandra | tail -50
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
        curl -s "http://localhost:8080/dispatch?customer=123&nonce=$i" > /dev/null || true
        if [ $((i % 5)) -eq 0 ]; then
            echo "  Sent $i requests..."
        fi
    done
    echo "Waiting for traces to be written to Cassandra..."
    sleep 30
}

verify_traces() {
    echo "Verifying traces in Cassandra"
    local span_count
    local max_attempts=5
    local attempt=1
    
    while [ $attempt -le $max_attempts ]; do
        echo "Attempt $attempt/$max_attempts: Checking Cassandra..."
        span_count=$(docker compose -f "$compose_file" exec -T cassandra \
            cqlsh -e "USE jaeger_v1_dc1; SELECT COUNT(*) FROM traces;" 2>/dev/null | \
            grep -o '[0-9]\+' | tail -1 || echo "0")
        
        if [ "$span_count" -ge 1 ]; then
            echo "SUCCESS: Found $span_count span(s) in Cassandra - E2E pipeline is working!"
            return 0
        fi
        
        echo "No spans found yet (attempt $attempt/$max_attempts)"
        if [ $attempt -lt $max_attempts ]; then
            echo "Waiting 15 seconds before retry..."
            sleep 15
        fi
        attempt=$((attempt + 1))
    done
    
    echo "ERROR: No spans found after $max_attempts attempts and 75s of waiting"
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
    
    echo "Waiting for collector and other services to start..."
    sleep 40
    
    echo "Checking service status..."
    docker compose -f "$compose_file" ps
    
    verify_services
    generate_traces
    verify_traces
    
    success="true"
    echo "All tests passed"
}

main

