#!/bin/bash

function usage {
    >&2 echo "Error: $1"
    >&2 echo ""
    >&2 echo "Usage: MODE=(prod|test) [PARAM=value ...] $0 [template-file] | cqlsh"
    >&2 echo ""
    >&2 echo "The following parameters can be set via environment:"
    >&2 echo "  MODE               - prod or test. Test keyspace is usable on a single node cluster (no replication)"
    >&2 echo "  DATACENTER         - datacenter name for network topology used in prod (optional in MODE=test)"
    >&2 echo "  TRACE_TTL          - time to live for trace data, in seconds (default: 172800, 2 days)"
    >&2 echo "  DEPENDENCIES_TTL   - time to live for dependencies data, in seconds (default: 0, no TTL)"
    >&2 echo "  KEYSPACE           - keyspace (default: jaeger_v1_{datacenter})"
    >&2 echo "  REPLICATION_FACTOR - replication factor for prod (default: 2)"
    >&2 echo ""
    >&2 echo "The template-file argument must be fully qualified path to a v00#.cql.tmpl template file."
    >&2 echo "If omitted, the template file with the highest available version will be used."
    exit 1
}

trace_ttl=${TRACE_TTL:-172800}
dependencies_ttl=${DEPENDENCIES_TTL:-0}
replication_factor=${REPLICATION_FACTOR:-2}

template=$1
if [[ "$template" == "" ]]; then
    template=$(ls $(dirname $0)/*cql.tmpl | sort | tail -1)
fi

if [[ "$MODE" == "" ]]; then
    usage "missing MODE parameter"
elif [[ "$MODE" == "prod" ]]; then
    if [[ "$DATACENTER" == "" ]]; then usage "missing DATACENTER parameter for prod mode"; fi
    datacenter=$DATACENTER
    replication="{'class': 'NetworkTopologyStrategy', '$datacenter': '${replication_factor}' }"
elif [[ "$MODE" == "test" ]]; then 
    datacenter=${DATACENTER:-'test'}
    replication="{'class': 'SimpleStrategy', 'replication_factor': '1'}"
else
    usage "invalid MODE=$MODE, expecting 'prod' or 'test'"
fi

keyspace=${KEYSPACE:-"jaeger_v1_${datacenter}"}

>&2 cat <<EOF
Using template file $template with parameters:
    mode = $MODE
    datacenter = $datacenter
    keyspace = $keyspace
    replication = ${replication}
    trace_ttl = ${trace_ttl}
    dependencies_ttl = ${dependencies_ttl}
EOF

cat $template | sed \
    -e 's/--.*$//g'                                 \
    -e '/^\s*$/d'                                   \
    -e '/^$/d'                                      \
    -e "s/\${keyspace}/${keyspace}/g"               \
    -e "s/\${replication}/${replication}/g"         \
    -e "s/\${trace_ttl}/${trace_ttl}/g"             \
    -e "s/\${dependencies_ttl}/${dependencies_ttl}/g"
