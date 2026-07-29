#!/usr/bin/env bash
# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0
#
# Single-command launcher for the Ollama AI sidecar. Use via:
#
#     make run-ai-ollama
#
# No API key: the model runs on this machine.
#
# This script:
#   1. Runs the preflight check (Ollama reachable, model pulled).
#   2. Bootstraps the Python toolchain via `uv sync`.
#   3. Starts Jaeger in the background and waits for it to be ready.
#   4. Runs the sidecar in the foreground. Ctrl-C exits both.

set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../_lib.sh
source "$HERE/../_lib.sh"

ai::preflight ollama

if ! command -v uv >/dev/null 2>&1; then
	ai::log "uv is not installed. See https://docs.astral.sh/uv/ for installation."
	exit 1
fi

ai::log "bootstrapping sidecar toolchain (uv sync)…"
(cd "$HERE" && uv sync --quiet)

ai::start_jaeger
ai::wait_jaeger

ai::log "starting Ollama sidecar (model: ${JAEGER_AI_MODEL:-qwen3:8b})…"
cd "$HERE"
uv run python main.py 2>&1 | ai::tag sidecar "$AI_COLOR_SIDECAR"
