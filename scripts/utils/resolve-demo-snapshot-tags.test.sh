#!/bin/bash

# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

SHUNIT2="${SHUNIT2:?'expecting SHUNIT2 env var pointing to a dir with https://github.com/kward/shunit2 clone'}"

SCRIPT="$(cd "$(dirname "$0")" && pwd)/resolve-demo-snapshot-tags.sh"
REPO="jaegertracing/jaeger-snapshot"
PREFERRED="1111111111111111111111111111111111111111"
NEWEST="aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
MAIN_SHA="2222222222222222222222222222222222222222"
PUBLISHED="bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
OLDER="cccccccccccccccccccccccccccccccccccccccc"

oneTimeSetUp() {
  TMPDIR_TEST=$(mktemp -d)
  EMPTY_BIN="$TMPDIR_TEST/empty-bin"
  mkdir -p "$EMPTY_BIN"
  MOCK_CURL="$TMPDIR_TEST/mock-curl.sh"
  cat > "$MOCK_CURL" <<EOF
#!/usr/bin/env bash
set -eu
outfile=""
write_fmt=""
url=""

while [[ \$# -gt 0 ]]; do
  case "\$1" in
    -o) outfile=\$2; shift 2 ;;
    -w) write_fmt=\$2; shift 2 ;;
    -*) shift ;;
    *) url=\$1; shift ;;
  esac
done

emit() {
  local status=\$1
  local body=\$2
  if [[ -n "\$outfile" ]]; then
    printf '%s' "\$body" > "\$outfile"
  fi
  if [[ -n "\$write_fmt" ]]; then
    printf '%s' "\$status"
  fi
}

case "\$url" in
  */tags/${PUBLISHED}/)
    emit "200" '{"digest":"sha256:published"}'
    ;;
  */tags/${PREFERRED}/)
    emit "404" '{}'
    ;;
  */tags/${MAIN_SHA}/)
    emit "404" '{}'
    ;;
  */tags/${NEWEST}/)
    emit "200" '{"digest":"sha256:newest"}'
    ;;
  */tags/?page_size=*ordering=last_updated*)
    emit "200" '{"results":[{"name":"latest"},{"name":"'${NEWEST}'"},{"name":"'${OLDER}'"}]}'
    ;;
  */tags/?page_size=*)
    emit "500" '{"error":"unexpected ordering"}'
    ;;
  *)
    emit "500" '{}'
    ;;
esac
EOF
  chmod +x "$MOCK_CURL"
}

oneTimeTearDown() {
  rm -rf "$TMPDIR_TEST"
}

run_script() {
  env \
    DOCKERHUB_CURL="$MOCK_CURL" \
    MAIN_SHA="$MAIN_SHA" \
    JAEGER_DEMO_JAEGER_IMAGE_REPOSITORY="$REPO" \
    JAEGER_DEMO_HOTROD_IMAGE_REPOSITORY="$REPO" \
    "$@" \
    bash "$SCRIPT"
}

testRequiresMainSha() {
  err=$(env -i bash "$SCRIPT" 2>&1)
  rc=$?
  assertEquals "exit 1 without MAIN_SHA" 1 $rc
  assertContains "$err" "MAIN_SHA or GITHUB_SHA must be set"
}

testRequiresJq() {
  err=$(env -i PATH="$EMPTY_BIN" MAIN_SHA="$MAIN_SHA" /bin/bash "$SCRIPT" 2>&1)
  rc=$?
  assertEquals "exit 1 without jq" 1 $rc
  assertContains "$err" "jq is required but not installed"
}

testUsesPreferredTagWhenPublished() {
  out=$(run_script \
    GITHUB_EVENT_NAME=schedule \
    JAEGER_DEMO_JAEGER_IMAGE_TAG="$PUBLISHED" \
    JAEGER_DEMO_HOTROD_IMAGE_TAG="$PUBLISHED" 2>&1)
  rc=$?
  assertEquals "exit 0" 0 $rc
  assertContains "$out" "Jaeger tag=$PUBLISHED"
  assertContains "$out" "HotROD tag=$PUBLISHED"
}

testScheduledFallsBackToNewestPublishedSha() {
  out=$(run_script \
    GITHUB_EVENT_NAME=schedule \
    JAEGER_DEMO_JAEGER_IMAGE_TAG="$PREFERRED" \
    JAEGER_DEMO_HOTROD_IMAGE_TAG="$PREFERRED" 2>&1)
  rc=$?
  assertEquals "exit 0" 0 $rc
  assertContains "$out" "Jaeger tag=$NEWEST"
  assertContains "$out" "HotROD tag=$NEWEST"
  assertContains "$out" "Main HEAD ${MAIN_SHA} has no published Jaeger snapshot"
}

testManualDispatchFailsWhenTagMissing() {
  out=$(run_script \
    GITHUB_EVENT_NAME=workflow_dispatch \
    JAEGER_DEMO_JAEGER_IMAGE_TAG="$PREFERRED" \
    JAEGER_DEMO_HOTROD_IMAGE_TAG="$PREFERRED" 2>&1)
  rc=$?
  assertEquals "exit 1" 1 $rc
  assertContains "$out" "Snapshot image tag not found"
}

# shellcheck disable=SC1091
source "${SHUNIT2}/shunit2"
