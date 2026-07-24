#!/usr/bin/env bash
# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0
#
# Single-command launcher for the Claude AI sidecar. Use via:
#
# make run-ai-claude
#
# This script:
# 1. Runs the preflight check (verifies auth).
# 2. Bootstraps the Node toolchain via `npm install`.
# 3. Starts Jaeger in the background and waits for it to be ready.
# 4. Runs the sidecar in the foreground. Ctrl-C exits both.

set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../_lib.sh
source "$HERE/../_lib.sh"

if ! command -v node >/dev/null 2>&1; then
  ai::log "node is not installed. See https://nodejs.org/ for installation."
  exit 1
fi

if ! command -v npm >/dev/null 2>&1; then
  ai::log "npm is not installed. See https://nodejs.org/ for installation."
  exit 1
fi

ai::log "bootstrapping sidecar toolchain (npm ci)…"
(cd "$HERE" && npm ci --silent)

# Preflight check requires node_modules for claude-agent-acp auth status check
ai::preflight claude-code

ai::start_jaeger
ai::wait_jaeger

ai::log "starting Claude sidecar…"
cd "$HERE"
node jaeger-ws-bridge.mjs "$@" 2>&1 | ai::tag sidecar "$AI_COLOR_SIDECAR"
