#!/usr/bin/env bash

# Copyright (c) 2024 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

# Migration script from v004 to v005
# Adds OTel-native fields to the traces table and span_ref UDT.
# Sample usage: KEYSPACE=jaeger_v1 CQL_CMD='cqlsh host 9042' bash ./v004tov005.sh

set -euo pipefail

function usage {
    >&2 echo "Error: $1"
    >&2 echo ""
    >&2 echo "Usage: KEYSPACE={keyspace} CQL_CMD={cql_cmd} $0"
    >&2 echo ""
    >&2 echo "The following parameters can be set via environment:"
    >&2 echo "  KEYSPACE           - keyspace"
    >&2 echo "  CQL_CMD            - cqlsh host port -u user -p password"
    >&2 echo ""
    exit 1
}

if [[ ${KEYSPACE:-} == "" ]]; then
   usage "missing KEYSPACE parameter"
fi

keyspace=${KEYSPACE}
cqlsh_cmd=${CQL_CMD:-cqlsh}

echo "Using cql command: $cqlsh_cmd"
echo "Applying migration to keyspace: $keyspace"

# 1. Update span_ref UDT
echo "Adding trace_state and tags to span_ref UDT..."
${cqlsh_cmd} -e "ALTER TYPE $keyspace.span_ref ADD trace_state text;"
${cqlsh_cmd} -e "ALTER TYPE $keyspace.span_ref ADD tags frozen<list<frozen<$keyspace.keyvalue>>>;"

# 2. Update traces table
echo "Adding scope_name and scope_version to traces table..."
${cqlsh_cmd} -e "ALTER TABLE $keyspace.traces ADD scope_name text;"
${cqlsh_cmd} -e "ALTER TABLE $keyspace.traces ADD scope_version text;"

echo "Migration completed successfully!"
