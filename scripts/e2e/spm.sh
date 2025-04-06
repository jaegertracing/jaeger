#!/bin/bash

# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

set -euf -o pipefail

print_help() {
  echo "Usage: $0 [-b binary]"
  echo "-b: Which binary to build: 'all-in-one' or 'jaeger' (v2) (default)"
  echo "-h: Print help"
  exit 1
}

BINARY='jaeger'
compose_file=docker-compose/monitor/docker-compose.yml

while getopts "b:h" opt; do
	case "${opt}" in
	b)
		BINARY=${OPTARG}
		;;
	*)
		print_help
		;;
	esac
done

set -x

if [ "$BINARY" == "all-in-one" ]; then
  compose_file=docker-compose/monitor/docker-compose-v1.yml
fi

timeout=600
end_time=$((SECONDS + timeout))
success="false"

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

# Function to check if all services are healthy
wait_for_services() {
  echo "Waiting for services to be up and running..."
  check_service_health "Jaeger" "http://localhost:16686"
  check_service_health "Prometheus" "http://localhost:9090/query"
}

# Function to validate the service metrics
validate_service_metrics() {
    local service=$1
    # Time constants in milliseconds
    local fiveMinutes=300000
    local oneMinute=60000
    local fifteenSec=15000 # Prometheus is also configured to scrape every 15sec.

    # When endTs=(blank) the server will default it to now().
    local url="http://localhost:16686/api/metrics/calls?service=${service}&endTs=&lookback=${fiveMinutes}&step=${fifteenSec}&ratePer=${oneMinute}"
    response=$(curl -s "$url")
    if ! assert_service_name_equals "$response" "$service" ; then
      return 1
    fi

    # Check that at least some values are non-zero after the threshold
    local non_zero_count
    non_zero_count=$(echo "$response" | jq -r '[.metrics[0].metricPoints[].gaugeValue.doubleValue | select(. != 0)] | length')
    local expected_non_zero_count=4
    local zero_count
    zero_count=$(echo "$response" | jq -r '[.metrics[0].metricPoints[].gaugeValue.doubleValue | select(. == 0)] | length')
    local expected_max_zero_count=4
    echo "⏳ Metrics data points found: ${zero_count} zero, ${non_zero_count} non-zero"

    if [[ $zero_count -gt $expected_max_zero_count ]]; then
      echo "❌ ERROR: Zero values crossing threshold limit not expected (Threshold limit - '$expected_max_zero_count')"
      return 1
    fi
    if [[ $non_zero_count -lt $expected_non_zero_count ]]; then
      echo "⏳ Expecting at least 4 non-zero data points"
      return 1
    fi

     # Validate if labels are correct
    local url="http://localhost:16686/api/metrics/calls?service=${service}&groupByOperation=true&endTs=&lookback=${fiveMinutes}&step=${fifteenSec}&ratePer=${oneMinute}"
    response=$(curl -s "$url")
    if ! assert_labels_set_equals "$response" "operation service_name" ; then
      return 1
    fi

    ### Validate Errors Rate metrics
    local url="http://localhost:16686/api/metrics/errors?service=${service}&endTs=&lookback=${fiveMinutes}&step=${fifteenSec}&ratePer=${oneMinute}"
    response=$(curl -s "$url")
    if ! assert_service_name_equals "$response" "$service" ; then
      return 1
    fi

    local url="http://localhost:16686/api/metrics/calls?service=${service}&groupByOperation=true&endTs=&lookback=${fiveMinutes}&step=${fifteenSec}&ratePer=${oneMinute}"
    response=$(curl -s "$url")
    if ! assert_labels_set_equals "$response" "operation service_name" ; then
      return 1
    fi

    return 0
}

assert_service_name_equals() {
  local response=$1
  local expected=$2
  service_name=$(echo "$response" | jq -r 'if .metrics and .metrics[0] then .metrics[0].labels[] | select(.name=="service_name") | .value else empty end')
  if [[ "$service_name" != "$expected" ]]; then
    echo "❌ ERROR: Obtained service_name: '$service_name' are not same as expected: '$expected'"
    return 1
  fi
  return 0
}

assert_labels_set_equals() {
  local response=$1
  local expected="$2 " # need one extra space due to how labels is computed

  labels=$(echo "$response" | jq -r '.metrics[0].labels[].name' | sort | tr '\n' ' ')

  if [[ "$labels" != "$expected" ]]; then
    echo "❌ ERROR: Obtained labels: '$labels' are not same as expected labels: '$expected'"
    return 1
  fi
}

check_spm() {
  local wait_seconds=10
  local successful_service=0
  services_list=("driver" "customer" "mysql" "redis" "frontend" "route" "ui")
  for service in "${services_list[@]}"; do
    echo "Processing service: $service"
    while [ $SECONDS -lt $end_time ]; do
      if validate_service_metrics "$service"; then
        echo "✅ Found all expected metrics for service '$service'"
        successful_service=$((successful_service + 1))
        break
      fi
      sleep $wait_seconds
    done
  done
  if [ $successful_service -lt ${#services_list[@]} ]; then
    echo "❌ ERROR: Expected metrics from ${#services_list[@]} services, found only ${successful_service}"
    exit 1
  else
    echo "✅ All services metrics are returned by the API"
  fi
}

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

main() {
  if [ "$BINARY" == "jaeger" ]; then
    (cd docker-compose/monitor && make build BINARY="$BINARY" && make dev DOCKER_COMPOSE_ARGS="-d")
  else
    (cd docker-compose/monitor && make build BINARY="$BINARY" && make dev-v1 DOCKER_COMPOSE_ARGS="-d")
  fi
  wait_for_services
  check_spm
  success="true"
}

trap teardown_services EXIT INT

# Run the main function
main
