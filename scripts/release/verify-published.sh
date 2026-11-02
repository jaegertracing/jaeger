#!/bin/bash

# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0
#
# Read-only check of a published release: every binary archive is attached to
# the GitHub release, every image has the version tag on docker.io and quay.io,
# and the linux-amd64 binaries were built from the tag's commit.
# Missing assets or images fail; a commit mismatch is only a warning, because
# a recovery run may build a later commit on purpose.
#
# Usage: verify-published.sh vX.Y.Z

set -euo pipefail

tag=${1:?'expecting release tag, e.g. v2.23.0'}
version=${tag#v}
repo=${GITHUB_REPOSITORY:-jaegertracing/jaeger}
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
summary=${GITHUB_STEP_SUMMARY:-/dev/null}
missing=()

# Same names as scripts/build/package-deploy.sh, plus the SBOM and the
# provenance file that ci-release.yml uploads after packaging.
expected_assets() {
  echo "jaeger-${version}.sha256sum.txt"
  echo "jaeger-SBOM.spdx.json"
  echo "multiple.intoto.jsonl"
  local platform os_arch pkg
  for platform in $(make -s -C "$repo_root" echo-platforms | tr ',' ' '); do
    os_arch="${platform%%/*}-${platform##*/}"
    for pkg in jaeger jaeger-tools; do
      echo "${pkg}-${version}-${os_arch}.sha256sum.txt"
      echo "${pkg}-${version}-${os_arch}.tar.gz"
      echo "${pkg}-${version}-${os_arch}.tar.gz.asc"
      if [[ "${platform%%/*}" == "windows" ]]; then
        echo "${pkg}-${version}-${os_arch}.zip"
        echo "${pkg}-${version}-${os_arch}.zip.asc"
      fi
    done
  done
}

# build_image calls in build-upload-docker-images.sh (a "true" fourth argument
# adds a -debug image), plus the images of build-all-in-one-image.sh and
# build-hotrod-image.sh.
expected_images() {
  awk '$1 == "build_image" { print $2; if ($5 == "true") print $2 "-debug" }' \
    "$repo_root/scripts/build/build-upload-docker-images.sh"
  printf '%s\n' jaeger jaeger-debug example-hotrod
}

report_missing() {
  missing+=("$1")
  echo "::error::Missing $1"
}

echo "Checking release assets of ${tag}"
assets=$(gh release view "$tag" -R "$repo" --json assets --jq '.assets[].name')
while read -r asset; do
  grep -Fxq "$asset" <<< "$assets" || report_missing "release asset ${asset}"
done < <(expected_assets)

echo "Checking images for tag ${version}"
while read -r image; do
  for registry in docker.io quay.io; do
    ref="${registry}/jaegertracing/${image}:${version}"
    if bash "$repo_root/scripts/utils/retry.sh" docker manifest inspect "$ref" > /dev/null; then
      echo "found ${ref}"
    else
      report_missing "image ${ref}"
    fi
  done
done < <(expected_images)

echo "Checking the commit the linux-amd64 binaries were built from"
tag_sha=$(gh api "repos/${repo}/commits/${tag}" --jq .sha)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
for pkg in jaeger jaeger-tools; do
  archive="${pkg}-${version}-linux-amd64.tar.gz"
  grep -Fxq "$archive" <<< "$assets" || continue
  gh release download "$tag" -R "$repo" -p "$archive" -D "$tmp"
  tar -xzf "$tmp/$archive" -C "$tmp"
done
mismatched=()
for bin in "$tmp"/*/*; do
  [[ -f "$bin" ]] || continue
  rev=$(go version -m "$bin" | awk '$1 == "build" && $2 ~ /^vcs.revision=/ { sub(/^vcs.revision=/, "", $2); print $2 }')
  if [[ -z "$rev" ]]; then
    # example-hotrod is built from a file list, so Go records no commit.
    echo "$(basename "$bin") records no vcs.revision, skipped"
  elif [[ "$rev" == "$tag_sha" ]]; then
    echo "$(basename "$bin") built from ${tag} (${tag_sha})"
  else
    mismatched+=("$(basename "$bin") built from ${rev}")
    echo "::warning::$(basename "$bin") was built from ${rev}, not ${tag} (${tag_sha})"
  fi
done

{
  echo "### Published release check for ${tag}"
  if (( ${#missing[@]} == 0 )); then
    echo "All expected release assets and images are published."
  else
    echo "Missing:"
    printf -- '- %s\n' "${missing[@]}"
  fi
  if (( ${#mismatched[@]} > 0 )); then
    echo "Binaries not built from \`${tag}\` (\`${tag_sha}\`):"
    printf -- '- %s\n' "${mismatched[@]}"
  fi
} | tee -a "$summary"

(( ${#missing[@]} == 0 ))
