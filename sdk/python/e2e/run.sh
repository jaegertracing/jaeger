#!/usr/bin/env bash
# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0
#
# Runs the SDK's end-to-end test: OpenSearch in docker, Jaeger from source against
# the repository's existing OpenSearch config, and the test driving both through the
# SDK. Everything it starts, it stops.

set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SDK_DIR="$(dirname "$HERE")"
REPO_ROOT="$(cd "$SDK_DIR/../.." && pwd)"

JAEGER_CONFIG="$REPO_ROOT/cmd/jaeger/config-opensearch.yaml"
JAEGER_BIN="$HERE/.build/jaeger"
JAEGER_PID=""

# The structured filter is behind an Alpha feature gate, off by default, and a query
# carrying one is refused until it is on.
FEATURE_GATES="jaeger.query.structuredFilters"

# Everything this test needs a port for. It refuses to start when one is taken rather
# than testing whatever else is listening: a stale Jaeger on 16685 answers queries
# happily, and one built before the filter existed drops it as an unknown proto field,
# so every predicate silently stops filtering and the failures look like Jaeger's.
REQUIRED_PORTS=(9200 4318 16685)

# An explicit project name, because `include` leaves Compose to derive one from the
# included file's directory, and `up` and `down` then disagree about what to tear down.
COMPOSE=(docker compose --project-name jaeger-sdk-e2e --file "$HERE/docker-compose.yml")

log() { echo "🧪 [sdk-e2e] $*"; }

# Clear anything a previous run left behind before checking the ports, so our own
# leftovers do not read as somebody else's process.
"${COMPOSE[@]}" down --volumes --remove-orphans >/dev/null 2>&1 || true

for port in "${REQUIRED_PORTS[@]}"; do
	if lsof -nP -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1; then
		log "port $port is already in use, and this test needs it:"
		lsof -nP -iTCP:"$port" -sTCP:LISTEN | sed 1d | awk '{print "         " $1, "pid", $2}' >&2
		log "stop that process and run again."
		exit 1
	fi
done

cleanup() {
	local status=$?
	if [[ -n "$JAEGER_PID" ]]; then
		log "stopping Jaeger"
		kill "$JAEGER_PID" 2>/dev/null || true
		wait "$JAEGER_PID" 2>/dev/null || true
		# Belt and braces: if it is somehow still holding the query port, say so rather
		# than leaving the next run to discover it.
		if lsof -nP -iTCP:16685 -sTCP:LISTEN >/dev/null 2>&1; then
			log "warning: something is still listening on 16685"
		fi
	fi
	log "stopping OpenSearch"
	"${COMPOSE[@]}" down --volumes --remove-orphans >/dev/null 2>&1 || true
	exit $status
}
trap cleanup EXIT

log "starting OpenSearch"
"${COMPOSE[@]}" up --detach --wait

# The compose --wait above only reports the container started, and OpenSearch answers
# on 9200 a little later; Jaeger exits if its storage is not there yet.
log "waiting for OpenSearch to answer on 9200"
for _ in $(seq 1 60); do
	if curl --silent --fail http://localhost:9200 >/dev/null 2>&1; then break; fi
	sleep 2
done
curl --silent --fail http://localhost:9200 >/dev/null

# Built rather than `go run`, which execs the binary as a grandchild and hands back a
# wrapper PID; killing that leaves Jaeger holding the ports for the next run to trip over.
log "building Jaeger"
(cd "$REPO_ROOT" && go build -o "$JAEGER_BIN" ./cmd/jaeger)

log "starting Jaeger ($JAEGER_CONFIG, gates: $FEATURE_GATES)"
(cd "$REPO_ROOT" && exec "$JAEGER_BIN" --config "$JAEGER_CONFIG" --feature-gates "$FEATURE_GATES") &
JAEGER_PID=$!

# The test waits for the query service itself, and for OpenSearch to index what it
# sent, so there is nothing to poll here.
log "running the end-to-end test"
cd "$SDK_DIR"
JAEGER_QUERY_ADDR="${JAEGER_QUERY_ADDR:-localhost:16685}" \
	JAEGER_OTLP_HTTP="${JAEGER_OTLP_HTTP:-http://localhost:4318}" \
	uv run --extra grpc pytest -q -m e2e "$@"
