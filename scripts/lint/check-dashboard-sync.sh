#!/usr/bin/env bash

# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

# Verifies that monitoring/jaeger-mixin/dashboard-for-grafana-v2.json is in sync
# with the Go generator at monitoring/jaeger-mixin/generate/main.go.
#
# If they differ, the developer forgot to run 'make generate-dashboards' after
# changing the generator source.
#
# Usage: ./scripts/lint/check-dashboard-sync.sh

set -euo pipefail

MIXIN_DIR="monitoring/jaeger-mixin"
COMMITTED="${MIXIN_DIR}/dashboard-for-grafana-v2.json"

if [[ ! -f "$COMMITTED" ]]; then
  echo "ERROR: ${COMMITTED} not found. Run 'make generate-dashboards' first."
  exit 1
fi

GENERATED=$(cd "${MIXIN_DIR}/generate" && go run . | python3 -m json.tool --sort-keys)
EXPECTED=$(python3 -m json.tool --sort-keys "$COMMITTED")

if [[ "$GENERATED" != "$EXPECTED" ]]; then
  echo "ERROR: ${COMMITTED} is out of sync with generate/main.go."
  echo "Run 'make generate-dashboards' and commit the result."
  exit 1
fi

echo "OK: ${COMMITTED} is in sync with generate/main.go."
