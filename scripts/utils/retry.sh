#!/usr/bin/env bash

# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0
#
# Generic retry wrapper for transient command failures (e.g. CI image pulls).
#
# Usage: retry.sh <cmd> [args...]
# Env vars:
#   ATTEMPTS  total attempts before giving up   (default 3)
#   BACKOFF   initial sleep between attempts    (default 15 seconds);
#             doubles after each failed attempt (exponential).

set -euf -o pipefail

: "${ATTEMPTS:=3}"
: "${BACKOFF:=15}"

if [ "$#" -eq 0 ]; then
  echo "usage: $(basename "$0") <cmd> [args...]" >&2
  exit 2
fi

i=1
backoff=$BACKOFF
while true; do
  if "$@"; then
    exit 0
  fi
  if [ "$i" -ge "$ATTEMPTS" ]; then
    echo "retry.sh: '$*' failed after $i attempts" >&2
    exit 1
  fi
  echo "retry.sh: attempt $i/$ATTEMPTS of '$*' failed; sleeping ${backoff}s" >&2
  sleep "$backoff"
  i=$((i + 1))
  backoff=$((backoff * 2))
done
