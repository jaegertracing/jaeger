#!/bin/bash

# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

set -euf -o pipefail

print_help() {
  echo "Usage: $0 [-b binary] [-m metricstore]"
  echo "-b: Which binary to build: 'all-in-one' or 'jaeger' (v2, default)"
  echo "-m: Which database to use as metrics store: 'prometheus' (default) or 'elasticsearch' or 'opensearch'"
  echo "-h: Print help"
  exit 1
}

BINARY='jaeger'
METRICSTORE='prometheus'
compose_file=docker-compose/monitor/docker-compose.yml
make_target="dev"

while getopts "b:m:h" opt; do
  case "${opt}" in
  b)
    BINARY=${OPTARG}
    ;;
  m)
    METRICSTORE=${OPTARG}
    ;;
  *)
    print_help
    ;;
  esac
done

set -x

# Validate binary option
case "$BINARY" in
  "all-in-one"|"jaeger")
    # Valid options
    ;;
  *)
    echo "❌ ERROR: Invalid binary option: $BINARY"
    print_help
    ;;
esac

# Validate metricstore option
case "$METRICSTORE" in
  "prometheus"|"elasticsearch"|"opensearch")
    # Valid options
    ;;
  *)
    echo "❌ ERROR: Invalid metricstore option: $METRICSTORE"
    print_help
    ;;
esac

# Set compose file based on binary and metricstore
if [ "$BINARY" == "all-in-one" ]; then
  compose_file=docker-compose/monitor/docker-compose-v1.yml
  make_target="dev-v1"
fi

if [ "$METRICSTORE" == "elasticsearch" ]; then
  compose_file=docker-compose/monitor/docker-compose-elasticsearch.yml
  make_target="elasticsearch"
fi

if [ "$METRICSTORE" == "opensearch" ]; then
  compose_file=docker-compose/monitor/docker-compose-opensearch.yml
  make_target="opensearch"
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
wait_for_services_to_be_healthy() {
  echo "Waiting for services to be up and running..."

  case "$METRICSTORE" in
    "elasticsearch")
      check_service_health "Elasticsearch" "http://localhost:9200"
      ;;
    "opensearch")
      check_service_health "Opensearch" "http://localhost:9200"
      ;;
    "prometheus")
      check_service_health "Prometheus" "http://localhost:9090/query"
      ;;
  esac

  check_service_health "Jaeger" "http://localhost:16686"
}

get_expected_operations_of_service() {
  # Which span names do we expect from which service? 
  # See https://github.com/yurishkuro/microsim/blob/main/config/hotrod.go
  local service=$1
  case "$service" in
    "driver")
      echo "/FindNearest"
      ;;
    "customer")
      echo "/customer"
      ;;
    "mysql")
      echo "/sql_select"
      ;;
    "redis")
      echo "/FindDriverIDs /GetDriver"
      ;;
    "frontend")
      echo "/dispatch"
      ;;
    "route")
      echo "/GetShortestRoute"
      ;;
    "ui")
      echo "/"
      ;;
    *)
      echo ""
      ;;
  esac
}

# Validate that found operations match expected operations for a service
validate_operations_for_service() {
  local service=$1
  local found_operations=$2
  
  local expected_operations
  expected_operations=$(get_expected_operations_of_service "$service")
  
  # If no expected operations defined for this service, skip validation
  if [[ -z "$expected_operations" ]]; then
    return 0
  fi
  
  # Log expected and found operations
  if [[ -n "$found_operations" ]]; then
    echo "Expected operations for service '$service': [$expected_operations] | Found operations: [$found_operations]"
  else
    echo "Expected operations for service '$service': [$expected_operations] | Found operations: []"
  fi
  
  # If no operations found, that's an error
  if [[ -z "$found_operations" ]]; then
    echo "❌ ERROR: No operations found for service '$service', but expected: [$expected_operations]"
    return 1
  fi
  
  # Parse comma-separated operations (format: "op1, op2, op3")
  # Convert to space-separated and normalize whitespace
  local found_ops_list
  found_ops_list=$(echo "$found_operations" | sed 's/,/ /g' | tr -s ' ' | sed 's/^ *//;s/ *$//')
  
  # Check each found operation against expected ones
  local found_op
  for found_op in $found_ops_list; do
    # Remove any leading/trailing spaces
    found_op=$(echo "$found_op" | sed 's/^ *//;s/ *$//')
    
    # Skip empty operations
    if [[ -z "$found_op" ]]; then
      continue
    fi
    
    # Check if this operation is in the expected list
    local is_expected=false
    local expected_op
    for expected_op in $expected_operations; do
      if [[ "$found_op" == "$expected_op" ]]; then
        is_expected=true
        break
      fi
    done
    
    if [[ "$is_expected" == "false" ]]; then
      echo "❌ ERROR: Unexpected operation '$found_op' found for service '$service'. Expected operations: [$expected_operations]"
      return 1
    fi
  done
  
  echo "✅ Operation validation passed for service '$service'"
  return 0
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
    non_zero_count=$(count_non_zero_metrics_point "$response")
    local expected_non_zero_count=4
    local zero_count
    zero_count=$(count_zero_metrics_point "$response")
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
    
    # Validate operations from this service are what we expect.
    echo "Checking operations for service: $service"
    local operations
    operations=$(extract_operations "$response")
    if [[ -n "$operations" ]]; then
      # Validate that found operations match expected ones
      if ! validate_operations_for_service "$service" "$operations" "calls"; then
        return 1
      fi
    else
      echo "❌ ERROR No operations found yet for service '${service}'. We expected to find some."
    fi

    ### Validate Errors Rate metrics
    local url="http://localhost:16686/api/metrics/errors?service=${service}&endTs=&lookback=${fiveMinutes}&step=${fifteenSec}&ratePer=${oneMinute}"
    response=$(curl -s "$url")
    if ! assert_service_name_equals "$response" "$service" ; then
      return 1
    fi

    local url="http://localhost:16686/api/metrics/errors?service=${service}&groupByOperation=true&endTs=&lookback=${fiveMinutes}&step=${fifteenSec}&ratePer=${oneMinute}"
    response=$(curl -s "$url")
    if ! assert_labels_set_equals "$response" "operation service_name" ; then
      return 1
    fi

    non_zero_count=$(count_non_zero_metrics_point "$response")
    local services_with_error="driver frontend ui redis"
    if [[ "$services_with_error" =~ $service ]]; then # the service is in the list
      if [[ $non_zero_count == "0" ]]; then
        echo "❌ ERROR: expect service $service to have positive errors rate"
        return 1
      fi
    else
      if [[ $non_zero_count != "0" ]]; then
        echo "❌ ERROR: expect service $service to have 0 errors, but have $non_zero_count data points with positive errors"
        return 1
      fi
    fi


    return 0
}

assert_service_name_equals() {
  local response=$1
  local expected=$2
  # First check if metrics structure exists at all
  if ! echo "$response" | jq -e '.metrics and .metrics[0]' >/dev/null; then
    echo "⏳ Metrics not available yet (no metrics array)"
    return 1
  fi
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
  return 0
}

extract_operations() {
  local response=$1
  # Extract all unique operation names from all metrics in the response
  # Each metric has labels array, and when groupByOperation=true, each metric has a label with name=="operation"
  local operations
  operations=$(echo "$response" | jq -r '
    if .metrics and (.metrics | length > 0) then
      [.metrics[] | .labels[] | select(.name=="operation") | .value] | unique | sort | .[]
    else
      empty
    end' 2>/dev/null)
  
  if [[ -z "$operations" ]]; then
    echo ""
    return 0
  fi
  
  # Return operations as a comma-separated list
  echo "$operations" | tr '\n' ',' | sed 's/,$//' | sed 's/,/, /g'
}

count_zero_metrics_point() {
  echo "$1" | jq -r '[.metrics[0].metricPoints[].gaugeValue.doubleValue | select(. == 0)] | length'
}

count_non_zero_metrics_point() {
  echo "$1" | jq -r '[.metrics[0].metricPoints[].gaugeValue.doubleValue | select(. != 0 and (. | tostring != "NaN"))] | length'
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
  (cd docker-compose/monitor && make build BINARY="$BINARY" && make $make_target DOCKER_COMPOSE_ARGS="-d")

  wait_for_services_to_be_healthy
  check_spm
  success="true"
}

trap teardown_services EXIT INT

main