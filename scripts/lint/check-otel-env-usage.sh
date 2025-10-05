#!/bin/bash

# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

# Deprecated Jaeger env vars that should not be used with OTEL SDK
DEPRECATED_VARS=(
  "JAEGER_AGENT_HOST"
  "JAEGER_AGENT_PORT"
  "JAEGER_ENDPOINT"
  "JAEGER_SAMPLER_TYPE"
  "JAEGER_SAMPLER_PARAM"
)

ERRORS=0
TARGET_FILE="${1:-}"

if [ -n "$TARGET_FILE" ]; then
  FILES="$TARGET_FILE"
else
  FILES=$(find . -name "docker-compose*.yml" -o -name "docker-compose*.yaml" | grep -v node_modules || true)
fi

echo "Checking for deprecated Jaeger SDK environment variables..."

for file in $FILES; do
  for var in "${DEPRECATED_VARS[@]}"; do
    if grep -q "$var" "$file" 2>/dev/null; then
      echo "❌ DEPRECATED: Found $var in $file"
      ERRORS=$((ERRORS + 1))
    fi
  done
done

if [ $ERRORS -gt 0 ]; then
  echo ""
  echo "Migration Guide:"
  echo "  JAEGER_AGENT_HOST/PORT → OTEL_EXPORTER_OTLP_ENDPOINT"
  echo "  JAEGER_ENDPOINT → OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"
  echo "  JAEGER_SAMPLER_TYPE → OTEL_TRACES_SAMPLER"
  echo "  JAEGER_SAMPLER_PARAM → OTEL_TRACES_SAMPLER_ARG"
  exit 1
fi

echo "✅ All checks passed"
exit 0
