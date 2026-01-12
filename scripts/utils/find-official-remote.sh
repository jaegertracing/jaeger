#!/usr/bin/env bash

# Copyright (c) 2025 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

# finds the git remote pointing to the official jaeger repo.
set -euo pipefail

OFFICIAL_REPO_PATTERN='(github\.com[:/])jaegertracing/jaeger(\.git)?$'

for remote in $(git remote); do
    remote_url=$(git remote get-url "$remote" 2>/dev/null || true)
    if echo "$remote_url" | grep -Eq "$OFFICIAL_REPO_PATTERN"; then
        echo "$remote"
        exit 0
    fi
done

echo "Error: could not find a remote pointing to jaegertracing/jaeger" >&2
echo "Available remotes:" >&2
git remote -v >&2
exit 1