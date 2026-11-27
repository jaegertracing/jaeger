#!/bin/bash

# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

SHUNIT2="${SHUNIT2:?'expecting SHUNIT2 env var pointing to a shunit2 clone'}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
REAL_BASH="$(command -v bash)"
export REAL_BASH

setUp() {
  ORIGINAL_PATH="$PATH"
  TEST_DIR="$(mktemp -d)"
  export COMMAND_LOG="$TEST_DIR/commands" MANIFEST="$TEST_DIR/manifest"
  mkdir "$TEST_DIR/bin"
  : > "$COMMAND_LOG"
  cat > "$TEST_DIR/bin/kubectl" <<'STUB'
#!/bin/bash
printf 'kubectl %s\n' "$*" >> "$COMMAND_LOG"
if [[ "$1" == apply ]]; then
  cat > "$MANIFEST"
  exit "${APPLY_STATUS:-0}"
fi
if [[ "$*" == *" logs "* ]]; then
  exit "${LOG_STATUS:-0}"
fi
STUB
  chmod +x "$TEST_DIR/bin/kubectl"
  export PATH="$TEST_DIR/bin:$ORIGINAL_PATH"
}

tearDown() {
  export PATH="$ORIGINAL_PATH"
  unset APPLY_STATUS LOG_STATUS
  rm -rf "$TEST_DIR"
}

testRenderUsesLocalImagesAndIndependentConfiguration() {
  output=$("$REAL_BASH" "$SCRIPT_DIR/deploy.sh" render test-sha)
  assertEquals 0 "$?"
  assertContains "$output" 'localhost:5000/jaegertracing/jaeger:test-sha'
  assertContains "$output" 'localhost:5000/jaegertracing/example-hotrod:test-sha'
  assertEquals 2 "$(printf '%s\n' "$output" | grep -c 'imagePullPolicy: Never')"
  assertContains "$output" 'config.yaml: |'
  assertContains "$output" 'ui-config.json: |'
  assertContains "$output" 'max_traces: 10000'
  assertContains "$output" 'base_path: /jaeger'
  assertNotContains "$output" '__IMAGE_TAG__'
  assertNotContains "$output" 'prometheus'
  assertEquals 'render must not access the cluster' '' "$(cat "$COMMAND_LOG")"
}

testInvalidOperationAndTagsFailBeforeClusterAccess() {
  output=$("$REAL_BASH" "$SCRIPT_DIR/deploy.sh" 2>&1)
  assertEquals 1 "$?"
  assertContains "$output" 'Usage:'
  for tag in '' 'bad/tag' '-leading-dash' 'tag:other' 'tag&replacement' $'tag\nkind: Namespace'; do
    output=$("$REAL_BASH" "$SCRIPT_DIR/deploy.sh" apply "$tag" 2>&1)
    assertEquals "invalid tag $tag must fail" 1 "$?"
    assertContains "$output" 'A valid local image tag is required'
  done
  assertEquals '' "$(cat "$COMMAND_LOG")"
}

testApplyPassesTheRenderedManifest() {
  "$REAL_BASH" "$SCRIPT_DIR/deploy.sh" apply test-sha
  assertEquals 0 "$?"
  "$REAL_BASH" "$SCRIPT_DIR/deploy.sh" render test-sha > "$TEST_DIR/expected"
  diff -u "$TEST_DIR/expected" "$MANIFEST"
  assertEquals 'apply must use the complete render' 0 "$?"
  assertEquals 'kubectl apply -f -' "$(cat "$COMMAND_LOG")"
}

testApplyPropagatesClusterFailure() {
  export APPLY_STATUS=7
  "$REAL_BASH" "$SCRIPT_DIR/deploy.sh" apply test-sha
  assertEquals 7 "$?"
}

testDeleteTargetsOnlyFixtureNamespace() {
  "$REAL_BASH" "$SCRIPT_DIR/deploy.sh" delete
  assertEquals 0 "$?"
  assertEquals 'kubectl delete namespace hotrod-test --ignore-not-found' "$(cat "$COMMAND_LOG")"
}

mock_build_commands() {
  cat > "$TEST_DIR/bin/bash" <<'STUB'
#!/bin/bash
printf 'bash %s\n' "$*" >> "$COMMAND_LOG"
case "$1" in
  ./examples/hotrod/kubernetes/deploy.sh) exec "$REAL_BASH" "$@" ;;
  scripts/build/build-upload-a-docker-image.sh|scripts/utils/retry.sh) exit 0 ;;
  *) echo "Unexpected child script: $1" >&2; exit 1 ;;
esac
STUB
  cat > "$TEST_DIR/bin/make" <<'STUB'
#!/bin/bash
if [[ "$1" == echo-linux-platforms ]]; then echo linux/amd64; fi
STUB
  cat > "$TEST_DIR/bin/go" <<'STUB'
#!/bin/bash
case "$2" in GOOS) echo linux ;; GOARCH) echo amd64 ;; esac
STUB
  cat > "$TEST_DIR/bin/kind" <<'STUB'
#!/bin/bash
printf 'kind %s\n' "$*" >> "$COMMAND_LOG"
if [[ "$1 $2" == 'get clusters' ]]; then echo hotrod-test; fi
STUB
  cat > "$TEST_DIR/bin/docker" <<'STUB'
#!/bin/bash
printf 'docker %s\n' "$*" >> "$COMMAND_LOG"
STUB
  cat > "$TEST_DIR/bin/helm" <<'STUB'
#!/bin/bash
printf 'helm %s\n' "$*" >> "$COMMAND_LOG"
echo 'The core fixture must not use Helm' >&2
exit 1
STUB
  cat > "$TEST_DIR/bin/curl" <<'STUB'
#!/bin/bash
case "$*" in
  *http_code*) printf 200 ;;
  *dispatch*) echo 'Traceresponse: 00-0123456789abcdef0123456789abcdef-0123456789abcdef-01' ;;
  *api/v3/traces*) echo '{}' ;;
  *) echo 'Rides On Demand' ;;
esac
STUB
  cat > "$TEST_DIR/bin/jq" <<'STUB'
#!/bin/bash
cat >/dev/null
echo 35
STUB
  chmod +x "$TEST_DIR/bin/"*
}

run_image_test() {
  (
    cd "$REPO_ROOT" || exit 1
    GITHUB_SHA=test-sha "$REAL_BASH" scripts/build/build-hotrod-image.sh -l -p linux/amd64 -r "$1"
  ) > "$TEST_DIR/build.log" 2>&1
}

testKubernetesBuildUsesFixtureAndCleansUp() {
  mock_build_commands
  run_image_test k8s
  result=$?
  assertEquals "$(cat "$TEST_DIR/build.log")" 0 "$result"
  commands=$(cat "$COMMAND_LOG")
  assertContains "$commands" 'bash ./examples/hotrod/kubernetes/deploy.sh apply test-sha'
  assertContains "$commands" 'kubectl --namespace hotrod-test wait --for=condition=available --timeout=180s deployment/jaeger'
  assertContains "$commands" 'kubectl --namespace hotrod-test wait --for=condition=available --timeout=180s deployment/jaeger-hotrod'
  assertContains "$commands" 'kubectl --namespace hotrod-test port-forward svc/jaeger-query 16686:16686'
  assertContains "$commands" 'kubectl --namespace hotrod-test port-forward svc/jaeger-hotrod 8080:80'
  assertContains "$commands" 'kubectl delete namespace hotrod-test --ignore-not-found'
  assertNotContains "$commands" 'examples/oci'
  assertNotContains "$commands" 'helm '
  assertContains "$commands" 'bash scripts/build/build-upload-a-docker-image.sh -l -c example-hotrod -d examples/hotrod -p linux/amd64'
}

testKubernetesFailureDumpsLogsAndCleansUpWithoutPublishing() {
  mock_build_commands
  export APPLY_STATUS=7 LOG_STATUS=9
  run_image_test k8s
  assertEquals 7 "$?"
  commands=$(cat "$COMMAND_LOG")
  assertContains "$commands" 'kubectl --namespace hotrod-test logs -l app.kubernetes.io/name=jaeger'
  assertContains "$commands" 'kubectl --namespace hotrod-test logs -l app.kubernetes.io/name=hotrod'
  assertContains "$commands" 'kubectl delete namespace hotrod-test --ignore-not-found'
  assertEquals 'only the initial local HotROD build may run on failure' 1 "$(grep -c 'build-upload-a-docker-image.sh -l -c example-hotrod' "$COMMAND_LOG")"
}

testDockerBuildKeepsComposePath() {
  mock_build_commands
  run_image_test docker
  result=$?
  assertEquals "$(cat "$TEST_DIR/build.log")" 0 "$result"
  commands=$(cat "$COMMAND_LOG")
  assertContains "$commands" 'docker compose -f ./examples/hotrod/docker-compose.yml up -d'
  assertContains "$commands" 'docker compose -f ./examples/hotrod/docker-compose.yml down'
  assertNotContains "$commands" 'kubectl '
  assertNotContains "$commands" 'kubernetes/deploy.sh'
}

# shellcheck source=/dev/null
. "$SHUNIT2/shunit2"
