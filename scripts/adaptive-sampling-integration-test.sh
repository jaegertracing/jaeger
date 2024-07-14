#!/bin/bash

set -euf -o pipefail

start_jaeger() {
  echo "Starting Jaeger with adaptive sampling..."
  SAMPLING_STORAGE_TYPE=memory SAMPLING_CONFIG_TYPE=adaptive go run -tags=ui ../cmd/all-in-one --log-level=debug &

  # Wait for Jaeger to start (adjust sleep duration as needed)
  until curl -s 'http://localhost:16686' > /dev/null; do
    echo "waiting for Jaeger to start"
    sleep 2
  done
  echo "Jaeger started"
}

set_initial_sampling_rate() {
  local service_name="$1"
  local initial_rate="$2"

  echo "Setting initial sampling rate for service $service_name to $initial_rate..."
  curl -X POST 'http://localhost:14268/api/sampling' \
    -H 'Content-Type: application/json' \
    -d '{
          "service": "'"$service_name"'",
          "operation": "default",
          "type": "probabilistic",
          "param": '"$initial_rate"'
        }'

  echo "Initial sampling rate set."
}

start_trace_generator() {
  echo "Running trace generator with adaptive sampling..."
  go run ../cmd/tracegen -adaptive-sampling=http://localhost:14268/api/sampling -pause=10ms -duration=60m &
  tracegen_pid=$!

  # Wait for trace generator to start
  until curl -s 'http://localhost:14268' > /dev/null; do
    echo "waiting for trace generator to start.."
    sleep 2
  done 
  echo "Trace generator started."
}


check_sampling_strategy() {
  echo "Initial adaptive sampling strategy:"
  initial_strategy=$(curl -s 'http://localhost:14268/api/sampling?service=tracegen')
  echo "$initial_strategy" | jq .

  echo "Monitoring adaptive sampling adjustments..."
  max_checks=60  # Maximum number of checks before timing out
  check_interval=5  # Time in seconds between checks

  for ((i=1; i<=max_checks; i++)); do
    echo "Check $i of $max_checks..."

    updated_strategy=$(curl -s 'http://localhost:14268/api/sampling?service=tracegen')
    echo "Updated adaptive sampling strategy:"
    echo "$updated_strategy" | jq .

    initial_prob=$(echo "$initial_strategy" | jq -r '.probabilities[0].probability')
    updated_prob=$(echo "$updated_strategy" | jq -r '.probabilities[0].probability')

    echo "Initial probability: $initial_prob"
    echo "Updated probability: $updated_prob"

    if (( $(echo "$updated_prob < $initial_prob" | bc -l) )); then
      echo "Adaptive sampling is working: probability decreased from $initial_prob to $updated_prob"
      return 0
    else
      echo "Adaptive sampling adjustment not yet observed, sleeping for $check_interval seconds..."
      sleep $check_interval
    fi
  done

  echo "Adaptive sampling is not working as expected: probability did not decrease after $((max_checks * check_interval)) seconds"
  exit 1
}

main() {
  local service_name="tracegen"
  local initial_rate=0.5  # Set your initial sampling rate here

  start_jaeger
  set_initial_sampling_rate "$service_name" "$initial_rate"
  start_trace_generator
  check_sampling_strategy
}

main
