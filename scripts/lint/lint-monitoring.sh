#!/bin/bash

# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0




# Install jsonnet and jb if not present
if ! command -v jsonnet &> /dev/null; then
    echo "Installing jsonnet..."
    go install github.com/google/go-jsonnet/cmd/jsonnet@latest
fi

if ! command -v jb &> /dev/null; then
    echo "Installing jb..."
    go install github.com/jsonnet-bundler/jsonnet-bundler/cmd/jb@latest
fi

# Navigate to monitoring directory
cd monitoring/jaeger-mixin

# Install dependencies
~/go/bin/jb install

# Run tests
echo "Running dashboard tests..."
~/go/bin/jsonnet -J vendor tests.jsonnet

# Verify dashboard generation matches checked-in file
echo "Verifying generated dashboard matches checked-in file..."
~/go/bin/jsonnet -J vendor -e '(import "mixin.libsonnet").grafanaDashboards["jaeger.json"]' > dashboard-generated.json
diff dashboard-for-grafana.json dashboard-generated.json
start_code=$?
rm dashboard-generated.json

if [ $start_code -eq 0 ]; then
    echo "Dashboard verification passed."
    exit 0
else
    echo "Dashboard verification failed: generated JSON differs from checked-in file."
    exit 1
fi
