#!/bin/bash

# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

SHUNIT2="${SHUNIT2:?'expecting SHUNIT2 env var pointing to a dir with https://github.com/kward/shunit2 clone'}"

DIR="$(cd "$(dirname "$0")" && pwd)"
VALUES="$DIR/jaeger-values.yaml"
CONFIG="$DIR/jaeger-config.yaml"
# shellcheck source=scripts/demo/jaeger-chart.sh
source "$DIR/../../scripts/demo/jaeger-chart.sh"

oneTimeSetUp() {
  CHART_DIR="${JAEGER_TEST_HELM_CHART:-}"
  if [[ -z "$CHART_DIR" ]]; then
    CHART_TEMP=$(mktemp -d)
    git clone --quiet --no-checkout https://github.com/jaegertracing/helm-charts.git "$CHART_TEMP/helm-charts" || return 1
    git -C "$CHART_TEMP/helm-charts" checkout --quiet --detach "$JAEGER_CHART_COMMIT" || return 1
    CHART_DIR="$CHART_TEMP/helm-charts/charts/jaeger"
    export HELM_REPOSITORY_CONFIG="$CHART_TEMP/repositories.yaml"
    export HELM_REPOSITORY_CACHE="$CHART_TEMP/repository"
    helm repo add incubator https://charts.helm.sh/incubator >/dev/null || return 1
    helm repo add bitnami https://charts.bitnami.com/bitnami >/dev/null || return 1
    helm dependency build --skip-refresh "$CHART_DIR" >/dev/null || return 1
  fi
  [[ "$(git -C "$CHART_DIR" rev-parse HEAD)" == "$JAEGER_CHART_COMMIT" ]] || return 1
  RENDERED=$(helm template jaeger "$CHART_DIR" --namespace jaeger \
    --set storage.type=elasticsearch \
    --set goose.image=example.invalid/goose:test \
    --set-file userconfig="$CONFIG" -f "$VALUES") || return 1
}

oneTimeTearDown() {
  if [[ -n "${CHART_TEMP:-}" ]]; then
    rm -rf "$CHART_TEMP"
  fi
}

# value_in_block KEY_PATH FILE prints the value of a scalar nested under
# top-level key "$1" at the given indentation, e.g. value_in_block esIndexCleaner "  numberOfDays".
value_in_block() {
  local block=$1 key=$2 file=$3
  awk -v block="$block" -v key="$key" '
    $0 ~ "^"block":" { in_block = 1; next }
    in_block && /^[^ #]/ { in_block = 0 }
    in_block && index($0, key":") == 1 {
      sub(key":[ ]*", ""); gsub(/"/, ""); print; exit
    }
  ' "$file"
}

testIndexCleanerIsEnabled() {
  assertEquals "true" "$(value_in_block esIndexCleaner "  enabled" "$VALUES")"
}

testRetentionIs30Days() {
  assertEquals "30" "$(value_in_block esIndexCleaner "  numberOfDays" "$VALUES")"
}

testCleanerImageTagIsPinnedToPublishedRelease() {
  tag=$(awk '/^esIndexCleaner:/{b=1;next} b&&/^[^ #]/{b=0} b&&/^    tag:/{gsub(/"/,"",$2);print $2}' "$VALUES")
  local version=${tag%%@*}
  local digest=${tag#*@}
  assertTrue "tag '$tag' should contain a release version" "[[ '$version' =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]"
  assertTrue "tag '$tag' should contain a sha256 digest" "[[ '$digest' =~ ^sha256:[a-f0-9]{64}$ ]]"
}

# Pinned es-index-cleaner 2.21.0 appends "-" to every nonempty prefix and
# matches "<prefix>jaeger-(span|service|...)-<date>", so the value here must
# equal the index_prefix Jaeger writes with.
testCleanerPrefixMatchesJaegerIndexPrefix() {
  written=$(awk '/index_prefix:/{gsub(/"/,"",$2);print $2;exit}' "$CONFIG")
  cleaned=$(value_in_block esIndexCleaner "    index-prefix" "$VALUES")
  assertNotNull "index_prefix should be set in jaeger-config.yaml" "$written"
  assertEquals "jaeger-main" "$written"
  assertEquals "$written" "$cleaned"
}

# The Jaeger indices use the daily date layout the cleaner expects by default.
testJaegerWritesDailyIndicesWithDefaultDateLayout() {
  assertEquals "$(grep -c 'date_layout:' "$CONFIG")" "$(grep -c 'date_layout: "2006-01-02"' "$CONFIG")"
  assertEquals "$(grep -c 'rollover_frequency:' "$CONFIG")" "$(grep -c 'rollover_frequency: "day"' "$CONFIG")"
}

testQueryReadsUIConfigFromChartMount() {
  assertEquals "/etc/jaeger/ui-config/ui-config.json" "$(value_in_block extensions "      config_file" "$CONFIG")"
}

testGooseImageMustBeConfiguredBeforeDeployment() {
  output=$(helm template jaeger "$CHART_DIR" --namespace jaeger \
    --set storage.type=elasticsearch -f "$VALUES" 2>&1)
  assertNotEquals "an unconfigured image must fail before Helm upgrade" 0 "$?"
  assertContains "$output" "Set goose.image"
}

rendered_object() {
  printf '%s\n' "$RENDERED" | awk -v kind="$1" -v name="$2" '
    /^---$/ { if (matched_kind && matched_name) printf "%s", doc; doc = ""; matched_kind = 0; matched_name = 0; next }
    { doc = doc $0 "\n" }
    $0 == "kind: " kind { matched_kind = 1 }
    $0 == "  name: " name { matched_name = 1 }
    END { if (matched_kind && matched_name) printf "%s", doc }
  '
}

testGooseIsPrivateAndHasNoKubernetesCredential() {
  local service policy deployment
  service=$(rendered_object Service jaeger-goose)
  policy=$(rendered_object NetworkPolicy jaeger-goose)
  deployment=$(rendered_object Deployment jaeger-goose)
  assertNotNull "Goose Service must exist" "$service"
  assertNotNull "Goose NetworkPolicy must exist" "$policy"
  assertNotNull "Goose Deployment must exist" "$deployment"
  assertContains "$service" "type: ClusterIP"
  assertContains "$service" "app.kubernetes.io/name: jaeger-goose"
  assertContains "$service" "port: 16688"
  assertContains "$policy" "app.kubernetes.io/name: jaeger-goose"
  assertContains "$policy" "policyTypes:"
  assertContains "$policy" "- Ingress"
  assertContains "$policy" "app.kubernetes.io/instance: 'jaeger'"
  assertContains "$policy" "app.kubernetes.io/component: all-in-one"
  assertContains "$policy" "port: 16688"
  assertContains "$deployment" "automountServiceAccountToken: false"
  assertNotContains "$deployment" "hostNetwork: true"
  assertNotContains "$service" "type: LoadBalancer"
}

testOnlyJaegerMayStartWithoutTheAgentSecret() {
  local jaeger goose
  jaeger=$(rendered_object Deployment jaeger)
  goose=$(rendered_object Deployment jaeger-goose)
  assertContains "$jaeger" 'key: GOOSE_SERVER__SECRET_KEY
              optional: true'
  assertContains "$goose" 'key: GOOSE_SERVER__SECRET_KEY'
  assertContains "$goose" 'key: OPENAI_API_KEY'
  assertNotContains "$goose" 'optional: true'
}

testGooseUsesBoundedDirectOpenAiRequests() {
  assertContains "$RENDERED" "- name: GOOSE_PROVIDER
          value: openai"
  assertContains "$RENDERED" "- name: GOOSE_MODEL
          value: gpt-6-luna"
  assertContains "$RENDERED" '- name: GOOSE_THINKING_EFFORT
          value: "off"'
  assertContains "$RENDERED" '- name: GOOSE_MAX_TURNS
          value: "8"'
  assertContains "$RENDERED" '- name: GOOSE_MAX_TOKENS
          value: "4096"'
  assertContains "$RENDERED" '- name: GOOSE_DISABLE_SESSION_NAMING
          value: "true"'
  assertNotContains "$RENDERED" "OPENROUTER_API_KEY"
}

testGooseStartsDisabledUntilRolloutPrerequisitesPass() {
  assertContains "$(rendered_object Deployment jaeger-goose)" 'replicas: 0'
}

testBothProcessesReadTheAgentSecret() {
  assertEquals "Jaeger and Goose need the same agent credential" 2 \
    "$(printf '%s\n' "$RENDERED" | grep -c 'key: GOOSE_SERVER__SECRET_KEY')"
  assertEquals "only Goose gets the provider key" 1 \
    "$(printf '%s\n' "$RENDERED" | grep -c 'key: OPENAI_API_KEY')"
  assertContains "$RENDERED" "X-Secret-Key: \${env:GOOSE_SERVER__SECRET_KEY}"
  assertContains "$RENDERED" "agent_url: ws://jaeger-goose:16688/acp"
  assertContains "$RENDERED" "base_url: http://jaeger-query-clusterip:16686"
}

testWritableRuntimePreservesDisabledExtensionConfiguration() {
  assertContains "$RENDERED" "name: goose-config"
  assertContains "$RENDERED" "cp -R /tmp/.config/goose /runtime/.config/"
  assertContains "$RENDERED" "mountPath: /tmp"
  assertContains "$RENDERED" "readOnlyRootFilesystem: true"
}

testChatLimitsDoNotCoverBrowsingOrMcp() {
  local ingress="$DIR/ingress/ingress-jaeger.yaml"
  local chat
  local browsing
  chat=$(sed -n '/^  name: jaeger-demo-ai-ingress$/,$p' "$ingress")
  browsing=$(sed '/^---$/,$d' "$ingress")
  assertContains "$chat" 'nginx.ingress.kubernetes.io/limit-rpm: "10"'
  assertContains "$chat" 'nginx.ingress.kubernetes.io/limit-connections: "2"'
  assertContains "$chat" 'nginx.ingress.kubernetes.io/limit-burst-multiplier: "1"'
  assertContains "$chat" 'nginx.ingress.kubernetes.io/limit-allowlist: ""'
  assertContains "$chat" 'nginx.ingress.kubernetes.io/limit-whitelist: ""'
  assertContains "$chat" 'nginx.ingress.kubernetes.io/proxy-buffering: "off"'
  assertContains "$chat" "path: /api/ai/chat"
  assertContains "$chat" "pathType: Exact"
  assertNotContains "$chat" "/api/ai/mcp"
  assertNotContains "$browsing" "limit-rpm"
  assertNotContains "$browsing" "limit-connections"
}

# shellcheck disable=SC1091
source "$SHUNIT2/shunit2"
