#!/usr/bin/env bash

# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0
#
# Resolve and verify Jaeger demo snapshot image tags from Docker Hub.
#
# Scheduled runs prefer MAIN_SHA when published; otherwise they deploy the
# newest published commit SHA tag (never the mutable :latest tag).
#
# Manual workflow_dispatch runs use the requested tag (default MAIN_SHA), resolve
# latest to a concrete SHA tag, and fail if the requested tag is missing.
#
# Jaeger and HotROD tags are resolved independently; each image is
# published to its own repository and may fall back to a different SHA.
#
# Exports (and appends to GITHUB_ENV when set):
#   JAEGER_DEMO_JAEGER_IMAGE_TAG
#   JAEGER_DEMO_HOTROD_IMAGE_TAG
#   JAEGER_DEMO_IMAGE_TAG

set -euo pipefail

: "${JAEGER_DEMO_JAEGER_IMAGE_REPOSITORY:=jaegertracing/jaeger-snapshot}"
: "${JAEGER_DEMO_HOTROD_IMAGE_REPOSITORY:=jaegertracing/example-hotrod-snapshot}"
: "${MAIN_SHA:=${GITHUB_SHA:-}}"
: "${GITHUB_EVENT_NAME:=workflow_dispatch}"

if [[ -z "$MAIN_SHA" ]]; then
  echo "MAIN_SHA or GITHUB_SHA must be set" >&2
  exit 1
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required but not installed" >&2
  exit 1
fi

run_curl() {
  if [[ -n "${DOCKERHUB_CURL:-}" ]]; then
    "$DOCKERHUB_CURL" "$@"
  else
    curl -sS --connect-timeout 5 --max-time 15 --retry 3 --retry-delay 2 "$@"
  fi
}

dockerhub_tag_status() {
  local repo=$1
  local tag=$2
  local status

  status=$(run_curl -o /dev/null -w "%{http_code}" \
    "https://hub.docker.com/v2/repositories/${repo}/tags/${tag}/" || true)
  status=${status:-000}
  echo "$status"
}

newest_published_sha() {
  local repo=$1
  local status
  local tag
  local tags_file

  tags_file=$(mktemp "${TMPDIR:-/tmp}/docker-tags-page.XXXXXX")

  status=$(run_curl -o "$tags_file" -w "%{http_code}" \
    "https://hub.docker.com/v2/repositories/${repo}/tags/?page_size=100&ordering=last_updated" || true)
  status=${status:-000}
  if [[ "$status" != "200" ]]; then
    rm -f "$tags_file"
    echo "Docker Hub tag list for ${repo} returned HTTP ${status}" >&2
    return 1
  fi

  # Both snapshot repos have far more than one page of SHA tags, so we rely on
  # ordering=last_updated to surface the newest tags on the first (only) page we
  # fetch, then sort that page by last_updated as a guard against out-of-order
  # results and take the newest. This does not scan beyond the first page.
  if ! tag=$(jq -r '[.results[] | select(.name | test("^[0-9a-f]{40}$"))] | sort_by(.last_updated) | last | .name // empty' "$tags_file"); then
    rm -f "$tags_file"
    return 1
  fi
  rm -f "$tags_file"

  if [[ -z "$tag" ]]; then
    echo "No published snapshot SHA tags found for ${repo}" >&2
    return 1
  fi
  echo "$tag"
}

verify_deploy_tag() {
  local repo=$1
  local tag=$2
  local label=$3
  local status

  status=$(dockerhub_tag_status "$repo" "$tag")
  if [[ "$status" == "404" ]]; then
    echo "::error::Snapshot image tag not found on Docker Hub: ${repo}:${tag} (${label})" >&2
    return 1
  fi
  if [[ "$status" != "200" ]]; then
    echo "::error::Failed to verify Docker Hub tag for ${repo}:${tag} (${label}); Docker Hub API returned HTTP ${status}." >&2
    return 1
  fi
}

resolve_snapshot_tag() {
  local repo=$1
  local preferred=$2
  local label=$3
  local status
  local resolved

  if [[ -z "$preferred" ]]; then
    echo "Preferred tag is empty for ${label}" >&2
    return 1
  fi

  if [[ "$preferred" == "latest" ]]; then
    resolved=$(newest_published_sha "$repo") || return 1
    echo "$resolved"
    return 0
  fi

  status=$(dockerhub_tag_status "$repo" "$preferred")
  if [[ "$status" == "200" ]]; then
    echo "$preferred"
    return 0
  fi
  if [[ "$status" != "404" ]]; then
    echo "::error::Failed to verify Docker Hub tag for ${repo}:${preferred} (${label}); Docker Hub API returned HTTP ${status}." >&2
    return 1
  fi

  if [[ "$GITHUB_EVENT_NAME" != "schedule" ]]; then
    echo "::error::Snapshot image tag not found on Docker Hub: ${repo}:${preferred} (${label})" >&2
    return 1
  fi

  resolved=$(newest_published_sha "$repo") || return 1
  echo "$resolved"
}

set_github_env() {
  local key=$1
  local value=$2

  if [[ -n "${GITHUB_ENV:-}" ]]; then
    printf '%s=%s\n' "$key" "$value" >> "$GITHUB_ENV"
  fi
  export "$key=$value"
}

main() {
  local jaeger_preferred
  local hotrod_preferred
  local jaeger_tag
  local hotrod_tag
  local main_status

  jaeger_preferred="${JAEGER_DEMO_JAEGER_IMAGE_TAG:-${JAEGER_DEMO_IMAGE_TAG:-$MAIN_SHA}}"
  hotrod_preferred="${JAEGER_DEMO_HOTROD_IMAGE_TAG:-${JAEGER_DEMO_IMAGE_TAG:-$MAIN_SHA}}"

  jaeger_tag=$(resolve_snapshot_tag "$JAEGER_DEMO_JAEGER_IMAGE_REPOSITORY" "$jaeger_preferred" "Jaeger")
  hotrod_tag=$(resolve_snapshot_tag "$JAEGER_DEMO_HOTROD_IMAGE_REPOSITORY" "$hotrod_preferred" "HotROD")

  verify_deploy_tag "$JAEGER_DEMO_JAEGER_IMAGE_REPOSITORY" "$jaeger_tag" "Jaeger"
  verify_deploy_tag "$JAEGER_DEMO_HOTROD_IMAGE_REPOSITORY" "$hotrod_tag" "HotROD"

  if [[ "$GITHUB_EVENT_NAME" == "schedule" && "$jaeger_tag" != "$MAIN_SHA" ]]; then
    main_status=$(dockerhub_tag_status "$JAEGER_DEMO_JAEGER_IMAGE_REPOSITORY" "$MAIN_SHA")
    if [[ "$main_status" == "404" ]]; then
      echo "Main HEAD ${MAIN_SHA} has no published Jaeger snapshot; deploying ${jaeger_tag}"
    fi
  fi
  if [[ "$GITHUB_EVENT_NAME" == "schedule" && "$hotrod_tag" != "$MAIN_SHA" ]]; then
    main_status=$(dockerhub_tag_status "$JAEGER_DEMO_HOTROD_IMAGE_REPOSITORY" "$MAIN_SHA")
    if [[ "$main_status" == "404" ]]; then
      echo "Main HEAD ${MAIN_SHA} has no published HotROD snapshot; deploying ${hotrod_tag}"
    fi
  fi

  set_github_env JAEGER_DEMO_JAEGER_IMAGE_TAG "$jaeger_tag"
  set_github_env JAEGER_DEMO_HOTROD_IMAGE_TAG "$hotrod_tag"
  set_github_env JAEGER_DEMO_IMAGE_TAG "$jaeger_tag"

  echo "Deploy: main=${MAIN_SHA}, Jaeger tag=${jaeger_tag}, HotROD tag=${hotrod_tag}"
}

main "$@"
