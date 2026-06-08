#!/usr/bin/env bash
# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0
#
# Shared helpers for per-sidecar launchers under scripts/ai-sidecar/<name>/run.sh.
# Sourced by run.sh; not directly executable.

set -euo pipefail

# Repository root, resolved once and exported for sidecar scripts.
AI_REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

# Path to the Jaeger config file the launcher uses. The example config is the
# only one in-tree that opts into the `ai:` block; the embedded all-in-one
# config does not, by design (see RFC 0003 §3.2).
AI_JAEGER_CONFIG="$AI_REPO_ROOT/cmd/jaeger/config.yaml"

# Jaeger query HTTP endpoint we poll to declare readiness.
AI_JAEGER_READY_URL="http://127.0.0.1:16686/api/services"

# How long to wait for Jaeger to come up before giving up. Cold `go run` can
# take 30s+ on a fresh build cache; 90s leaves room without hanging forever.
AI_JAEGER_READY_TIMEOUT_SEC=${AI_JAEGER_READY_TIMEOUT_SEC:-90}

ai::log() {
	echo "[ai-sidecar] $*"
}

# Run the preflight script for the named sidecar. Each sidecar contributes a
# `preflight.sh` next to its source that exits non-zero with a one-line error
# when its auth prerequisite isn't met.
ai::preflight() {
	local sidecar="$1"
	local script="$AI_REPO_ROOT/scripts/ai-sidecar/${sidecar}/preflight.sh"
	if [[ ! -x "$script" ]]; then
		ai::log "missing preflight script: $script"
		exit 1
	fi
	"$script"
}

# Launch Jaeger in the background via `go run`. Sets AI_JAEGER_PID so the EXIT
# trap can stop it on shutdown. The first call also installs the trap.
#
# `go run` is a wrapper: it builds a temporary binary and execs the compiled
# Jaeger process as a child. Signalling only the wrapper PID leaves the
# compiled Jaeger orphaned (reparented to init). We enable bash monitor
# mode (`set -m`) for the duration of the launch so the backgrounded job
# becomes its own process group leader; cleanup_on_exit then kills the whole
# group with `kill -- -PGID`, taking the compiled binary with it.
ai::start_jaeger() {
	ai::log "starting Jaeger (config: $AI_JAEGER_CONFIG)…"
	set -m
	(cd "$AI_REPO_ROOT" && exec go run ./cmd/jaeger --config "$AI_JAEGER_CONFIG") &
	AI_JAEGER_PID=$!
	set +m
	trap ai::cleanup_on_exit EXIT
}

# Poll the Jaeger query HTTP port until it answers or the timeout elapses.
ai::wait_jaeger() {
	local deadline=$((SECONDS + AI_JAEGER_READY_TIMEOUT_SEC))
	ai::log "waiting for Jaeger to become ready at $AI_JAEGER_READY_URL (up to ${AI_JAEGER_READY_TIMEOUT_SEC}s)…"
	while ! curl -fsS "$AI_JAEGER_READY_URL" >/dev/null 2>&1; do
		if (( SECONDS > deadline )); then
			ai::log "Jaeger did not become ready within ${AI_JAEGER_READY_TIMEOUT_SEC}s"
			return 1
		fi
		# Bail early if the Jaeger process has already died — there's nothing
		# left to wait for and the operator wants to see the failure now.
		if ! kill -0 "$AI_JAEGER_PID" 2>/dev/null; then
			ai::log "Jaeger process exited before becoming ready"
			return 1
		fi
		sleep 0.5
	done
	ai::log "Jaeger is ready"
}

ai::cleanup_on_exit() {
	if [[ -n "${AI_JAEGER_PID:-}" ]] && kill -0 "$AI_JAEGER_PID" 2>/dev/null; then
		ai::log "stopping Jaeger (pgid $AI_JAEGER_PID)…"
		# Negative PID signals the entire process group — see start_jaeger.
		# This catches both the `go run` wrapper and the compiled binary
		# it execs. Errors are ignored: by the time wait returns, members
		# of the group may already have exited.
		kill -TERM -- -"$AI_JAEGER_PID" 2>/dev/null || true
		wait "$AI_JAEGER_PID" 2>/dev/null || true
	fi
}
