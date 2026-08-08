#!/usr/bin/env bash
# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0
#
# Preflight check for the OpenRouter AI sidecar. Verifies the operator-provided
# auth prerequisite and model selection are in place before the launcher starts
# spinning up Jaeger and the toolchain.

set -euo pipefail

if [[ -z "${OPENROUTER_API_KEY:-}" ]]; then
	cat >&2 <<'EOF'
[ai-sidecar/openrouter] OPENROUTER_API_KEY is not set.

Set it before running the launcher:

    export OPENROUTER_API_KEY=…
    export OPENROUTER_MODEL=google/gemini-2.5-flash
    make run-ai-openrouter

See https://openrouter.ai/keys to obtain a key.
EOF
	exit 1
fi

if [[ -z "${OPENROUTER_MODEL:-}" ]]; then
	cat >&2 <<'EOF'
[ai-sidecar/openrouter] OPENROUTER_MODEL is not set.

This sidecar exists so the model is a variable, so there is no default:

    export OPENROUTER_MODEL=google/gemini-2.5-flash
    make run-ai-openrouter

See https://openrouter.ai/models for slugs. The model must support tool calling.
EOF
	exit 1
fi
