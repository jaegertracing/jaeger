#!/bin/bash

# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

set -euf -o pipefail

print_help() {
  echo "Usage: $0 [-c component] [-o output_dir]"
  echo "  -c: Jaeger component (agent, collector, query, ingester, all-in-one)"
  echo "  -o: Output directory for metrics files"
  exit 1
}

# Default values
component=""
output_dir="./metrics"

# Jaeger component metrics ports
declare -A COMPONENT_PORTS=(
    ["agent"]=14271
    ["collector"]=14269
    ["query"]=16687
    ["ingester"]=14270
    ["all-in-one"]=14269
)

# Parse arguments
while getopts "c:o:" opt; do
    case "${opt}" in
    c)
        component="${OPTARG}"
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
    local component=$1
    local port=${COMPONENT_PORTS[$component]}
    local output_file="${output_dir}/jaeger_${component}_metrics.txt"
    
    if [ -z "$port" ]; then
        echo "Unknown Jaeger component: $component"
        exit 1
    fi

    # Create output directory if it doesn't exist
    mkdir -p "$output_dir"

    curl -s "http://localhost:$port/metrics" > "$output_file"
    echo "Metrics for $component saved to $output_file"
}


if [ -z "$component" ]; then
    echo "Component is required"
    print_help
fi

scrape_metrics "$component"