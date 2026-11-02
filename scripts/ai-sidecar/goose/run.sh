#!/usr/bin/env bash
# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

# This script starts Jaeger with an AI agent and a trickle of traces, so the
# assistant can be tried in the UI and checked from the command line. It brings
# up docker-compose.yml: Jaeger, Goose as the ACP agent, microsim sending traces
# and a reverse proxy between Goose and Jaeger's MCP endpoint that logs the
# headers of every request. It then sends one chat request that carries a caller
# identity and checks the proxy log: every MCP request Goose made must carry the
# turn token and the caller's identity headers, and none may carry a header the
# gateway was not told to forward. The stack stays up afterwards, with the UI at
# http://localhost:16686, unless --down is given, in which case the script tears
# the stack down and exits with the check's result, for use in CI.
#
# Goose lists the MCP server's tools before it calls the model, so the header
# check needs no model provider. Chatting in the UI, and the agent calling a
# tool, needs OPENROUTER_API_KEY (or another provider; see README.md).
#
# Usage: [OPENROUTER_API_KEY=...] ./run.sh [--release] [--down] [extra docker compose args]
#
# By default the script builds cmd/jaeger from this working tree for Linux and
# runs that binary inside the released image (see docker-compose.local.yml), so
# the stack exercises the checkout. --release runs the released image as is.
set -euo pipefail
cd "$(dirname "$0")"

export OPENROUTER_API_KEY="${OPENROUTER_API_KEY:-unset}"
compose=(docker compose -f docker-compose.yml)
release=0 down=0
while [ $# -gt 0 ]; do
  case "$1" in
    --release) release=1; shift ;;
    --down) down=1; shift ;;
    *) break ;;
  esac
done
if [ "$release" = 0 ]; then
  build_dir=$(mktemp -d)
  echo "--- building cmd/jaeger for linux into $build_dir"
  CGO_ENABLED=0 GOOS=linux go build -o "$build_dir/jaeger-linux" ../../../cmd/jaeger
  export JAEGER_BINARY="$build_dir/jaeger-linux"
  compose+=(-f docker-compose.local.yml)
fi
compose+=("$@")

"${compose[@]}" build --quiet
"${compose[@]}" up -d --wait --wait-timeout 120 || { "${compose[@]}" logs; exit 1; }
if [ "$down" = 1 ]; then
  trap '"${compose[@]}" down' EXIT
fi

for _ in $(seq 1 30); do
  curl -fsS -o /dev/null http://127.0.0.1:16690/api/v3/services && break
  sleep 1
done

echo "--- chat request with a caller identity"
curl -sS -N --max-time 180 http://127.0.0.1:16690/api/ai/chat \
  -H 'Content-Type: application/json' \
  -H 'X-User: alice' \
  -H 'Authorization: Bearer user-token' \
  -H 'X-Unlisted: private' \
  -d '{"threadId":"header-check","runId":"header-check-1","messages":[{"id":"m1","role":"user","content":"Call the get_services tool and list the service names it returns. Do not answer from memory."}],"tools":[],"context":[],"state":{},"forwardedProps":{}}' \
  | grep -E '"type":"(TOOL_CALL_START|RUN_ERROR)"' || true

echo "--- MCP requests seen by the proxy"
mcp=$("${compose[@]}" logs --no-log-prefix mcp-proxy | grep -E '^[A-Z]+ /api/ai/mcp/' || true)
echo "${mcp:-<none>}"

fail=0
[ -n "$mcp" ] || { echo "FAIL: Goose made no MCP request"; fail=1; }
grep -q 'turn=-' <<<"$mcp" && { echo "FAIL: an MCP request had no turn token"; fail=1; }
grep -qv 'user=alice' <<<"$mcp" && { echo "FAIL: an MCP request did not carry X-User"; fail=1; }
grep -qv 'auth=Bearer user-token' <<<"$mcp" && { echo "FAIL: an MCP request did not carry the caller's bearer token"; fail=1; }
grep -q 'unlisted=private' <<<"$mcp" && { echo "FAIL: an unlisted header leaked to the agent"; fail=1; }
[ "$fail" = 0 ] && echo "PASS"
if [ "$down" = 0 ]; then
  echo "--- the stack is up: Jaeger UI at http://localhost:16686, with the AI assistant. Stop it with:"
  echo "    JAEGER_BINARY=${JAEGER_BINARY:-} ${compose[*]} down"
fi
exit "$fail"
