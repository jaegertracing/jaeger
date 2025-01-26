#!/bin/bash
# Copyright (c) 2025 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

# This script uses https://github.com/kward/shunit2 to run unit tests.
# The path to this repo must be provided via SHUNIT2 env var.

SHUNIT2="${SHUNIT2:?'expecting SHUNIT2 env var pointing to a dir with https://github.com/kward/shunit2 clone'}"


createScript="$(dirname $0)/create.sh"


unset MODE
unset DATACENTER
unset KEYSPACE
unset REPLICATION
unset REPLICATION_FACTOR
unset TRACE_TTL
unset DEPENDENCIES_TTL
unset COMPACTION_WINDOW
unset VERSION

testRequireMode() {
    err=$(bash "$createScript" 2>&1)
    assertContains "$err" "missing MODE parameter"
}

testInvalidMode() {
    err=$(MODE=invalid bash "$createScript" 2>&1)
    assertContains "$err" "invalid MODE=invalid, expecting 'prod' or 'test'"
}

testProdModeRequiresDatacenter() {
    err=$(MODE=prod bash "$createScript" 2>&1)
    assertContains "$err" "missing DATACENTER parameter for prod mode"
}

testProdModeWithDatacenter() {
    out=$(MODE=prod DATACENTER=dc1 bash "$createScript" 2>&1)
    assertContains "$out" "mode = prod"
    assertContains "$out" "datacenter = dc1"
    assertContains "$out" "replication = {'class': 'NetworkTopologyStrategy', 'dc1': '2' }"
}

testTestMode() {
    out=$(MODE=test bash "$createScript" 2>&1)
    assertContains "$out" "mode = test"
    assertContains "$out" "datacenter = test"
    assertContains "$out" "replication = {'class': 'SimpleStrategy', 'replication_factor': '1'}"
}

testCustomTTL() {
    out=$(MODE=test TRACE_TTL=86400 DEPENDENCIES_TTL=172800 bash "$createScript" 2>&1)
    assertContains "$out" "trace_ttl = 86400"
    assertContains "$out" "dependencies_ttl = 172800"
}

testInvalidKeyspace() {
    err=$(MODE=test KEYSPACE=invalid-keyspace bash "$createScript" 2>&1)
    assertContains "$err" "invalid characters in KEYSPACE"
}

testValidKeyspace() {
    out=$(MODE=test KEYSPACE=valid_keyspace_123 bash "$createScript" 2>&1)
    assertContains "$out" "keyspace = valid_keyspace_123"
}

testCustomCompactionWindow() {
    out=$(MODE=test COMPACTION_WINDOW=24h bash "$createScript" 2>&1)
    assertContains "$out" "compaction_window_size = 24"
    assertContains "$out" "compaction_window_unit = HOURS"
}

testInvalidCompactionWindow() {
    err=$(MODE=test COMPACTION_WINDOW=24x bash "$createScript" 2>&1)
    assertContains "$err" "Invalid compaction window size format"
}

testCustomVersion() {
    out=$(MODE=test VERSION=3 bash "$createScript" 2>&1)
    assertContains "$out" "v003.cql.tmpl"
}


source "${SHUNIT2}/shunit2"
