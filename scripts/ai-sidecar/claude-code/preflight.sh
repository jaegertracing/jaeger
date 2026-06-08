#!/usr/bin/env bash
# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0
#
# Preflight check for the Claude AI sidecar. Verifies the operator-provided
# auth prerequisite is in place before the launcher starts spinning up
# Jaeger and the toolchain.

set -euo pipefail

# Auto-detect authentication
AUTH_OK=false

if [[ -n "${ANTHROPIC_API_KEY:-}" ]]; then
    AUTH_OK=true
elif [[ -d "$(dirname "${BASH_SOURCE[0]}")/node_modules" ]]; then
    if "$(dirname "${BASH_SOURCE[0]}")/node_modules/.bin/claude-agent-acp" --cli auth status >/dev/null 2>&1; then
        AUTH_OK=true
    fi
fi

if [[ "$AUTH_OK" = false ]]; then
  cat >&2 <<'EOF'
[ai-sidecar/claude] No authentication detected.

The Claude sidecar requires either an API key or a Claude CLI session.

Option A (API Key):
    export ANTHROPIC_API_KEY=sk-...
    make run-ai-claude

Option B (Claude CLI):
    cd scripts/ai-sidecar/claude-code && npm run auth:max

See https://console.anthropic.com/ to obtain a key.
EOF
  exit 1
fi
