#!/bin/bash

set -euf -o pipefail

# Function to start Jaeger with adaptive sampling
start_jaeger() {
  echo "Starting Jaeger with adaptive sampling..."
  SAMPLING_STORAGE_TYPE=memory SAMPLING_CONFIG_TYPE=adaptive go run -tags=ui ../cmd/all-in-one --log-level=debug

  # Wait for Jaeger to start (adjust sleep duration as needed)
  until curl -s 'http://localhost:16686' > /dev/null; do
    echo "waiting for Jaeger to start"
    sleep 2
  done
  echo "Jeager started"
}

# Function to run trace generator with adaptive sampling
run_trace_generator() {
  echo "Running trace generator with adaptive sampling..."
  go run ./cmd/tracegen -adaptive-sampling=http://localhost:14268/api/sampling -pause=10ms -duration=60m

  # Wait for trace generation to complete (adjust duration as needed)
  until curl -s 'http://localhost:14268' >dev/null; do
    echo "waiting for trace generator to start.."
    sleep 2
  done 
  echo "Trace generator started."
}

# Function to check adaptive sampling strategy changes
check_sampling_strategy() {
  echo "Initial adaptive sampling strategy:"
  initial_strategy=$(curl -s 'http://localhost:14268/api/sampling?service=tracegen')
  echo "Initial adaptive sampling stratergy:"
  echo "$initial stratergy" | jq.
  
  # Wait for some time to allow for adaptive sampling adjustments (adjust sleep duration as needed)
  adjustment_duration=30
  echo "Waiting for $adjustment_duration seconds to allow for adaptive sampling adjustments..."
  sleep $adjustment_duration

  echo "Fetching updated adaptive sampling strategy..."
  updated_strategy=$(curl -s 'http://localhost:14268/api/sampling?service=tracegen')
  echo "Updated adaptive sampling strategy:"
  echo "$updated_strategy" | jq .

  initial_prob=$(echo "$initial_strategy" | jq -r '.probabilities[0].probability')
  updated_prob=$(echo "$updated_strategy" | jq -r '.probabilities[0].probability')

  echo "Initial probability: $initial_prob"
  echo "Updated probability: $updated_prob"

  if (( $(echo "$updated_prob < $initial_prob" | bc -l) )); then
    echo "Adaptive sampling is working: probability decreased from $initial_prob to $updated_prob"
  else
    echo "Adaptive sampling is not working as expected: probability did not decrease"
    exit 1
  fi
}

# Main function to execute test cases
main() {
  start_jaeger
  run_trace_generator
  check_sampling_strategy
}

# Execute main function
main