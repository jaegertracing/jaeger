#!/bin/bash

set -e -uxf -o pipefail

compose_file=docker-compose/monitor/docker-compose.yml

dump_logs(){
  echo "::group:: docker logs"
  docker compose -f $compose_file logs
  echo "::endgroup::"
}

# Function to check if a service is healthy
check_service_health() {
  local service_name=$1
  local url=$2
  local wait_seconds=5
  local retry_count=10
  echo "Checking health of service: $service_name at $url"
  for i in $(seq 1 $retry_count); do
    if curl -s -L --head --request GET "$url" | grep "200 OK" > /dev/null; then
      echo "$service_name is healthy"
      return 0
    else
      echo "Waiting for $service_name to be healthy... ($i/$retry_count)"
      sleep $wait_seconds
    fi
  done

  echo "Error: $service_name did not become healthy in time"
  return 1
}

# Function to check if all services are healthy
wait_for_services() {
  echo "Waiting for services to be up and running..."
  check_service_health "Jaeger" "http://localhost:16686"
  check_service_health "Prometheus" "http://localhost:9090/graph"
  check_service_health "Grafana" "http://localhost:3000"
}
# Function to validate the service metrics
service_metrics_valider(){
    local service=$1
    local non_zero_count=0
    local all_non_zero=true
    # Check if the service is up and running
    response=$(curl -s "http://localhost:16686/api/metrics/calls?service=$service&endTs=$(date +%s)000&lookback=1000&step=100&ratePer=60000")
    service_name=$(echo "$response" | jq -r 'if .metrics and .metrics[0] then .metrics[0].labels[] | select(.name=="service_name") | .value else empty end')
    if [ "$service_name" != "$service" ]; then
      echo "Service name does not match '$service'"
      return 1
    else
      echo "Service name matched with '$service'"
    fi
    #stores the gauge values in an array and make sure that there are at least 3 gauge values
    mapfile -t metric_points < <(echo "$response" | jq -r '.metrics[0].metricPoints[].gaugeValue.doubleValue')
    while [ ${#metric_points[@]} -lt 3 ]; do
      echo "Metric points for service $service are less than 3"
      mapfile -t metric_points < <(echo "$response" | jq -r '.metrics[0].metricPoints[].gaugeValue.doubleValue')
    done
    #check if all gauge values are non-zero and count is greater than 3
    for value in "${metric_points[@]}"; do
      if [[ "$value" == "0" || "$value" == "0.0" ]]; then
        all_non_zero=false
        break
      else
        non_zero_count=$((non_zero_count + 1))
      fi
    done
    if [ "$all_non_zero" = true ] && [ $non_zero_count -gt 3 ]; then
      echo "All gauge values are non-zero and count is greater than 3 for $service"
      return 0 
    else
      echo "Some gauge values are zero or count is not greater than 3 for $service"
      return 1
    fi
}


# Function to check SPM
check_spm() {
  local timeout=180
  local interval=5
  local end_time=$((SECONDS + timeout))
  local successful_service=0
  echo "Checking SPM"
  #list of services to check
  services_list=("driver" "customer" "mysql" "redis" "frontend" "route" "ui")
  for service in "${services_list[@]}"; do
    echo "Processing service: $service"
    while [ $SECONDS -lt $end_time ]; do
      #check if the service metrics are returned by the API
      if service_metrics_valider "$service"; then
        successful_service=$((successful_service + 1))
        break
      else
        #timeout condition
        if [ $SECONDS -gt $end_time ]; then
          echo "Error: no metrics returned by the API for service $service"
          exit 1
        fi
        echo "Waiting for metrics to be returned by the API for service $service..."
        sleep $interval
      fi
    done
  done
  if [ $successful_service -lt ${#services_list[@]} ]; then
    echo "All services metrics are not returned by the API"
    exit 1
  else
    echo "All services metrics are returned by the API"
  fi
    
}

# Function to tear down Docker Compose services
teardown_services() {
  dump_logs
  docker compose -f $compose_file down
}

# Main function
main() {
  (cd docker-compose/monitor && make build && make dev DOCKER_COMPOSE_ARGS="-d")
  wait_for_services
  check_spm
  echo "All services are running correctly"
}

trap teardown_services EXIT INT

# Run the main function
main