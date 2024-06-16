#!/bin/bash

set -e -uxf -o pipefail

# Function to check if a service is healthy
check_service_health() {
  local service_name=$1
  local url=$2
  local retry_count=10
  local wait_seconds=5

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

# Function to tear down Docker Compose services
teardown_services() {
  docker compose -f docker-compose/monitor/docker-compose.yml down
}

# Main function
main() {
  docker compose -f docker-compose/monitor/docker-compose.yml up -d
  wait_for_services
  echo "All services are running correctly"
  teardown_services
}
trap teardown_services EXIT INT
# Run the main function
main
