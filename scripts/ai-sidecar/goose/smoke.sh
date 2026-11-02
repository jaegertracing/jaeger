#!/usr/bin/env bash
# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

# This script runs the smoke test of docker-compose.yml. It starts the stack,
# sends one chat request that carries a caller identity, and then checks the
# proxy log: every MCP request Goose made must carry the turn token and the
# caller's identity headers, and none may carry a header the gateway was not
# told to forward. It exits non-zero when any check fails.
#
# Goose connects to the announced MCP server and lists its tools before it
# calls the model, so the header checks pass or fail without a model provider.
# With a real OPENROUTER_API_KEY the agent also calls a tool, and the chat
# output shows the tool call.
#
# Usage: [OPENROUTER_API_KEY=...] ./smoke.sh [extra docker compose args, e.g. -f docker-compose.local.yml]
set -euo pipefail
cd "$(dirname "$0")"

export OPENROUTER_API_KEY="${OPENROUTER_API_KEY:-unset}"
compose=(docker compose -f docker-compose.yml "$@")

"${compose[@]}" build --quiet
"${compose[@]}" up -d --wait --wait-timeout 120 || { "${compose[@]}" logs; exit 1; }
trap '"${compose[@]}" down' EXIT

for _ in $(seq 1 30); do
  curl -fsS -o /dev/null http://127.0.0.1:16690/api/v3/services && break
  sleep 1
done

echo "--- chat"
curl -sS -N --max-time 180 http://127.0.0.1:16690/api/ai/chat \
  -H 'Content-Type: application/json' \
  -H 'X-User: alice' \
  -H 'Authorization: Bearer user-token' \
  -H 'X-Unlisted: private' \
  -d '{"threadId":"smoke","runId":"smoke-1","messages":[{"id":"m1","role":"user","content":"Call the get_services tool and list the service names it returns. Do not answer from memory."}],"tools":[],"context":[],"state":{},"forwardedProps":{}}' \
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
exit "$fail"
