#!/bin/bash

# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

SHUNIT2="${SHUNIT2:?'expecting SHUNIT2 env var pointing to a dir with https://github.com/kward/shunit2 clone'}"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

testPinnedChartRendersGrafanaLivenessProbe() {
  local chart_version
  local rendered
  local render_rc
  local grafana_deploy
  local initial_delay
  local failure_threshold

  chart_version=$(sed -n -E 's/^PROMETHEUS_STACK_CHART_VERSION="\$\{PROMETHEUS_STACK_CHART_VERSION:-([^}]+)\}"/\1/p' "$SCRIPT_DIR/deploy-all.sh")

  rendered=$(helm template prometheus kube-prometheus-stack \
    --repo https://prometheus-community.github.io/helm-charts \
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

  initial_delay=$(printf '%s\n' "$grafana_deploy" | awk '/livenessProbe:/{f=1} f && /initialDelaySeconds:/{print $2; exit}')
  assertEquals "Grafana livenessProbe initialDelaySeconds should be 300" "300" "$initial_delay"

  failure_threshold=$(printf '%s\n' "$grafana_deploy" | awk '/livenessProbe:/{f=1} f && /failureThreshold:/{print $2; exit}')
  assertEquals "Grafana livenessProbe failureThreshold should be 20" "20" "$failure_threshold"
}

# --- Upgrade-mode Prometheus release-status contract (no cluster needed) ---
# These run deploy-all.sh end to end in monitoring-only scope with stub
# helm/kubectl binaries on PATH, exercising the release-status handling
# without a Kubernetes cluster or chart downloads. The contract is the one
# main established in #10132: only a deployed release is upgraded in place;
# any other state aborts the deploy with a repair message so a monitoring
# outage can no longer stay green (the silent-skip failure mode in #10002).

PROM_STUB_DIR=""

setupPromStubs() {
  PROM_STUB_DIR=$(mktemp -d)
  export HELM_CALLS="$PROM_STUB_DIR/helm-calls.log"
  : > "$HELM_CALLS"

  cat > "$PROM_STUB_DIR/helm" <<'STUB'
#!/usr/bin/env bash
echo "helm $*" >> "${HELM_CALLS:-/dev/null}"
if [[ "${1:-}" == "status" && "${2:-}" == "prometheus" ]]; then
  if [[ "${FAKE_PROM_STATUS:-deployed}" == "missing" ]]; then
    echo "Error: release: not found" >&2
    exit 1
  fi
  printf 'NAME: prometheus\nLAST DEPLOYED: Sat Nov 14 2026\nNAMESPACE: default\nSTATUS: %s\nREVISION: 3\n' "${FAKE_PROM_STATUS:-deployed}"
fi
exit 0
STUB

  cat > "$PROM_STUB_DIR/kubectl" <<'STUB'
#!/usr/bin/env bash
exit 0
STUB

  chmod +x "$PROM_STUB_DIR/helm" "$PROM_STUB_DIR/kubectl"
}

teardownPromStubs() {
  [[ -n "$PROM_STUB_DIR" ]] && rm -rf "$PROM_STUB_DIR"
  PROM_STUB_DIR=""
  unset HELM_CALLS
}

# Runs the script in upgrade mode, monitoring scope only, against the stubs.
runUpgradeMonitoringOnly() {
  env FAKE_PROM_STATUS="$1" HELM_CALLS="$HELM_CALLS" \
    JAEGER_DEMO_DEPLOY_SCOPE=monitoring RUN_PUBLIC_SMOKE_TESTS=false \
    PATH="$PROM_STUB_DIR:$PATH" \
    bash "$SCRIPT_DIR/deploy-all.sh" upgrade 2>&1
}

helmUpgradedPrometheus() {
  grep -q 'upgrade --install --wait --timeout 15m0s prometheus prometheus-community/kube-prometheus-stack' "$HELM_CALLS"
}

testUpgradeKeepsDeployedPrometheusRelease() {
  setupPromStubs
  local output
  output=$(runUpgradeMonitoringOnly deployed)
  assertEquals 'script should succeed for a deployed release' 0 $?
  assertTrue 'prometheus helm upgrade should run' 'helmUpgradedPrometheus'
  assertNotContains "$output" "repair it separately before upgrading"
  teardownPromStubs
}

testUpgradeAbortsOnFailedPrometheusRelease() {
  setupPromStubs
  local output
  output=$(runUpgradeMonitoringOnly failed)
  assertEquals 'script must fail closed on a failed release' 1 $?
  assertContains "$output" "Prometheus is not deployed (status: failed); repair it separately before upgrading"
  assertFalse 'prometheus helm upgrade must not run' 'helmUpgradedPrometheus'
  teardownPromStubs
}

testUpgradeAbortsOnMissingPrometheusRelease() {
  setupPromStubs
  local output
  output=$(runUpgradeMonitoringOnly missing)
  assertEquals 'script must fail closed when prometheus is not installed' 1 $?
  assertContains "$output" "Prometheus is not deployed (status: missing); repair it separately before upgrading"
  assertFalse 'prometheus helm upgrade must not run' 'helmUpgradedPrometheus'
  teardownPromStubs
}

testUpgradeAbortsOnTransitionalPrometheusStates() {
  setupPromStubs
  local output
  output=$(runUpgradeMonitoringOnly pending_install)
  assertEquals 'script must fail closed on a pending_install release' 1 $?
  assertContains "$output" "Prometheus is not deployed (status: pending_install); repair it separately before upgrading"
  assertFalse 'prometheus helm upgrade must not run' 'helmUpgradedPrometheus'
  teardownPromStubs
}

# --- Public smoke tests (Step 7) with stub curl endpoints ---
# Run deploy-all.sh end to end in upgrade mode with stub git/helm/kubectl and
# a dispatch curl stub serving canned endpoint responses. These exercise the
# smoke functions and the monitoring gating directly: healthy and failed HTTP,
# compact and pretty Grafana JSON, missing or wrong database, empty metrics
# series, and deploy_monitoring on/off.

SMOKE_STUB_DIR=""

setupSmokeStubs() {
  SMOKE_STUB_DIR=$(mktemp -d)
  export HELM_CALLS="$SMOKE_STUB_DIR/helm-calls.log"
  export CURL_CALLS="$SMOKE_STUB_DIR/curl-calls.log"
  : > "$HELM_CALLS"
  : > "$CURL_CALLS"
  unset FAKE_GRAFANA_BODY FAKE_METRICS_BODY FAKE_METRICS_HTTP_500

  # helm: every release looks deployed; mutating calls are just logged.
  cat > "$SMOKE_STUB_DIR/helm" <<'STUB'
#!/usr/bin/env bash
echo "helm $*" >> "${HELM_CALLS:-/dev/null}"
if [[ "${1:-}" == "status" ]]; then
  printf 'NAME: %s\nNAMESPACE: default\nSTATUS: deployed\nREVISION: 3\n' "${2:-}"
fi
exit 0
STUB

  cat > "$SMOKE_STUB_DIR/kubectl" <<'STUB'
#!/usr/bin/env bash
exit 0
STUB

  cat > "$SMOKE_STUB_DIR/git" <<'STUB'
#!/usr/bin/env bash
exit 0
STUB

  # curl: dispatch canned bodies per endpoint; -f-style HTTP failures use curl's
  # exit code 22 and suppress the body file, like real curl -fsS.
  cat > "$SMOKE_STUB_DIR/curl" <<'STUB'
#!/usr/bin/env bash
url=""
output=""
while (($#)); do
  case "$1" in
    -o) output="$2"; shift 2 ;;
    -*) shift ;;
    *) url="$1"; shift ;;
  esac
done
echo "curl $url" >> "${CURL_CALLS:-/dev/null}"
body=""
rc=0
case "$url" in
  *hotrod/dispatch*)
    body='{"Driver":"T123","ETA":120000000000}' ;;
  *api/v3/services*)
    body='{"data":["frontend","customer","driver","route"]}' ;;
  *api/metrics/calls*)
    body="${FAKE_METRICS_BODY:-}"
    if [[ "${FAKE_METRICS_HTTP_500:-false}" == "true" ]]; then rc=22; fi ;;
  *grafana/api/health*)
    body="${FAKE_GRAFANA_BODY:-}" ;;
esac
if [[ "$rc" -eq 0 && -n "$output" ]]; then printf '%s' "$body" > "$output"; fi
exit "$rc"
STUB

  chmod +x "$SMOKE_STUB_DIR/helm" "$SMOKE_STUB_DIR/kubectl" "$SMOKE_STUB_DIR/git" "$SMOKE_STUB_DIR/curl"
}

teardownSmokeStubs() {
  [[ -n "$SMOKE_STUB_DIR" ]] && rm -rf "$SMOKE_STUB_DIR"
  SMOKE_STUB_DIR=""
  unset HELM_CALLS CURL_CALLS FAKE_GRAFANA_BODY FAKE_METRICS_BODY FAKE_METRICS_HTTP_500
}

HEALTHY_GRAFANA_PRETTY='{
  "database": "ok",
  "version": "12.4.1"
}'
HEALTHY_METRICS='{"name":"service_call_rate","type":"GAUGE","help":"calls/sec","metrics":[{"labels":[{"name":"service_name","value":"frontend"}],"metricPoints":[{"gaugeValue":{"doubleValue":0.27}}]}]}'

# Runs the script in upgrade mode with the given deploy scope against the stubs,
# with smoke retries shrunk so failure paths return quickly.
runSmoke() {
  env HELM_CALLS="$HELM_CALLS" CURL_CALLS="$CURL_CALLS" \
    FAKE_GRAFANA_BODY="${FAKE_GRAFANA_BODY:-$HEALTHY_GRAFANA_PRETTY}" \
    FAKE_METRICS_BODY="${FAKE_METRICS_BODY:-$HEALTHY_METRICS}" \
    FAKE_METRICS_HTTP_500="${FAKE_METRICS_HTTP_500:-false}" \
    JAEGER_DEMO_DEPLOY_SCOPE="$1" RUN_PUBLIC_SMOKE_TESTS=true \
    JAEGER_DEMO_SMOKE_ATTEMPTS=2 JAEGER_DEMO_SMOKE_SLEEP_SECONDS=0 \
    PATH="$SMOKE_STUB_DIR:$PATH" \
    bash "$SCRIPT_DIR/deploy-all.sh" upgrade 2>&1
}

testSmokeChecksPassAgainstHealthyPrettyJsonResponses() {
  setupSmokeStubs
  local output
  output=$(runSmoke all)
  assertEquals 'script should succeed against healthy endpoints' 0 $?
  assertContains "$output" 'Smoke check passed: https://demo.jaegertracing.io/grafana/api/health (satisfied .database == "ok")'
  assertContains "$output" 'Smoke check passed: https://demo.jaegertracing.io/jaeger/api/metrics/calls?service=frontend (satisfied (.metrics | type) == "array" and (.metrics | length) > 0)'
  teardownSmokeStubs
}

testSmokeChecksAcceptCompactGrafanaHealthJson() {
  setupSmokeStubs
  FAKE_GRAFANA_BODY='{"database":"ok","version":"12.4.1"}'
  local output
  output=$(runSmoke all)
  assertEquals 'compact grafana health JSON should pass' 0 $?
  assertContains "$output" 'grafana/api/health (satisfied'
  teardownSmokeStubs
}

testSmokeChecksFailWhenGrafanaDatabaseIsNotOk() {
  setupSmokeStubs
  FAKE_GRAFANA_BODY='{"database":"degraded"}'
  local output
  output=$(runSmoke all)
  assertEquals 'a degraded grafana database must fail the deploy' 1 $?
  assertContains "$output" '❌ Smoke check failed: https://demo.jaegertracing.io/grafana/api/health'
  assertContains "$output" 'Expected HTTP 2xx with a JSON body satisfying: .database == "ok"'
  teardownSmokeStubs
}

testSmokeChecksFailWhenGrafanaHealthOmitsDatabase() {
  setupSmokeStubs
  FAKE_GRAFANA_BODY='{"version":"12.4.1"}'
  local output
  output=$(runSmoke all)
  assertEquals 'grafana health without a database field must fail' 1 $?
  assertContains "$output" '❌ Smoke check failed: https://demo.jaegertracing.io/grafana/api/health'
  teardownSmokeStubs
}

testSmokeChecksFailWhenMetricsEndpointReturnsHttpError() {
  setupSmokeStubs
  FAKE_METRICS_HTTP_500=true
  local output
  output=$(runSmoke all)
  assertEquals 'a metrics endpoint HTTP error must fail the deploy' 1 $?
  assertContains "$output" '❌ Smoke check failed: https://demo.jaegertracing.io/jaeger/api/metrics/calls?service=frontend'
  teardownSmokeStubs
}

testSmokeChecksFailWhenMetricsResponseHasNoSeries() {
  setupSmokeStubs
  FAKE_METRICS_BODY='{"name":"service_call_rate","type":"GAUGE","help":"calls/sec","metrics":[]}'
  local output
  output=$(runSmoke all)
  assertEquals 'a metrics response with no series must fail the deploy' 1 $?
  assertContains "$output" '❌ Smoke check failed: https://demo.jaegertracing.io/jaeger/api/metrics/calls?service=frontend'
  teardownSmokeStubs
}

testSmokeChecksFailWhenMetricsFieldIsNotAnArray() {
  setupSmokeStubs
  FAKE_METRICS_BODY='{"name":"service_call_rate","metrics":{"frontend":"ok"}}'
  local output
  output=$(runSmoke all)
  assertEquals 'a non-array metrics field must fail the deploy' 1 $?
  assertContains "$output" '❌ Smoke check failed: https://demo.jaegertracing.io/jaeger/api/metrics/calls?service=frontend'
  teardownSmokeStubs
}

testSmokeChecksSkipGrafanaWhenMonitoringIsNotDeployed() {
  setupSmokeStubs
  local output
  output=$(runSmoke jaeger)
  assertEquals 'jaeger-only scope with smoke tests should succeed' 0 $?
  assertNotContains "$output" 'grafana/api/health'
  if grep -q 'grafana/api/health' "$CURL_CALLS"; then
    fail 'grafana health endpoint must not be queried in jaeger-only scope'
  fi
  teardownSmokeStubs
}

# Load and run shunit2
# shellcheck disable=SC1091
. "${SHUNIT2}/shunit2"
