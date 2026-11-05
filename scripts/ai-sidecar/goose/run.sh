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
# http://localhost:16686, until ./run.sh --down stops it. With --ci the script
# tears the stack down after the check and exits with the check's result.
#
# Goose lists the MCP server's tools before it calls the model, so the header
# check needs no model provider. Chatting in the UI, and the agent calling a
# tool, needs OPENROUTER_API_KEY (or another provider; see README.md). When
# OPENROUTER_API_KEY is not set and SECRET_RESOLVER_CMD is, the script reads the
# key from a secret store by running "$SECRET_RESOLVER_CMD OPENROUTER_API_KEY",
# so the key never sits in the shell's environment or history. For example:
#
#   export SECRET_RESOLVER_CMD="security find-generic-password -s api-tokens -w -a"   # macOS Keychain
#   export SECRET_RESOLVER_CMD="pass show"                                            # Linux, pass
#   export SECRET_RESOLVER_CMD="secret-tool lookup service api-tokens key"            # Linux, libsecret
#
# Usage: [OPENROUTER_API_KEY=... | SECRET_RESOLVER_CMD=...] ./run.sh [--release] [--ci] [extra docker compose args]
#        ./run.sh --down
#
# By default the script builds cmd/jaeger from this working tree for Linux and
# runs that binary inside the released image (see docker-compose.local.yml), so
# the stack exercises the checkout. --release runs the released image as is.
set -euo pipefail
cd "$(dirname "$0")"

compose=(docker compose -f docker-compose.yml)
if [ "${1:-}" = "--down" ]; then
  # The override needs JAEGER_BINARY to parse; down mounts nothing.
  JAEGER_BINARY="${JAEGER_BINARY:-/dev/null}" exec "${compose[@]}" -f docker-compose.local.yml down
fi

if [ -z "${OPENROUTER_API_KEY:-}" ] && [ -n "${SECRET_RESOLVER_CMD:-}" ]; then
  OPENROUTER_API_KEY=$(eval "$SECRET_RESOLVER_CMD OPENROUTER_API_KEY")
fi

if [ -z "${OPENROUTER_API_KEY:-}" ]; then
  echo "--- OPENROUTER_API_KEY is not set: the header check still runs, but chatting in the UI fails until Goose has a model key (see README.md)"
fi
export OPENROUTER_API_KEY="${OPENROUTER_API_KEY:-unset}"
release=0 ci=0
while [ $# -gt 0 ]; do
  case "$1" in
    --release) release=1; shift ;;
    --ci) ci=1; shift ;;
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
if [ "$ci" = 1 ]; then
  trap '"${compose[@]}" down' EXIT
fi
"${compose[@]}" up -d --wait --wait-timeout 120 || { "${compose[@]}" logs; exit 1; }

for _ in $(seq 1 30); do
  curl -fsS -o /dev/null http://127.0.0.1:16690/api/v3/services && break
  sleep 1
done

# Only the proxy log lines written from here on belong to this check; the
# stack may have served chat turns from the UI before, and those carry no
# identity headers.
since=$(date -u +%Y-%m-%dT%H:%M:%SZ)
echo "--- chat request with a caller identity"
curl -sS -N --max-time 180 http://127.0.0.1:16690/api/ai/chat \
  -H 'Content-Type: application/json' \
  -H 'X-User: alice' \
  -H 'Authorization: Bearer user-token' \
  -H 'X-Unlisted: private' \
  -d '{"threadId":"header-check","runId":"header-check-1","messages":[{"id":"m1","role":"user","content":"Call the get_services tool and list the service names it returns. Do not answer from memory."}],"tools":[],"context":[],"state":{},"forwardedProps":{}}' \
  | grep -E '"type":"(TOOL_CALL_START|RUN_ERROR)"' || true

echo "--- MCP requests seen by the proxy"
mcp=$("${compose[@]}" logs --no-log-prefix --since "$since" mcp-proxy | grep -E '^[A-Z]+ /api/ai/mcp/' || true)
echo "${mcp:-<none>}"

fail=0
[ -n "$mcp" ] || { echo "FAIL: Goose made no MCP request"; fail=1; }
grep -qv -- '-> 200 ' <<<"$mcp" && { echo "FAIL: an MCP request was not answered 200"; fail=1; }
grep -q 'turn="-"' <<<"$mcp" && { echo "FAIL: an MCP request had no turn token"; fail=1; }
grep -qv 'user="alice"' <<<"$mcp" && { echo "FAIL: an MCP request did not carry X-User"; fail=1; }
grep -qv 'auth="Bearer user-token"' <<<"$mcp" && { echo "FAIL: an MCP request did not carry the caller's bearer token"; fail=1; }
grep -q 'unlisted="private"' <<<"$mcp" && { echo "FAIL: an unlisted header leaked to the agent"; fail=1; }
[ "$fail" = 0 ] && echo "PASS"
if [ "$ci" = 0 ]; then
  echo "--- the stack is up: Jaeger UI at http://localhost:16686, with the AI assistant. Stop it with: $0 --down"
fi
exit "$fail"
