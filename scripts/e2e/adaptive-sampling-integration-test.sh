#!/bin/bash

# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

set -euf -o pipefail

# This script is currently a placeholder.

# Commands to run integration test:
#   SAMPLING_STORAGE_TYPE=memory SAMPLING_CONFIG_TYPE=adaptive go run -tags=ui ./cmd/all-in-one --log-level=debug
#   go run ./cmd/tracegen -adaptive-sampling=http://localhost:14268/api/sampling -pause=10ms -duration=60m

# Check how strategy is changing
#   curl 'http://localhost:14268/api/sampling?service=tracegen' | jq .

# Issues
# - SDK does not report sampling probability in the tags the way Jaeger SDKs did
# - Server probably does not recognize spans as having adaptive sampling without sampler info
# - There is no way to modify target traces-per-second dynamically, must restart collector.
