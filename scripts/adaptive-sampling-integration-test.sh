#!/bin/bash

set -euf -o pipefail

# Function to start Jaeger with adaptive sampling
start_jaeger() {
  echo "Starting Jaeger with adaptive sampling..."
  SAMPLING_STORAGE_TYPE=memory SAMPLING_CONFIG_TYPE=adaptive go run -tags=ui ../cmd/all-in-one --log-level=debug
  # Wait for Jaeger to start (adjust sleep duration as needed)
  sleep 10 &
}

# Function to run trace generator with adaptive sampling
run_trace_generator() {
  echo "Running trace generator with adaptive sampling..."
  go run ./cmd/tracegen -adaptive-sampling=http://localhost:14268/api/sampling -pause=10ms -duration=60m
  # Wait for trace generation to complete (adjust duration as needed)
  sleep 300 &
}

# Function to check adaptive sampling strategy changes
check_sampling_strategy() {
  echo "Initial adaptive sampling strategy:"
  curl -s 'http://localhost:14268/api/sampling?service=tracegen' | jq .

  # Wait for some time to allow for adaptive sampling adjustments (adjust sleep duration as needed)
  sleep 300

  echo "Updated adaptive sampling strategy:"
  curl -s 'http://localhost:14268/api/sampling?service=tracegen' | jq .

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