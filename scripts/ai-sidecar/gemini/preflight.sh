#!/usr/bin/env bash
# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0
#
# Preflight check for the Gemini AI sidecar. Verifies the operator-provided
# auth prerequisite is in place before the launcher starts spinning up
# Jaeger and the toolchain.

set -euo pipefail

if [[ -z "${GEMINI_API_KEY:-}" ]]; then
	cat >&2 <<'EOF'
[ai-sidecar/gemini] GEMINI_API_KEY is not set.

Set it before running the launcher:

    export GEMINI_API_KEY=…
    make run-ai-gemini

See https://aistudio.google.com/app/apikey to obtain a key.
EOF
	exit 1
fi
