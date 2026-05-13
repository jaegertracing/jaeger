#!/bin/bash

# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

# E2E tests for Jaeger UI served behind a reverse proxy at a URL prefix.
# Exercises three use cases defined in ADR-009:
#
#   UC-1: proxy forwards the prefix unchanged  (existing examples/reverse-proxy example)
#   UC-2: single Jaeger pod served under two different external prefixes simultaneously
#   UC-3: proxy rewrites an external prefix to a different internal prefix

# run with PAUSE=true env var to pause for user input before existing / cleaning up.

set -euf -o pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

COMPOSE_FILE="${REPO_ROOT}/examples/reverse-proxy/docker-compose.yml"
COMPOSE_PROJECT="jaeger-ui-rp"
success="false"

log() {
  echo "[$(date -u '+%H:%M:%S')] $*"
}

wait_for_url() {
  local desc="$1"
  local url="$2"
  local max=30
  local i=0
  until [[ "$(curl -s -o /dev/null -w '%{http_code}' "$url")" == "200" ]]; do
    i=$((i + 1))
    if [[ $i -ge $max ]]; then
      log "❌ Timed out waiting for $desc at $url"
      return 1
    fi
    sleep 1
  done
  log "✅ $desc is up"
}

check() {
  local desc="$1"
  local url="$2"
  local expected_code="${3:-200}"
  local actual
  actual=$(curl -s -o /dev/null -w '%{http_code}' "$url")
  if [[ "$actual" == "$expected_code" ]]; then
    log "✅ $desc ($url) -> $actual"
  else
    log "❌ $desc ($url) -> $actual (expected $expected_code)"
    return 1
  fi
}

check_body() {
  local desc="$1"
  local url="$2"
  local pattern="$3"
  local body
  body=$(curl -s "$url")
  if echo "$body" | grep -q "$pattern"; then
    log "✅ $desc body contains '$pattern'"
  else
    log "❌ $desc body missing '$pattern'"
    return 1
  fi
}

# Fetch index.html at BASE_URL and verify every static/ asset it references
# is reachable via BASE_URL (covers JS bundle, CSS bundle, and favicon).
check_static_assets() {
  local desc="$1"
  local base_url="$2"
  local html
  html=$(curl -s "${base_url}")
  local ok=true
  while IFS= read -r rel; do
    local url="${base_url}${rel}"
    local code
    code=$(curl -s -o /dev/null -w '%{http_code}' "$url")
    if [[ "$code" == "200" ]]; then
      log "✅ $desc static asset $url -> $code"
    else
      log "❌ $desc static asset $url -> $code (expected 200)"
      ok=false
    fi
  done < <(echo "$html" | grep -o 'static/[^"'"'"' >]*' | sort -u)
  [[ "$ok" == "true" ]]
}

teardown() {
  if [[ "$success" == "false" ]]; then
    echo "::group::docker compose logs"
    docker compose -p "${COMPOSE_PROJECT}" -f "${COMPOSE_FILE}" logs 2>&1 || true
    echo "::endgroup::"
  fi
  docker compose -p "${COMPOSE_PROJECT}" -f "${COMPOSE_FILE}" down --volumes --remove-orphans 2>/dev/null || true
}

trap teardown EXIT INT

# ---------------------------------------------------------------------------
# Determine the Jaeger image to use.
# In CI this is passed as JAEGER_IMAGE; locally we build a fresh one.
# ---------------------------------------------------------------------------
JAEGER_IMAGE="${JAEGER_IMAGE:-}"
if [[ -z "$JAEGER_IMAGE" ]]; then
  log "JAEGER_IMAGE not set — building jaeger-local:e2e-ui-rp from current source"
  ARCH=$(go env GOARCH)
  make -C "${REPO_ROOT}" build-ui build-jaeger \
    GOOS=linux GOARCH="${ARCH}" \
    SKIP_DEBUG_BINARIES=1
  docker build \
    --build-arg base_image=alpine:3.21 \
    --build-arg debug_image=alpine:3.21 \
    --target release \
    -t jaeger-local:e2e-ui-rp \
    -f "${REPO_ROOT}/cmd/jaeger/Dockerfile" \
    "${REPO_ROOT}/cmd/jaeger/"
  JAEGER_IMAGE="jaeger-local:e2e-ui-rp"
fi
log "Using Jaeger image: ${JAEGER_IMAGE}"

JAEGER_IMAGE="${JAEGER_IMAGE}" \
  docker compose -p "${COMPOSE_PROJECT}" -f "${COMPOSE_FILE}" up -d

# ---------------------------------------------------------------------------
# UC-1: Proxy forwards prefix unchanged
#   Jaeger: --query.base-path=/jaeger/prefix
#   httpd:  ProxyPass /jaeger/prefix → http://jaeger:16686/jaeger/prefix  (no rewrite)
#   Access: http://localhost:18080/jaeger/prefix/
# ---------------------------------------------------------------------------
log "=== UC-1: prefix-forwarding proxy ==="
wait_for_url "UC-1 proxy" "http://localhost:18080/jaeger/prefix/"

check               "UC-1 redirect"      "http://localhost:18080/jaeger/prefix" 301
check               "UC-1 index"         "http://localhost:18080/jaeger/prefix/"
check_body          "UC-1 inline script" "http://localhost:18080/jaeger/prefix/" "knownSubPaths"
check_static_assets "UC-1"               "http://localhost:18080/jaeger/prefix/"
check               "UC-1 api/services"  "http://localhost:18080/jaeger/prefix/api/services"
check               "UC-1 api/operations" "http://localhost:18080/jaeger/prefix/api/operations?service=jaeger"
check_body          "UC-1 unknown route"  "http://localhost:18080/jaeger/prefix/unknown-page" "did not load"

TRACE_ID=$(curl -s "http://localhost:18080/jaeger/prefix/api/traces?service=jaeger&limit=1" \
  | grep -o '"traceID":"[^"]*"' | head -1 | cut -d'"' -f4 || true)
if [[ -n "$TRACE_ID" ]]; then
  check      "UC-1 trace deep-link"        "http://localhost:18080/jaeger/prefix/trace/${TRACE_ID}"
  check_body "UC-1 trace deep-link script" "http://localhost:18080/jaeger/prefix/trace/${TRACE_ID}" "knownSubPaths"
else
  log "⚠ No traces found for UC-1 deep-link check (skipped)"
fi

log "✅ UC-1 PASSED"

# ---------------------------------------------------------------------------
# UC-2: Single pod, two external prefixes
#   Jaeger: no --query.base-path (serves at root /)
#   httpd:
#     ProxyPass /alt/  → http://jaeger:16686/  (strips /alt)
#     ProxyPass /      → http://jaeger:16686/  (pass-through)
# ---------------------------------------------------------------------------
log "=== UC-2: single pod, two external prefixes ==="
wait_for_url "UC-2 proxy (root)" "http://localhost:18081/"

# Root prefix
check               "UC-2 root index"         "http://localhost:18081/"
check_body          "UC-2 root script"        "http://localhost:18081/" "knownSubPaths"
check_static_assets "UC-2 root"               "http://localhost:18081/"
check               "UC-2 root api/services"  "http://localhost:18081/api/services"
check               "UC-2 root api/operations" "http://localhost:18081/api/operations?service=jaeger"
check_body          "UC-2 root unknown route" "http://localhost:18081/unknown-page" "did not load"

TRACE_ID2=$(curl -s "http://localhost:18081/api/traces?service=jaeger&limit=1" \
  | grep -o '"traceID":"[^"]*"' | head -1 | cut -d'"' -f4 || true)
if [[ -n "$TRACE_ID2" ]]; then
  check      "UC-2 root trace deep-link"        "http://localhost:18081/trace/${TRACE_ID2}"
  check_body "UC-2 root trace deep-link script" "http://localhost:18081/trace/${TRACE_ID2}" "knownSubPaths"
fi

# /alt/ prefix (same Jaeger, different external prefix)
check               "UC-2 /alt/ index"         "http://localhost:18081/alt/"
check_body          "UC-2 /alt/ script"        "http://localhost:18081/alt/" "knownSubPaths"
check_static_assets "UC-2 /alt/"               "http://localhost:18081/alt/"
check               "UC-2 /alt/ api/services"  "http://localhost:18081/alt/api/services"
check               "UC-2 /alt/ api/operations" "http://localhost:18081/alt/api/operations?service=jaeger"
check_body          "UC-2 /alt/ unknown route" "http://localhost:18081/alt/unknown-page" "did not load"
if [[ -n "$TRACE_ID2" ]]; then
  check      "UC-2 /alt/ trace deep-link"        "http://localhost:18081/alt/trace/${TRACE_ID2}"
  check_body "UC-2 /alt/ trace deep-link script" "http://localhost:18081/alt/trace/${TRACE_ID2}" "knownSubPaths"
fi

log "✅ UC-2 PASSED"

# ---------------------------------------------------------------------------
# UC-3: Proxy rewrites external prefix to a different internal prefix
#   Jaeger: --query.base-path=/internal
#   httpd:  /external/ → rewrite to /internal/
# ---------------------------------------------------------------------------
log "=== UC-3: proxy rewrites external prefix to different internal prefix ==="
wait_for_url "UC-3 proxy" "http://localhost:18082/external/"

check               "UC-3 redirect"       "http://localhost:18082/external" 301
check               "UC-3 index"          "http://localhost:18082/external/"
check_body          "UC-3 inline script"  "http://localhost:18082/external/" "knownSubPaths"
check_static_assets "UC-3"               "http://localhost:18082/external/"
check               "UC-3 api/services"   "http://localhost:18082/external/api/services"
check               "UC-3 api/operations" "http://localhost:18082/external/api/operations?service=jaeger"
check_body          "UC-3 unknown route"  "http://localhost:18082/external/unknown-page" "did not load"

TRACE_ID3=$(curl -s "http://localhost:18082/external/api/traces?service=jaeger&limit=1" \
  | grep -o '"traceID":"[^"]*"' | head -1 | cut -d'"' -f4 || true)
if [[ -n "$TRACE_ID3" ]]; then
  check      "UC-3 trace deep-link"        "http://localhost:18082/external/trace/${TRACE_ID3}"
  check_body "UC-3 trace deep-link script" "http://localhost:18082/external/trace/${TRACE_ID3}" "knownSubPaths"
else
  log "⚠ No traces found for UC-3 deep-link check (skipped)"
fi

log "✅ UC-3 PASSED"

success="true"
log "✅ All UI reverse-proxy integration tests passed"

if [[ "${PAUSE:-}" == "true" ]]; then
  log "PAUSE=true — containers still running. Press Enter to tear down."
  read -r
fi
