#!/bin/bash

# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OPERATION="${1:-}"
IMAGE_TAG="${2:-}"

case "$OPERATION" in
  delete)
    kubectl delete namespace hotrod-test --ignore-not-found
    exit
    ;;
  render|apply)
    if [[ ! "$IMAGE_TAG" =~ ^[a-zA-Z0-9_][a-zA-Z0-9_.-]{0,127}$ ]]; then
      echo "A valid local image tag is required" >&2
      exit 1
    fi
    ;;
  *)
    echo "Usage: $0 {render|apply} <image-tag> | delete" >&2
    exit 1
    ;;
esac

render() {
  cat <<'YAML'
apiVersion: v1
kind: Namespace
metadata:
  name: hotrod-test
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: jaeger-config
  namespace: hotrod-test
data:
  config.yaml: |
YAML
  sed 's/^/    /' "$SCRIPT_DIR/config.yaml"
  echo '  ui-config.json: |'
  sed 's/^/    /' "$SCRIPT_DIR/ui-config.json"
  echo '---'
  sed "s/__IMAGE_TAG__/$IMAGE_TAG/g" "$SCRIPT_DIR/resources.yaml"
}

if [[ "$OPERATION" == render ]]; then
  render
else
  render | kubectl apply -f -
fi
