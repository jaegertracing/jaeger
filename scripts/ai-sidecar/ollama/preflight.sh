#!/usr/bin/env bash
# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0
#
# Preflight check for the Ollama AI sidecar. There is no API key to verify —
# the prerequisite here is a reachable Ollama server that already holds the
# model, since a missing model would otherwise surface as a mid-turn failure
# long after the launcher reported success.

set -euo pipefail

OLLAMA_URL="${JAEGER_AI_OLLAMA_URL:-http://localhost:11434}"
MODEL="${JAEGER_AI_MODEL:-qwen3:8b}"

if ! command -v curl >/dev/null 2>&1; then
	echo "[ai-sidecar/ollama] curl is required to probe Ollama but was not found in PATH. Install it and retry." >&2
	exit 1
fi

if ! curl -fsS "${OLLAMA_URL}/api/tags" >/dev/null 2>&1; then
	cat >&2 <<EOF
[ai-sidecar/ollama] No Ollama server responding at ${OLLAMA_URL}.

Install Ollama (https://ollama.com/download), then:

    ollama serve
    ollama pull ${MODEL}
    make run-ai-ollama

Already running it elsewhere? Point the sidecar at it:

    export JAEGER_AI_OLLAMA_URL=http://<host>:11434
EOF
	exit 1
fi

if ! curl -fsS "${OLLAMA_URL}/api/tags" | grep -q "\"${MODEL}\""; then
	cat >&2 <<EOF
[ai-sidecar/ollama] Ollama is running at ${OLLAMA_URL}, but the model '${MODEL}' is not pulled.

    ollama pull ${MODEL}

Or select a model you already have. It must support tool calling, or the agent
cannot query Jaeger's MCP tools:

    export JAEGER_AI_MODEL=<model>
EOF
	exit 1
fi
