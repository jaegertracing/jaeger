#!/bin/bash

# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0
#
# shunit2 tests for retry.sh.

SHUNIT2="${SHUNIT2:?'expecting SHUNIT2 env var pointing to a dir with https://github.com/kward/shunit2 clone'}"

retry="$(cd "$(dirname "$0")" && pwd)/retry.sh"

oneTimeSetUp() {
  TMPDIR_TEST=$(mktemp -d)
}

oneTimeTearDown() {
  rm -rf "$TMPDIR_TEST"
}

testNoArgsPrintsUsage() {
  out=$(bash "$retry" 2>&1)
  rc=$?
  assertEquals "exit 2 on no args" 2 $rc
  assertContains "$out" "usage:"
}

testSucceedsOnFirstAttempt() {
  out=$(ATTEMPTS=3 BACKOFF=0 bash "$retry" true 2>&1)
  rc=$?
  assertEquals "exit 0" 0 $rc
  # No retry lines should be printed when the first attempt succeeds.
  assertNotContains "$out" "sleeping"
}

testFailsAfterAttemptsExhausted() {
  out=$(ATTEMPTS=3 BACKOFF=0 bash "$retry" false 2>&1)
  rc=$?
  assertEquals "exit 1" 1 $rc
  assertContains "$out" "failed after 3 attempts"
  # We expect 2 sleep messages between 3 attempts.
  count=$(echo "$out" | grep -c "sleeping" || true)
  assertEquals "two retries between three attempts" 2 "$count"
}

testSucceedsAfterTransientFailure() {
  # cmd: increment counter file; fail on first two invocations, succeed on third.
  counter="$TMPDIR_TEST/counter-$$"
  echo 0 > "$counter"
  cmd="$TMPDIR_TEST/cmd-$$"
  cat > "$cmd" <<EOF
#!/bin/bash
n=\$(cat "$counter")
n=\$((n + 1))
echo \$n > "$counter"
[ "\$n" -ge 3 ]
EOF
  chmod +x "$cmd"
  out=$(ATTEMPTS=5 BACKOFF=0 bash "$retry" "$cmd" 2>&1)
  rc=$?
  assertEquals "exit 0" 0 $rc
  assertEquals "three attempts ran" 3 "$(cat "$counter")"
  rm -f "$counter" "$cmd"
}

testAttemptsEnvSingleTry() {
  out=$(ATTEMPTS=1 BACKOFF=0 bash "$retry" false 2>&1)
  rc=$?
  assertEquals "exit 1" 1 $rc
  assertContains "$out" "failed after 1 attempts"
  count=$(echo "$out" | grep -c "sleeping" || true)
  assertEquals "no sleeps with ATTEMPTS=1" 0 "$count"
}

testRejectsNonNumericAttempts() {
  out=$(ATTEMPTS=foo BACKOFF=0 bash "$retry" true 2>&1)
  rc=$?
  assertEquals "exit 2" 2 $rc
  assertContains "$out" "ATTEMPTS must be a non-negative integer"
}

testRejectsNonNumericBackoff() {
  out=$(ATTEMPTS=3 BACKOFF=bar bash "$retry" true 2>&1)
  rc=$?
  assertEquals "exit 2" 2 $rc
  assertContains "$out" "BACKOFF must be a non-negative integer"
}

testRejectsZeroAttempts() {
  out=$(ATTEMPTS=0 BACKOFF=0 bash "$retry" true 2>&1)
  rc=$?
  assertEquals "exit 2" 2 $rc
  assertContains "$out" "ATTEMPTS must be at least 1"
}

# shellcheck disable=SC1091
source "${SHUNIT2}/shunit2"
