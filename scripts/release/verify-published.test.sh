#!/bin/bash

# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0
#
# shunit2 tests for verify-published.sh, with gh, docker and go stubbed out.

SHUNIT2="${SHUNIT2:?'expecting SHUNIT2 env var pointing to a dir with https://github.com/kward/shunit2 clone'}"

script="$(cd "$(dirname "$0")" && pwd)/verify-published.sh"
TAG_SHA=462bb49fcff9af4be9556bf0206d07f5171bd8d7

setUp() {
  T=$(mktemp -d)
  mkdir -p "$T/bin" "$T/pkg/jaeger-2.22.0-linux-amd64" "$T/pkg/jaeger-tools-2.22.0-linux-amd64"
  echo jaeger > "$T/pkg/jaeger-2.22.0-linux-amd64/jaeger"
  echo hotrod > "$T/pkg/jaeger-2.22.0-linux-amd64/example-hotrod"
  echo rollover > "$T/pkg/jaeger-tools-2.22.0-linux-amd64/jaeger-es-rollover"
  tar -czf "$T/jaeger-2.22.0-linux-amd64.tar.gz" -C "$T/pkg" jaeger-2.22.0-linux-amd64
  tar -czf "$T/jaeger-tools-2.22.0-linux-amd64.tar.gz" -C "$T/pkg" jaeger-tools-2.22.0-linux-amd64
  # The asset names of the real v2.22.0 release.
  real_assets > "$T/assets"
  : > "$T/missing-images"
  echo "$TAG_SHA" > "$T/jaeger.rev"
  echo "$TAG_SHA" > "$T/jaeger-es-rollover.rev"
  : > "$T/example-hotrod.rev"
  export T TAG_SHA ATTEMPTS=1 BACKOFF=0 GITHUB_STEP_SUMMARY="$T/summary"
  ORIG_PATH="$PATH"
  export PATH="$T/bin:$PATH"
  # shellcheck disable=SC2016
  stub gh 'case "$1 $2" in
    "release view") cat "$T/assets";;
    "api repos/jaegertracing/jaeger/commits/v2.22.0") echo "$TAG_SHA";;
    "release download") cp "$T/$7" "$9/";;
    *) echo "unexpected gh $*" >&2; exit 1;;
  esac'
  # shellcheck disable=SC2016
  stub docker '! grep -Fxq "$3" "$T/missing-images"'
  # shellcheck disable=SC2016
  stub go 'rev=$(cat "$T/$(basename "$3").rev"); [ -z "$rev" ] || printf "\tbuild\tvcs.revision=%s\n" "$rev"'
}

tearDown() {
  export PATH="$ORIG_PATH"
  rm -rf "$T"
}

stub() {
  printf '#!/bin/bash\n%s\n' "$2" > "$T/bin/$1"
  chmod +x "$T/bin/$1"
}

real_assets() {
  echo jaeger-2.22.0.sha256sum.txt
  echo jaeger-SBOM.spdx.json
  echo multiple.intoto.jsonl
  for p in darwin-amd64 darwin-arm64 linux-amd64 linux-arm64 linux-ppc64le linux-s390x windows-amd64; do
    for pkg in jaeger jaeger-tools; do
      printf '%s\n' "$pkg-2.22.0-$p.sha256sum.txt" "$pkg-2.22.0-$p.tar.gz" "$pkg-2.22.0-$p.tar.gz.asc"
    done
  done
  printf '%s\n' jaeger-2.22.0-windows-amd64.zip jaeger-2.22.0-windows-amd64.zip.asc \
    jaeger-tools-2.22.0-windows-amd64.zip jaeger-tools-2.22.0-windows-amd64.zip.asc
}

testCompleteReleasePasses() {
  assertEquals "fixture should match the 49 assets of v2.22.0" 49 "$(wc -l < "$T/assets" | tr -d ' ')"
  out=$(bash "$script" v2.22.0 2>&1)
  assertEquals "verify failed: $out" 0 $?
  assertContains "$out" "All expected release assets and images are published."
  assertContains "$out" "found quay.io/jaegertracing/jaeger-remote-storage-debug:2.22.0"
  assertContains "$out" "found docker.io/jaegertracing/example-hotrod:2.22.0"
  assertContains "$out" "example-hotrod records no vcs.revision, skipped"
  assertNotContains "$out" "::warning::"
}

testMissingAssetAndImageFail() {
  grep -v '^jaeger-tools-2.22.0-linux-s390x.tar.gz.asc$' "$T/assets" > "$T/a" && mv "$T/a" "$T/assets"
  echo quay.io/jaegertracing/jaeger-es-rollover:2.22.0 > "$T/missing-images"
  out=$(bash "$script" v2.22.0 2>&1)
  assertEquals 1 $?
  assertContains "$out" "::error::Missing release asset jaeger-tools-2.22.0-linux-s390x.tar.gz.asc"
  assertContains "$out" "::error::Missing image quay.io/jaegertracing/jaeger-es-rollover:2.22.0"
  assertContains "$(cat "$T/summary")" "- image quay.io/jaegertracing/jaeger-es-rollover:2.22.0"
}

testCommitMismatchOnlyWarns() {
  echo d5a26080d5a26080d5a26080d5a26080d5a26080 > "$T/jaeger-es-rollover.rev"
  out=$(bash "$script" v2.22.0 2>&1)
  assertEquals "mismatch should not fail: $out" 0 $?
  assertContains "$out" "::warning::jaeger-es-rollover was built from d5a26080d5a26080d5a26080d5a26080d5a26080, not v2.22.0"
  assertContains "$out" "jaeger built from v2.22.0"
}

# shellcheck disable=SC1091
source "${SHUNIT2}/shunit2"
