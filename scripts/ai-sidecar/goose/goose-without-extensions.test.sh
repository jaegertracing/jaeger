#!/bin/bash

# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0
#
# shunit2 tests for goose-without-extensions.sh. They run the script against a
# fake goose on the PATH that reports its configuration the way "goose info -v"
# does: the config file under the home directory when there is one, and
# otherwise the defaults in FAKE_GOOSE_DEFAULTS.

SHUNIT2="${SHUNIT2:?'expecting SHUNIT2 env var pointing to a dir with https://github.com/kward/shunit2 clone'}"

script="$(cd "$(dirname "$0")" && pwd)/goose-without-extensions.sh"

oneTimeSetUp() {
  fake_bin=$(mktemp -d)
  cat > "$fake_bin/goose" <<'EOF'
#!/bin/sh
if [ "$1" != "info" ]; then
  echo "goose started with: $*"
  exit 0
fi
if [ -n "${FAKE_GOOSE_INFO_FAILS:-}" ]; then
  exit 1
fi
config="$HOME/.config/goose/config.yaml"
echo "goose Version:"
echo "  Version:                  0.0.0"
echo
echo "Paths:"
echo "Config yaml:             $config"
echo
echo "goose Configuration:"
if [ -f "$config" ] && [ -z "${FAKE_GOOSE_IGNORES_CONFIG_FILE:-}" ]; then
  sed 's/^/  /' "$config"
else
  printf '%s\n' "$FAKE_GOOSE_DEFAULTS" | sed 's/^/  /'
fi
EOF
  chmod +x "$fake_bin/goose"
}

oneTimeTearDown() {
  rm -rf "$fake_bin"
}

setUp() {
  HOME=$(mktemp -d)
  export HOME
  config="$HOME/.config/goose/config.yaml"
  export FAKE_GOOSE_DEFAULTS='GOOSE_PROVIDER: openrouter
providers:
  acp-agent:
    enabled: true
extensions:
  developer:
    enabled: true
    type: platform
  todo:
    enabled: false
    type: platform
  apps:
    enabled: true
    type: platform
scheduler:
  enabled: true'
  unset FAKE_GOOSE_INFO_FAILS FAKE_GOOSE_IGNORES_CONFIG_FILE
}

tearDown() {
  rm -rf "$HOME"
}

run_script() {
  PATH="$fake_bin:$PATH" sh "$script" "$@" 2>&1
}

testDisablesEveryExtension() {
  out=$(run_script serve)
  assertEquals "exit 0" 0 $?
  extensions=$(sed -n '/^extensions:/,/^[^ ]/p' "$config")
  assertEquals "three extensions disabled" 3 "$(echo "$extensions" | grep -c '^    enabled: false$')"
  assertNotContains "$extensions" "    enabled: true"
}

testLeavesOtherSectionsUnchanged() {
  out=$(run_script serve)
  assertEquals "exit 0" 0 $?
  expected='GOOSE_PROVIDER: openrouter
providers:
  acp-agent:
    enabled: true
extensions:
  developer:
    enabled: false
    type: platform
  todo:
    enabled: false
    type: platform
  apps:
    enabled: false
    type: platform
scheduler:
  enabled: true'
  assertEquals "$expected" "$(cat "$config")"
}

testRunsGooseWithItsArguments() {
  out=$(run_script serve --host 0.0.0.0 --port 16688)
  assertEquals "exit 0" 0 $?
  assertEquals "goose started with: serve --host 0.0.0.0 --port 16688" "$out"
}

testFailsWhenGooseStillReportsAnEnabledExtension() {
  export FAKE_GOOSE_IGNORES_CONFIG_FILE=1
  out=$(run_script serve)
  assertEquals "exit 1" 1 $?
  assertContains "$out" "FAIL: Goose does not report every bundled extension as disabled"
  assertNotContains "$out" "goose started"
}

testFailsWhenGooseReportsNoExtensions() {
  export FAKE_GOOSE_DEFAULTS='GOOSE_PROVIDER: openrouter'
  out=$(run_script serve)
  assertEquals "exit 1" 1 $?
  assertContains "$out" "FAIL: Goose does not report every bundled extension as disabled"
  assertNotContains "$out" "goose started"
}

testFailsWhenGooseInfoFails() {
  export FAKE_GOOSE_INFO_FAILS=1
  out=$(run_script serve)
  assertNotEquals "non-zero exit" 0 $?
  assertNotContains "$out" "goose started"
}

# shellcheck disable=SC1091
source "${SHUNIT2}/shunit2"
