#!/bin/bash

# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

# checkout-ui-for-snapshot.sh
#
# Used by snapshot builds (push to main) to check out the latest jaeger-ui/main
# commit whose timestamp is <= the current jaeger commit's timestamp, then export
# JAEGER_UI_DIR so that subsequent `make build-ui` uses it instead of the submodule.
#
# This keeps snapshot builds current with jaeger-ui/main without bumping the
# submodule (which would force full npm builds on all PR builds).
#
# The selected UI commit is deterministic: given the same backend SHA, the same
# UI SHA is always chosen.
#
# In CI (GITHUB_ENV is set): writes JAEGER_UI_DIR and JAEGER_UI_SHA to $GITHUB_ENV
#   so they are available to subsequent workflow steps.
# Locally (no GITHUB_ENV): eval the output to set the variables in the current shell:
#   eval "$(bash scripts/build/checkout-ui-for-snapshot.sh)"

set -euf -o pipefail

JAEGER_UI_REPO="jaegertracing/jaeger-ui"
WORK_DIR="${RUNNER_TEMP:-/tmp}/jaeger-ui-snapshot"

# Timestamp of the current jaeger commit (ISO 8601).
JAEGER_COMMIT_TIME=$(git log -1 --format=%cI HEAD)

echo "Selecting latest jaeger-ui/main commit <= ${JAEGER_COMMIT_TIME}" >&2

# GitHub API: list commits on main up to (and including) the jaeger commit time.
UI_SHA=$(gh api "repos/${JAEGER_UI_REPO}/commits?sha=main&until=${JAEGER_COMMIT_TIME}&per_page=1" --jq '.[0].sha')
if [[ -z "${UI_SHA}" || "${UI_SHA}" == "null" ]]; then
    echo "No jaeger-ui commits found before ${JAEGER_COMMIT_TIME}" >&2
    exit 1
fi

echo "Selected jaeger-ui commit: ${UI_SHA}" >&2

# Shallow-fetch exactly the selected SHA (init+fetch avoids cloning the tip of main first).
rm -rf "${WORK_DIR}"
mkdir -p "${WORK_DIR}"
git -C "${WORK_DIR}" init --quiet
git -C "${WORK_DIR}" remote add origin "https://github.com/${JAEGER_UI_REPO}.git"
git -C "${WORK_DIR}" fetch --quiet --depth=1 origin "${UI_SHA}"
git -C "${WORK_DIR}" checkout --quiet FETCH_HEAD

# Export JAEGER_UI_DIR so subsequent `make build-ui` uses this checkout instead of
# the submodule. In CI (GITHUB_ENV set) the variable persists to following steps;
# locally, eval the output: eval "$(bash scripts/build/checkout-ui-for-snapshot.sh)"
if [[ -n "${GITHUB_ENV:-}" ]]; then
    echo "JAEGER_UI_DIR=${WORK_DIR}" >> "${GITHUB_ENV}"
else
    echo "export JAEGER_UI_DIR='${WORK_DIR}'"
fi
