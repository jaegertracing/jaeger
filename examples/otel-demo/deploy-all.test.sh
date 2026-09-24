#!/bin/bash

# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

SHUNIT2="${SHUNIT2:?'expecting SHUNIT2 env var pointing to a dir with https://github.com/kward/shunit2 clone'}"

SCRIPT="$(cd "$(dirname "$0")" && pwd)/deploy-all.sh"

run_route() {
  local mode=$1
  local scope=$2
  local predicate=$3

  bash -c 'source "$1"; MODE="$2"; DEPLOY_SCOPE="$3"; "$4"' \
    _ "$SCRIPT" "$mode" "$scope" "$predicate"
}

assertRouteEnabled() {
  local mode=$1
  local scope=$2
  local predicate=$3

  run_route "$mode" "$scope" "$predicate" >/dev/null 2>&1
  assertEquals "$mode/$scope should enable $predicate" 0 $?
}

assertRouteDisabled() {
  local mode=$1
  local scope=$2
  local predicate=$3

  run_route "$mode" "$scope" "$predicate" >/dev/null 2>&1
  assertEquals "$mode/$scope should disable $predicate" 1 $?
}

testJaegerUpgradeRouting() {
  assertRouteDisabled upgrade jaeger deploy_full_stack
  assertRouteDisabled upgrade jaeger deploy_opensearch_stack
  assertRouteEnabled upgrade jaeger deploy_jaeger_stack
  assertRouteDisabled upgrade jaeger deploy_otel_demo
  assertRouteEnabled upgrade jaeger reconcile_ingress
}

testOtelDemoOnlyUpgradeRouting() {
  assertRouteDisabled upgrade otel-demo deploy_full_stack
  assertRouteDisabled upgrade otel-demo deploy_opensearch_stack
  assertRouteDisabled upgrade otel-demo deploy_jaeger_stack
  assertRouteEnabled upgrade otel-demo deploy_otel_demo
  assertRouteDisabled upgrade otel-demo reconcile_ingress
}

testOpenSearchOnlyUpgradeRouting() {
  assertRouteDisabled upgrade opensearch deploy_full_stack
  assertRouteEnabled upgrade opensearch deploy_opensearch_stack
  assertRouteDisabled upgrade opensearch deploy_jaeger_stack
  assertRouteDisabled upgrade opensearch deploy_otel_demo
  assertRouteDisabled upgrade opensearch reconcile_ingress
}

testAllUpgradeRouting() {
  assertRouteEnabled upgrade all deploy_full_stack
  assertRouteEnabled upgrade all deploy_opensearch_stack
  assertRouteEnabled upgrade all deploy_jaeger_stack
  assertRouteEnabled upgrade all deploy_otel_demo
  assertRouteEnabled upgrade all reconcile_ingress
}

testCleanAlwaysUsesFullStackRouting() {
  assertRouteEnabled clean otel-demo deploy_full_stack
  assertRouteEnabled clean otel-demo deploy_opensearch_stack
  assertRouteEnabled clean otel-demo deploy_jaeger_stack
  assertRouteEnabled clean otel-demo deploy_otel_demo
  assertRouteEnabled clean otel-demo reconcile_ingress
}

testInvalidScopeIsRejected() {
  output=$(bash -c 'source "$1"; MODE=upgrade; DEPLOY_SCOPE=invalid; validate_options' _ "$SCRIPT" 2>&1)
  rc=$?

  assertEquals "invalid scope should fail" 1 $rc
  assertContains "$output" "Invalid deploy scope 'invalid'"
}

testInvalidModeIsRejected() {
  output=$(bash -c 'source "$1"; MODE=invalid; DEPLOY_SCOPE=jaeger; validate_options' _ "$SCRIPT" 2>&1)
  rc=$?

  assertEquals "invalid mode should fail" 1 $rc
  assertContains "$output" "Invalid mode 'invalid'"
}

testInvalidOpenSearchRecoveryPolicyIsRejected() {
  output=$(bash -c '
    source "$1"
    MODE=upgrade
    DEPLOY_SCOPE=opensearch
    OPENSEARCH_RECOVERY=invalid
    validate_options
  ' _ "$SCRIPT" 2>&1)
  rc=$?

  assertEquals "invalid recovery policy should fail" 1 $rc
  assertContains "$output" "Invalid OpenSearch recovery policy 'invalid'"
}

testOpenSearchRecoveryWaiverAcceptsOnlyManualMainIsolatedUpgrade() {
  output=$(GITHUB_EVENT_NAME=workflow_dispatch \
    GITHUB_REF=refs/heads/main \
    JAEGER_DEMO_STACK=otel \
    JAEGER_OTEL_DEMO_DEPLOY_SCOPE=opensearch \
    JAEGER_OTEL_DEMO_OPENSEARCH_RECOVERY=waive \
    bash -c 'source "$1"; MODE=upgrade; DEPLOY_SCOPE=opensearch; validate_options' _ "$SCRIPT" 2>&1)
  rc=$?

  assertEquals "exact manual waiver route should pass" 0 $rc
  assertEquals "" "$output"
}

testOpenSearchRecoveryWaiverRejectsOtherRoutes() {
  local cases=(
    "schedule|refs/heads/main|otel|upgrade|opensearch"
    "workflow_dispatch|refs/heads/feature|otel|upgrade|opensearch"
    "workflow_dispatch|refs/heads/main|oci|upgrade|opensearch"
    "workflow_dispatch|refs/heads/main|otel|clean|opensearch"
    "workflow_dispatch|refs/heads/main|otel|upgrade|all"
  )
  local values

  for values in "${cases[@]}"; do
    IFS='|' read -r event ref stack mode scope <<<"$values"
    output=$(GITHUB_EVENT_NAME="$event" \
      GITHUB_REF="$ref" \
      JAEGER_DEMO_STACK="$stack" \
      JAEGER_OTEL_DEMO_DEPLOY_SCOPE="$scope" \
      JAEGER_OTEL_DEMO_OPENSEARCH_RECOVERY=waive \
      bash -c 'source "$1"; MODE="$2"; validate_options' _ "$SCRIPT" "$mode" 2>&1)
    rc=$?

    assertEquals "$values should reject the waiver" 1 $rc
    assertContains "$output" "OpenSearch recovery may be waived only"
  done
}

testMainIsGuardedWhenSourced() {
  output=$(bash -c 'source "$1"; printf sourced-only' _ "$SCRIPT" 2>&1)
  rc=$?

  assertEquals "source should succeed without running main" 0 $rc
  assertEquals "sourced-only" "$output"
}

testPinsCompatibleOtelDemoChart() {
  output=$(bash -c 'source "$1"; printf "%s" "$OTEL_DEMO_CHART_VERSION"' _ "$SCRIPT")
  assertEquals "0.40.9" "$output"
}

testPinnedChartRendersCollectorStartupProbe() {
  local chart_version
  local rendered
  local render_rc
  local startup_probe_count
  local startup_probe
  local liveness_probe
  local readiness_probe
  local resources

  chart_version=$(bash -c \
    'source "$1"; printf "%s" "$OTEL_DEMO_CHART_VERSION"' \
    _ "$SCRIPT")

  rendered=$(helm template otel-demo opentelemetry-demo \
    --repo https://open-telemetry.github.io/opentelemetry-helm-charts \
    --version "$chart_version" \
    --namespace otel-demo \
    --values "$(dirname "$SCRIPT")/otel-demo-values.yaml" \
    --show-only charts/opentelemetry-collector/templates/daemonset.yaml)
  render_rc=$?

  assertEquals "pinned OTel Demo chart should render" 0 "$render_rc"
  [[ "$render_rc" -eq 0 ]] || return

  assertContains "$rendered" "kind: DaemonSet"
  assertContains "$rendered" "name: otel-collector-agent"
  assertContains "$rendered" "- name: opentelemetry-collector"

  startup_probe_count=$(printf '%s\n' "$rendered" | grep -c '^[[:space:]]*startupProbe:$' || true)
  assertEquals "collector should render exactly one startup probe" 1 "$startup_probe_count"

  startup_probe=$(printf '%s\n' "$rendered" |
    sed -n '/^[[:space:]]*startupProbe:$/,/^[[:space:]]*resources:$/p')
  assertContains "$startup_probe" "periodSeconds: 10"
  assertContains "$startup_probe" "timeoutSeconds: 1"
  assertContains "$startup_probe" "failureThreshold: 30"
  assertContains "$startup_probe" "httpGet:"
  assertContains "$startup_probe" "path: /"
  assertContains "$startup_probe" "port: 13133"

  liveness_probe=$(printf '%s\n' "$rendered" |
    sed -n '/^[[:space:]]*livenessProbe:$/,/^[[:space:]]*readinessProbe:$/p')
  assertContains "$liveness_probe" "path: /"
  assertContains "$liveness_probe" "port: 13133"

  readiness_probe=$(printf '%s\n' "$rendered" |
    sed -n '/^[[:space:]]*readinessProbe:$/,/^[[:space:]]*startupProbe:$/p')
  assertContains "$readiness_probe" "path: /"
  assertContains "$readiness_probe" "port: 13133"

  resources=$(printf '%s\n' "$rendered" |
    sed -n '/^[[:space:]]*resources:$/,/^[[:space:]]*volumeMounts:$/p')
  assertContains "$resources" "memory: 200Mi"
}

testPinsCompatibleOpenSearchCharts() {
  output=$(bash -c 'source "$1"; printf "%s|%s" "$OPENSEARCH_CHART_VERSION" "$OPENSEARCH_DASHBOARDS_CHART_VERSION"' _ "$SCRIPT")
  assertEquals "3.7.0|3.7.0" "$output"
}

testDeploysPinnedOpenSearchChartsInOrder() {
  output=$(bash -c '
    source "$1"
    MODE=upgrade
    log() { :; }
    bash() { printf "bash"; printf " %s" "$@"; printf "\n"; }
    helm() { printf "helm"; printf " %s" "$@"; printf "\n"; }
    wait_for_statefulset() { printf "wait-for-statefulset"; printf " %s" "$@"; printf "\n"; }
    wait_for_opensearch_dashboards() { printf "wait-for-dashboards"; printf " %s" "$@"; printf "\n"; }
    deploy_opensearch_releases
  ' _ "$SCRIPT")

  expected="bash $(dirname "$SCRIPT")/opensearch-recovery.sh verify
helm upgrade --install opensearch opensearch/opensearch --namespace opensearch --create-namespace --version 3.7.0 -f $(dirname "$SCRIPT")/opensearch-values.yaml --wait --timeout 10m
wait-for-statefulset opensearch opensearch-cluster-single 600s
helm upgrade --install opensearch-dashboards opensearch/opensearch-dashboards --namespace opensearch --version 3.7.0 -f $(dirname "$SCRIPT")/opensearch-dashboard-values.yaml --wait --timeout 60m"
  expected="$expected
wait-for-dashboards 600s"
  assertEquals "$expected" "$output"
}

testDashboardsHelmTimeoutCanBeOverridden() {
  output=$(OPENSEARCH_DASHBOARDS_HELM_TIMEOUT=75m bash -c '
    source "$1"
    MODE=clean
    log() { :; }
    helm() { printf "helm"; printf " %s" "$@"; printf "\n"; }
    wait_for_statefulset() { :; }
    wait_for_opensearch_dashboards() { :; }
    deploy_opensearch_releases
  ' _ "$SCRIPT")

  assertContains "$output" "helm upgrade --install opensearch-dashboards opensearch/opensearch-dashboards"
  assertContains "$output" "--wait --timeout 75m"
}

testDashboardsHelmFailureCollectsDiagnosticsAndStops() {
  output=$(bash -c '
    source "$1"
    MODE=clean
    log() { :; }
    helm() {
      if [[ "$*" == *"opensearch-dashboards opensearch/opensearch-dashboards"* ]]; then
        return 1
      fi
      return 0
    }
    wait_for_statefulset() { printf "wait-for-statefulset\n"; }
    wait_for_opensearch_dashboards() { printf "unsafe wait-for-deployment\n"; }
    diagnose_opensearch_dashboards_failure() { printf "dashboards diagnostics\n"; }
    deploy_opensearch_releases
  ' _ "$SCRIPT" 2>&1)
  rc=$?

  assertNotEquals "Dashboards Helm failure should fail deployment" 0 "$rc"
  assertContains "$output" "dashboards diagnostics"
  assertEquals "diagnostics should run exactly once" 1 "$(grep -c '^dashboards diagnostics$' <<<"$output")"
  assertContains "$output" "Helm release opensearch-dashboards failed"
  assertNotContains "$output" "unsafe wait-for-deployment"
}

testDashboardsDiagnosticsAreSanitized() {
  output=$(bash -c '
    source "$1"
    log() { printf "log %s\n" "$*"; }
    helm() {
      case "$1" in
        status)
          printf "%s\n" '\''{"version":12,"info":{"status":"failed"},"chart":{"values":{"password":"raw-status-value"}},"app_version":"2.19.6"}'\''
          ;;
        history)
          printf "%s\n" '\''[{"revision":12,"status":"failed","chart":"opensearch-dashboards-2.34.0","app_version":"2.19.6"},{"revision":13,"status":{"value":"failed"},"chart":{"values":{"password":"raw-history-value"}},"app_version":{"token":"raw-app-value"}}]'\''
          ;;
      esac
    }
    kubectl() {
      case "$*" in
        *"get deployment opensearch-dashboards -o json"*)
          printf "%s\n" '\''{"metadata":{"generation":2},"status":{"observedGeneration":2,"replicas":1,"readyReplicas":0,"conditions":[{"type":"Available","status":"False","reason":"MinimumReplicasUnavailable","lastTransitionTime":"2026-08-22T11:45:52Z"}]}}'\''
          ;;
        *"get pods -l app.kubernetes.io/name=opensearch-dashboards,app.kubernetes.io/instance=opensearch-dashboards -o json"*)
          printf "%s\n" '\''{"items":[{"metadata":{"name":"opensearch-dashboards-private-id"},"spec":{"nodeName":"oke-private-node"},"status":{"phase":"Pending","conditions":[{"type":"PodScheduled","status":"False","reason":"Unschedulable"}],"containerStatuses":[{"name":"dashboards","ready":false,"started":false,"restartCount":1,"state":{"waiting":{"reason":"ImagePullBackOff"}},"lastState":{"terminated":{"reason":"Error","exitCode":1}}}]}}]}'\''
          ;;
        *"get events -o json"*)
          printf "%s\n" '\''{"items":[{"involvedObject":{"name":"opensearch-dashboards-private-id"},"type":"Warning","reason":"FailedScheduling","count":2,"lastTimestamp":"2026-08-22T11:46:00Z","message":"private event text"}]}'\''
          ;;
        *"get pods -l app.kubernetes.io/name=opensearch-dashboards,app.kubernetes.io/instance=opensearch-dashboards -o name"*)
          printf "%s\n" "pod/opensearch-dashboards-abc123-def45"
          ;;
        *"logs pod/opensearch-dashboards-abc123-def45 -c dashboards --previous --tail=300"*)
          printf "%s\n" "warning: startup timed out" "Basic dXNlcjpwYXNz" "HTTPS://alice:uRLp4ss@example.invalid/"
          ;;
        *"logs pod/opensearch-dashboards-abc123-def45 -c dashboards --tail=300"*)
          printf "%s\n" "error: connection refused" '\''{"password":"json-value"}'\'' '\''{"scheme":"Bearer eyJhbGciOiJIUzI1NiJ9.opaque"}'\'' "endpoint 10.1.2.3" "endpoint 2001:db8::1" "pod opensearch-dashboards-abc123-def45" "node oke-private-node"
          ;;
      esac
    }
    diagnose_opensearch_dashboards_failure
  ' _ "$SCRIPT" 2>&1)

  assertContains "$output" '"chart":"opensearch-dashboards-2.34.0"'
  assertContains "$output" '"waitingReason":"ImagePullBackOff"'
  assertContains "$output" '"reason":"FailedScheduling"'
  assertContains "$output" "Current Dashboards container log summary (pod 1)"
  assertContains "$output" "lines=7 errors=1 warnings=0 timeouts=0 connection_failures=1 memory_failures=0 readiness_failures=0"
  assertContains "$output" "Previous Dashboards container log summary (pod 1)"
  assertContains "$output" "lines=3 errors=0 warnings=1 timeouts=1 connection_failures=0 memory_failures=0 readiness_failures=0"
  assertNotContains "$output" "dXNlcjpwYXNz"
  assertNotContains "$output" "uRLp4ss"
  assertNotContains "$output" "json-value"
  assertNotContains "$output" "eyJhbGciOiJIUzI1NiJ9.opaque"
  assertNotContains "$output" "10.1.2.3"
  assertNotContains "$output" "2001:db8::1"
  assertNotContains "$output" "opensearch-dashboards-abc123-def45"
  assertNotContains "$output" "opensearch-dashboards-private-id"
  assertNotContains "$output" "oke-private-node"
  assertNotContains "$output" "private event text"
  assertNotContains "$output" "raw-status-value"
  assertNotContains "$output" "raw-history-value"
  assertNotContains "$output" "raw-app-value"
}

testDashboardsRolloutFailureCollectsSanitizedDiagnostics() {
  output=$(bash -c '
    source "$1"
    log() { :; }
    kubectl() { printf "kubectl"; printf " %s" "$@"; printf "\n"; return 1; }
    diagnose_opensearch_dashboards_failure() { printf "dashboards diagnostics\n"; }
    wait_for_opensearch_dashboards 45s
  ' _ "$SCRIPT" 2>&1)
  rc=$?

  assertNotEquals "Dashboards rollout failure should fail deployment" 0 "$rc"
  assertContains "$output" "kubectl rollout status deployment/opensearch-dashboards -n opensearch --timeout=45s"
  assertContains "$output" "dashboards diagnostics"
  assertContains "$output" "OpenSearch Dashboards failed to remain ready"
  assertNotContains "$output" "-o wide"
  assertNotContains "$output" "describe"
}

testRecoveryGatePrecedesOpenSearchUpgrade() {
  output=$(bash -c '
    source "$1"
    MODE=upgrade
    log() { :; }
    bash() { printf "recovery %s %s\n" "$1" "$2"; return 1; }
    helm() { printf "unsafe helm call\n"; }
    deploy_opensearch_releases
  ' _ "$SCRIPT" 2>&1)
  rc=$?

  assertNotEquals "missing recovery point must stop deployment" 0 "$rc"
  assertContains "$output" "recovery $(dirname "$SCRIPT")/opensearch-recovery.sh verify"
  assertNotContains "$output" "unsafe helm call"
}

testRecoveryWaiverSkipsOnlyRecoveryGate() {
  output=$(bash -c '
    source "$1"
    MODE=upgrade
    OPENSEARCH_RECOVERY=waive
    log() { printf "log %s\n" "$*"; }
    bash() { printf "unsafe recovery call\n"; }
    helm() { printf "helm"; printf " %s" "$@"; printf "\n"; }
    wait_for_statefulset() { printf "wait-for-statefulset"; printf " %s" "$@"; printf "\n"; }
    wait_for_opensearch_dashboards() { printf "wait-for-dashboards"; printf " %s" "$@"; printf "\n"; }
    deploy_opensearch_releases
  ' _ "$SCRIPT")

  assertContains "$output" "WARNING: OpenSearch recovery was explicitly waived"
  assertNotContains "$output" "unsafe recovery call"
  assertContains "$output" "helm upgrade --install opensearch opensearch/opensearch"
  assertContains "$output" "wait-for-statefulset opensearch opensearch-cluster-single 600s"
  assertContains "$output" "helm upgrade --install opensearch-dashboards opensearch/opensearch-dashboards"
  assertContains "$output" "wait-for-dashboards 600s"
}

testInvalidRecoveryPolicyCannotCallHelm() {
  output=$(bash -c '
    source "$1"
    MODE=upgrade
    OPENSEARCH_RECOVERY=invalid
    log() { :; }
    helm() { printf "unsafe helm call\n"; }
    deploy_opensearch_releases
  ' _ "$SCRIPT" 2>&1)
  rc=$?

  assertNotEquals "invalid recovery policy must stop deployment" 0 "$rc"
  assertContains "$output" "Invalid OpenSearch recovery policy 'invalid'"
  assertNotContains "$output" "unsafe helm call"
}

# shellcheck disable=SC1091
source "${SHUNIT2}/shunit2"
