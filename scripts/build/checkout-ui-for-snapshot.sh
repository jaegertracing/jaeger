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
# In CI (GITHUB_ENV is set): writes the following to $GITHUB_ENV so they are
#   available to subsequent workflow steps:
#     JAEGER_UI_DIR              — path to the snapshot checkout
#     JAEGER_UI_SKIP_RELEASE_CHECK — tells rebuild-ui.sh to skip the tag lookup
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

# Patch the version in package.json to include the short SHA, so the About panel
# shows e.g. "Jaeger UI v2.18.0-1c082b77" instead of "v2.18.0".  This only
# modifies the ephemeral WORK_DIR checkout; the submodule is untouched.
UI_SHORT_SHA="${UI_SHA:0:8}"
PKG="${WORK_DIR}/packages/jaeger-ui/package.json"
BASE_VERSION=$(python3 -c "import sys,json; print(json.load(open(sys.argv[1]))['version'])" "${PKG}")
python3 -c "
import sys, json
path = sys.argv[1]
short_sha = sys.argv[2]
d = json.load(open(path))
d['version'] = d['version'] + '-' + short_sha
open(path, 'w').write(json.dumps(d, indent=2) + '\n')
" "${PKG}" "${UI_SHORT_SHA}"
echo "Patched jaeger-ui version to ${BASE_VERSION}-${UI_SHORT_SHA}" >&2

# Export JAEGER_UI_DIR so subsequent `make build-ui` uses this checkout instead of
# the submodule. JAEGER_UI_SKIP_RELEASE_CHECK tells rebuild-ui.sh to skip the
# git fetch --unshallow + tag lookup (the snapshot commit is never a release tag).
# In CI (GITHUB_ENV set) the variables persist to following steps;
# locally, eval the output: eval "$(bash scripts/build/checkout-ui-for-snapshot.sh)"
if [[ -n "${GITHUB_ENV:-}" ]]; then
    echo "JAEGER_UI_DIR=${WORK_DIR}" >> "${GITHUB_ENV}"
    echo "JAEGER_UI_SKIP_RELEASE_CHECK=true" >> "${GITHUB_ENV}"
else
    echo "export JAEGER_UI_DIR='${WORK_DIR}'"
    echo "export JAEGER_UI_SKIP_RELEASE_CHECK=true"
fi
