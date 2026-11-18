#!/bin/bash

# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

SHUNIT2="${SHUNIT2:?'expecting SHUNIT2 env var pointing to a dir with https://github.com/kward/shunit2 clone'}"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# --- Public smoke tests (Step 7) with stub curl endpoints ---
# Run deploy-all.sh end to end in upgrade mode with stub git/helm/kubectl and
# a dispatch curl stub serving canned endpoint responses. These exercise the
# smoke functions and the public-smoke gating directly: healthy and failed HTTP,
# compact and pretty Grafana JSON, missing or wrong database, empty metrics
# series, and jaeger-only scope.

SMOKE_STUB_DIR=""

setupSmokeStubs() {
  SMOKE_STUB_DIR=$(mktemp -d)
  export HELM_CALLS="$SMOKE_STUB_DIR/helm-calls.log"
  export CURL_CALLS="$SMOKE_STUB_DIR/curl-calls.log"
  : > "$HELM_CALLS"
  : > "$CURL_CALLS"
  unset FAKE_GRAFANA_BODY FAKE_METRICS_BODY FAKE_METRICS_HTTP_500 FAKE_METRICS_TRANSPORT_FAILURE

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

  mkdir "$SMOKE_STUB_DIR/responses"
  cat > "$SMOKE_STUB_DIR/sleep" <<'STUB'
#!/usr/bin/env bash
exit 0
STUB

  # curl: retain HTTP error bodies only when --fail-with-body is requested.
  cat > "$SMOKE_STUB_DIR/curl" <<'STUB'
#!/usr/bin/env bash
url=""
output=""
fail_with_body=false
while (($#)); do
  case "$1" in
    --fail-with-body) fail_with_body=true; shift ;;
    -o) output="$2"; shift 2 ;;
    -*) shift ;;
    *) url="$1"; shift ;;
  esac
done
echo "curl $url $output" >> "${CURL_CALLS:-/dev/null}"
body=""
rc=0
case "$url" in
  *hotrod/dispatch*)
    body='{"Driver":"T123","ETA":120000000000}' ;;
  *api/v3/services*)
    body='{"data":["frontend","customer","driver","route"]}' ;;
  *api/metrics/calls*)
    body="${FAKE_METRICS_BODY:-}"
    if [[ "${FAKE_METRICS_HTTP_500:-false}" == "true" ]]; then rc=22; fi
    if [[ "${FAKE_METRICS_TRANSPORT_FAILURE:-false}" == "true" ]]; then
      if [[ ! -e "${CURL_CALLS}.attempted" ]]; then
        touch "${CURL_CALLS}.attempted"
      else
        exit 7
      fi
    fi ;;
  *grafana/api/health*)
    body="${FAKE_GRAFANA_BODY:-}" ;;
esac
if [[ -n "$output" ]] && { [[ "$rc" -eq 0 ]] || "$fail_with_body"; }; then printf '%s' "$body" > "$output"; fi
exit "$rc"
STUB

  chmod +x "$SMOKE_STUB_DIR/helm" "$SMOKE_STUB_DIR/kubectl" "$SMOKE_STUB_DIR/git" "$SMOKE_STUB_DIR/curl" "$SMOKE_STUB_DIR/sleep"
}

teardownSmokeStubs() {
  [[ -n "$SMOKE_STUB_DIR" ]] && rm -rf "$SMOKE_STUB_DIR"
  SMOKE_STUB_DIR=""
  unset HELM_CALLS CURL_CALLS FAKE_GRAFANA_BODY FAKE_METRICS_BODY FAKE_METRICS_HTTP_500 FAKE_METRICS_TRANSPORT_FAILURE
}

HEALTHY_GRAFANA_PRETTY='{
  "database": "ok",
  "version": "12.4.1"
}'
HEALTHY_METRICS='{"name":"service_call_rate","type":"GAUGE","help":"calls/sec","metrics":[{"labels":[{"name":"service_name","value":"frontend"}],"metricPoints":[{"gaugeValue":{"doubleValue":0.27}}]}]}'

# Runs the script in upgrade mode with the given deploy scope against the stubs,
# with sleep stubbed so failure paths return quickly.
runSmoke() {
  env HELM_CALLS="$HELM_CALLS" CURL_CALLS="$CURL_CALLS" \
    FAKE_GRAFANA_BODY="${FAKE_GRAFANA_BODY:-$HEALTHY_GRAFANA_PRETTY}" \
    FAKE_METRICS_BODY="${FAKE_METRICS_BODY:-$HEALTHY_METRICS}" \
    FAKE_METRICS_HTTP_500="${FAKE_METRICS_HTTP_500:-false}" \
    JAEGER_DEMO_DEPLOY_SCOPE="$1" RUN_PUBLIC_SMOKE_TESTS=true \
    JAEGER_DEMO_PUBLIC_BASE_URL=https://demo.jaegertracing.io \
    FAKE_METRICS_TRANSPORT_FAILURE="${FAKE_METRICS_TRANSPORT_FAILURE:-false}" \
    TMPDIR="$SMOKE_STUB_DIR/responses" \
    PATH="$SMOKE_STUB_DIR:$PATH" \
    bash "$SCRIPT_DIR/deploy-all.sh" upgrade 2>&1
}

testSmokeChecksPassAgainstHealthyPrettyJsonResponses() {
  setupSmokeStubs
  local output
  output=$(runSmoke jaeger)
  assertEquals 'script should succeed against healthy endpoints' 0 $?
  assertContains "$output" 'Smoke check passed: https://demo.jaegertracing.io/grafana/api/health (satisfied .database == "ok")'
  assertContains "$output" 'Smoke check passed: https://demo.jaegertracing.io/jaeger/api/metrics/calls?service=frontend (satisfied (.metrics | type) == "array" and (.metrics | length) > 0)'
  teardownSmokeStubs
}

testSmokeChecksAcceptCompactGrafanaHealthJson() {
  setupSmokeStubs
  FAKE_GRAFANA_BODY='{"database":"ok","version":"12.4.1"}'
  local output
  output=$(runSmoke jaeger)
  assertEquals 'compact grafana health JSON should pass' 0 $?
  assertContains "$output" 'grafana/api/health (satisfied'
  teardownSmokeStubs
}

testSmokeChecksFailWhenGrafanaDatabaseIsNotOk() {
  setupSmokeStubs
  FAKE_GRAFANA_BODY='{"database":"degraded"}'
  local output
  output=$(runSmoke jaeger)
  assertEquals 'a degraded grafana database must fail the deploy' 1 $?
  assertContains "$output" '❌ Smoke check failed: https://demo.jaegertracing.io/grafana/api/health'
  assertContains "$output" 'Expected HTTP 2xx with a JSON body satisfying: .database == "ok"'
  teardownSmokeStubs
}

testSmokeChecksFailWhenGrafanaHealthOmitsDatabase() {
  setupSmokeStubs
  FAKE_GRAFANA_BODY='{"version":"12.4.1"}'
  local output
  output=$(runSmoke jaeger)
  assertEquals 'grafana health without a database field must fail' 1 $?
  assertContains "$output" '❌ Smoke check failed: https://demo.jaegertracing.io/grafana/api/health'
  teardownSmokeStubs
}

testSmokeChecksFailWhenMetricsEndpointReturnsHttpError() {
  setupSmokeStubs
  FAKE_METRICS_HTTP_500=true
  local output
  output=$(runSmoke jaeger)
  assertEquals 'a metrics endpoint HTTP error must fail the deploy' 1 $?
  assertContains "$output" '❌ Smoke check failed: https://demo.jaegertracing.io/jaeger/api/metrics/calls?service=frontend'
  teardownSmokeStubs
}

testSmokeChecksFailWhenMetricsResponseHasNoSeries() {
  setupSmokeStubs
  FAKE_METRICS_BODY='{"name":"service_call_rate","type":"GAUGE","help":"calls/sec","metrics":[]}'
  local output
  output=$(runSmoke jaeger)
  assertEquals 'a metrics response with no series must fail the deploy' 1 $?
  assertContains "$output" '❌ Smoke check failed: https://demo.jaegertracing.io/jaeger/api/metrics/calls?service=frontend'
  teardownSmokeStubs
}

testSmokeChecksFailWhenMetricsFieldIsNotAnArray() {
  setupSmokeStubs
  FAKE_METRICS_BODY='{"name":"service_call_rate","metrics":{"frontend":"ok"}}'
  local output
  output=$(runSmoke jaeger)
  assertEquals 'a non-array metrics field must fail the deploy' 1 $?
  assertContains "$output" '❌ Smoke check failed: https://demo.jaegertracing.io/jaeger/api/metrics/calls?service=frontend'
  teardownSmokeStubs
}

testSmokeChecksQueryGrafanaInJaegerOnlyScope() {
  setupSmokeStubs
  local output
  output=$(runSmoke jaeger)
  assertEquals 'jaeger-only scope with smoke tests should succeed' 0 $?
  assertContains "$output" 'grafana/api/health'
  if ! grep -q 'grafana/api/health' "$CURL_CALLS"; then
    fail 'grafana health endpoint must be queried in jaeger-only scope'
  fi
  teardownSmokeStubs
}

testSmokeChecksRetainMetricsHttpErrorBody() {
  setupSmokeStubs
  FAKE_METRICS_HTTP_500=true
  FAKE_METRICS_BODY='{"errors":[{"msg":"spanmetrics unavailable"}]}'
  local output
  output=$(runSmoke jaeger)
  assertEquals 'HTTP 500 must fail even with a JSON body' 1 $?
  assertContains "$output" 'spanmetrics unavailable'
  teardownSmokeStubs
}

testSmokeChecksDiscardStaleBodyAfterTransportFailure() {
  setupSmokeStubs
  FAKE_METRICS_TRANSPORT_FAILURE=true
  FAKE_METRICS_BODY='{"metrics":[],"stale":"first-attempt-body"}'
  local output
  output=$(runSmoke jaeger)
  assertEquals 'transport failure must fail the deploy' 1 $?
  assertNotContains "$output" 'first-attempt-body'
  teardownSmokeStubs
}

testSmokeChecksFailOnMalformedJson() {
  setupSmokeStubs
  FAKE_METRICS_BODY='not-json'
  local output
  output=$(runSmoke jaeger)
  assertEquals 'malformed metrics JSON must fail' 1 $?
  assertContains "$output" 'not-json'
  teardownSmokeStubs
}

testSmokeChecksFailOnMissingMetricsField() {
  setupSmokeStubs
  FAKE_METRICS_BODY='{"name":"calls"}'
  local output
  output=$(runSmoke jaeger)
  assertEquals 'missing metrics must fail' 1 $?
  teardownSmokeStubs
}

testSmokeChecksRunInAllScope() {
  setupSmokeStubs
  local output
  output=$(runSmoke all)
  assertEquals 'all scope must run both smoke additions' 0 $?
  assertContains "$output" 'grafana/api/health'
  assertContains "$output" 'api/metrics/calls'
  teardownSmokeStubs
}

testSmokeChecksSkipMonitoringOnlyScope() {
  setupSmokeStubs
  local output
  output=$(runSmoke monitoring)
  assertEquals 'monitoring-only scope should succeed without smoke checks' 0 $?
  assertNotContains "$output" 'Step 7'
  assertEquals 'monitoring-only scope must not query public endpoints' 0 "$(wc -c < "$CURL_CALLS")"
  teardownSmokeStubs
}

testSmokeChecksUseIsolatedDirectoriesAndCleanUpOnSuccess() {
  setupSmokeStubs
  local output first second
  output=$(runSmoke jaeger)
  assertEquals 'first run succeeds' 0 $?
  first=$(head -n 1 "$CURL_CALLS" | awk '{print $3}')
  : > "$CURL_CALLS"
  output=$(runSmoke jaeger)
  assertEquals 'second run succeeds' 0 $?
  second=$(head -n 1 "$CURL_CALLS" | awk '{print $3}')
  assertNotEquals 'separate runs need distinct response paths' "$first" "$second"
  assertContains "$first" "$SMOKE_STUB_DIR/responses/"
  assertFalse 'first response directory must be removed' "test -e '$first'"
  assertFalse 'second response directory must be removed' "test -e '$second'"
  assertEquals 'no per-run directories remain' '' "$(ls -A "$SMOKE_STUB_DIR/responses")"
  teardownSmokeStubs
}

testSmokeChecksCleanUpOnFailure() {
  setupSmokeStubs
  FAKE_GRAFANA_BODY='{"database":"degraded"}'
  local output
  output=$(runSmoke jaeger)
  assertEquals 'unhealthy response fails' 1 $?
  assertEquals 'failed run removes its response directory' '' "$(ls -A "$SMOKE_STUB_DIR/responses")"
  teardownSmokeStubs
}

runWithoutJq() {
  local mode=$1 scope=$2 enabled=$3
  local nojq="$SMOKE_STUB_DIR/without-jq"
  mkdir -p "$nojq"
  local cmd
  for cmd in dirname awk grep bash; do
    ln -sf "$(command -v "$cmd")" "$nojq/$cmd"
  done
  /usr/bin/env PATH="$SMOKE_STUB_DIR:$nojq" HELM_CALLS="$HELM_CALLS" \
    JAEGER_DEMO_DEPLOY_SCOPE="$scope" RUN_PUBLIC_SMOKE_TESTS="$enabled" \
    /bin/bash "$SCRIPT_DIR/deploy-all.sh" "$mode" 2>&1
}

testSmokeChecksRequireJqBeforeAnyClusterChanges() {
  setupSmokeStubs
  local output mode
  for mode in upgrade clean; do
    output=$(runWithoutJq "$mode" jaeger true)
    assertEquals 'missing jq fails before deployment' 1 $?
    assertContains "$output" 'jq is required'
    assertEquals 'no helm calls before jq guard' 0 "$(wc -c < "$HELM_CALLS")"
  done
  teardownSmokeStubs
}

testSmokeChecksDoNotRequireJqInMonitoringScope() {
  setupSmokeStubs
  local output
  output=$(runWithoutJq upgrade monitoring true)
  assertEquals 'monitoring-only deployment does not need jq' 0 $?
  assertNotContains "$output" 'jq is required'
  teardownSmokeStubs
}

testSmokeChecksDoNotRequireJqWhenDisabled() {
  setupSmokeStubs
  local output
  output=$(runWithoutJq upgrade jaeger false)
  assertEquals 'disabled smoke tests do not need jq' 0 $?
  assertNotContains "$output" 'jq is required'
  teardownSmokeStubs
}

# Load and run shunit2
# shellcheck disable=SC1091
. "${SHUNIT2}/shunit2"
