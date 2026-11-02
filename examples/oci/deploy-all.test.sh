#!/bin/bash

# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

SHUNIT2="${SHUNIT2:?'expecting SHUNIT2 env var pointing to a dir with https://github.com/kward/shunit2 clone'}"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

testPinnedChartRendersGrafanaLivenessProbe() {
  local chart_version
  local chart_ref
  local rendered
  local render_rc
  local grafana_deploy
  local initial_delay
  local failure_threshold

  chart_ref=$(sed -n -E 's/^PROMETHEUS_STACK_CHART="([^"]+)"/\1/p' "$SCRIPT_DIR/deploy-all.sh")
  chart_version=$(sed -n -E 's/^PROMETHEUS_STACK_CHART_VERSION="\$\{PROMETHEUS_STACK_CHART_VERSION:-([^}]+)\}"/\1/p' "$SCRIPT_DIR/deploy-all.sh")

  rendered=$(helm template prometheus "${chart_ref:-prometheus-community/kube-prometheus-stack}" \
    --version "${chart_version:-82.10.4}" \
    --values "$SCRIPT_DIR/monitoring-values.yaml" \
    --show-only charts/grafana/templates/deployment.yaml)
  render_rc=$?

  assertEquals "pinned kube-prometheus-stack chart should render" 0 "$render_rc"
  [[ "$render_rc" -eq 0 ]] || return

  grafana_deploy=$(printf '%s\n' "$rendered" | awk '
    /^---$/ { if (dep && named) printf "%s", doc; doc = ""; dep = 0; named = 0; next }
    { doc = doc $0 "\n" }
    /^kind: Deployment$/ { dep = 1 }
    /name: prometheus-grafana/ { named = 1 }
    END { if (dep && named) printf "%s", doc }
  ')
  assertContains "$grafana_deploy" "kind: Deployment"

  initial_delay=$(printf '%s\n' "$grafana_deploy" | grep -A5 'livenessProbe:' | grep 'initialDelaySeconds:' | awk '{print $2}')
  assertEquals "Grafana livenessProbe initialDelaySeconds should be 300" "300" "$initial_delay"

  failure_threshold=$(printf '%s\n' "$grafana_deploy" | grep -A5 'livenessProbe:' | grep 'failureThreshold:' | awk '{print $2}')
  assertEquals "Grafana livenessProbe failureThreshold should be 20" "20" "$failure_threshold"
}

# Load and run shunit2
# shellcheck disable=SC1091
. "${SHUNIT2}/shunit2"
