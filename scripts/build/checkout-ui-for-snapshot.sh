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
# UI SHA is always chosen. The UI SHA is printed to stdout so callers can record
# it (e.g. as a Docker image label).
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
UI_SHA=$(curl --silent --fail --location \
    --header "Accept: application/vnd.github+json" \
    --header "X-GitHub-Api-Version: 2022-11-28" \
    ${GITHUB_TOKEN:+--header "Authorization: Bearer ${GITHUB_TOKEN}"} \
    "https://api.github.com/repos/${JAEGER_UI_REPO}/commits?sha=main&until=${JAEGER_COMMIT_TIME}&per_page=1" \
    | python3 -c "import sys,json; print(json.load(sys.stdin)[0]['sha'])")

echo "Selected jaeger-ui commit: ${UI_SHA}" >&2

# Shallow-clone at the selected SHA.
rm -rf "${WORK_DIR}"
git clone --quiet --depth=1 "https://github.com/${JAEGER_UI_REPO}.git" "${WORK_DIR}"
git -C "${WORK_DIR}" fetch --quiet --depth=1 origin "${UI_SHA}"
git -C "${WORK_DIR}" checkout --quiet "${UI_SHA}"

# Export to GITHUB_ENV if running in CI, otherwise emit for eval.
if [[ -n "${GITHUB_ENV:-}" ]]; then
    echo "JAEGER_UI_DIR=${WORK_DIR}" >> "${GITHUB_ENV}"
    echo "JAEGER_UI_SHA=${UI_SHA}" >> "${GITHUB_ENV}"
else
    echo "export JAEGER_UI_DIR='${WORK_DIR}'"
    echo "export JAEGER_UI_SHA='${UI_SHA}'"
fi
