#!/bin/bash

# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

set -euf -o pipefail

print_help() {
  echo "Usage: $0 [-t test_type] [-p port] [-o output directory]"
  echo "  -t: Test type (badger, cassandra, etc.)"
  echo "  -p: Metrics port (default: 8888)"
  echo "  -o: Output directory for metrics files"
  exit 1
}

test_type=""
port=8888
output_dir="./metrics"

# Parse arguments
while getopts "t:p:o:" opt; do
    case "${opt}" in
    t)
        test_type="${OPTARG}"
        ;;
    p)
        port="${OPTARG}"
        ;;
    o)
        output_dir="${OPTARG}"
        ;;
    ?)
        print_help
        ;;
    esac
done

# Metrics scraping function
scrape_metrics() {
    local test_type=$1
    local port=$2
    local output_file="${output_dir}/${test_type}_metrics.txt"
    
    # Create output directory if it doesn't exist
    mkdir -p "$output_dir"

    if [ -z "$test_type" ]; then
        echo "Test type is required"
        exit 1
    fi

    if ! curl -s "http://localhost:$port/metrics" > "$output_file"; then
        echo "Failed to scrape metrics for $test_type from port $port"
        exit 1
    fi

    echo "Metrics for $test_type saved to $output_file"
}

if [ -z "$test_type" ]; then
    echo "Test type is required"
    print_help
fi

# Scrape metrics
scrape_metrics "$test_type" "$port"