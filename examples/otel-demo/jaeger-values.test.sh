#!/bin/bash

# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

SHUNIT2="${SHUNIT2:?'expecting SHUNIT2 env var pointing to a dir with https://github.com/kward/shunit2 clone'}"

DIR="$(cd "$(dirname "$0")" && pwd)"
VALUES="$DIR/jaeger-values.yaml"
CONFIG="$DIR/jaeger-config.yaml"

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
  assertTrue "tag '$tag' should be a release version" "[[ '$tag' =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]"
}

# The cleaner only deletes "<prefix>jaeger-(span|service|...)-<date>" indices,
# so its prefix must be the index_prefix Jaeger writes with plus a dash.
testCleanerPrefixMatchesJaegerIndexPrefix() {
  written=$(awk '/index_prefix:/{gsub(/"/,"",$2);print $2;exit}' "$CONFIG")
  cleaned=$(value_in_block esIndexCleaner "    index-prefix" "$VALUES")
  assertNotNull "index_prefix should be set in jaeger-config.yaml" "$written"
  assertEquals "${written}-" "$cleaned"
}

# The Jaeger indices use the daily date layout the cleaner expects by default.
testJaegerWritesDailyIndicesWithDefaultDateLayout() {
  assertEquals "$(grep -c 'date_layout:' "$CONFIG")" "$(grep -c 'date_layout: "2006-01-02"' "$CONFIG")"
  assertEquals "$(grep -c 'rollover_frequency:' "$CONFIG")" "$(grep -c 'rollover_frequency: "day"' "$CONFIG")"
}

# shellcheck disable=SC1091
source "$SHUNIT2/shunit2"
