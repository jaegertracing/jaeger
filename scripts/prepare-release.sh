#!/usr/bin/env bash
set -euo pipefail

# Inputs
VERSION="${1:-}"            # e.g. 1.53.0
BASE_BRANCH="${BASE_BRANCH:-main}"
UI_REPO="jaegertracing/jaeger-ui"

if [[ -z "${VERSION}" ]]; then
  echo "usage: scripts/prepare-release.sh <version>"; exit 1
fi
if ! command -v gh >/dev/null; then
  echo "gh CLI is required: https://cli.github.com/"; exit 1
fi
if ! command -v jq >/dev/null; then
  echo "jq is required: https://stedolan.github.io/jq/"; exit 1
fi

# Ensure clean state
git fetch origin --tags
git checkout "${BASE_BRANCH}"
git pull --ff-only origin "${BASE_BRANCH}"

LAST_TAG="$(git describe --tags --abbrev=0 2>/dev/null || true)"
SINCE="${LAST_TAG:-$(git rev-list --max-parents=0 HEAD)}"

BRANCH="release/v${VERSION}"
git switch -c "${BRANCH}"

# --- Optional UI bump ---
# If repo uses a UI submodule or a pinned UI version, try to bump it.
if git ls-files --stage | grep -q "160000 .* jaeger-ui$"; then
  echo "Updating UI submodule to latest tag…"
  git submodule update --init jaeger-ui
  pushd jaeger-ui >/dev/null
  git fetch --tags
NEW_UI_TAG="$(git tag --sort=-v:refname | head -n1)"
if [[ -z "${NEW_UI_TAG}" ]]; then
  echo "Error: No UI tags found in submodule"
  exit 1
fi
git checkout "${NEW_UI_TAG}"

  popd >/dev/null
  git add jaeger-ui
  git commit -m "chore(ui): bump jaeger-ui to ${NEW_UI_TAG}"
elif [[ -f cmd/query/ui-version.txt ]]; then
  echo "Updating UI version in cmd/query/ui-version.txt…"
  LATEST_UI_TAG="$(gh release list -R "${UI_REPO}" --limit 1 --json tagName -q '.[0].tagName')"
  echo "${LATEST_UI_TAG}" > cmd/query/ui-version.txt
  git add cmd/query/ui-version.txt
  git commit -m "chore(ui): bump jaeger-ui to ${LATEST_UI_TAG}"
else
  echo "UI bump skipped: no known pin found."
fi

# --- Changelog generation ---
CHANGELOG_FILE="CHANGELOG.md"
TODAY="$(date -u +%Y-%m-%d)"
HEADER="## v${VERSION} (${TODAY})"

# Collect merged PRs since last tag using a DATE, not a tag/sha
echo "Generating changelog since ${SINCE}…"
SINCE_COMMIT="$(git rev-parse "${SINCE}")"
SINCE_DATE="$(git show -s --format=%cs "${SINCE_COMMIT}")"
echo "Resolved ${SINCE} -> ${SINCE_COMMIT} (${SINCE_DATE})"

PRS_JSON="$(gh pr list -s merged -B "${BASE_BRANCH}" \
  --search "merged:>=${SINCE_DATE}" \
  --limit 1000 --json number,title,mergeCommit,labels,author,url,mergedAt)"

# Format bullets: "* Title (@author in #PR)"
ENTRIES="$(echo "${PRS_JSON}" | jq -r '.[] | "* \(.title) (@\(.author.login) in #\(.number))"' | sort || true)"

if [[ -z "${ENTRIES}" ]]; then
  ENTRIES="* No merged PRs found since ${SINCE}"
fi

# Insert into CHANGELOG.md
if [[ -f "${CHANGELOG_FILE}" ]]; then
  TMP="$(mktemp)"
  {
    echo "${HEADER}"
    echo
    echo "${ENTRIES}"
    echo
    cat "${CHANGELOG_FILE}"
  } > "${TMP}"
  mv "${TMP}" "${CHANGELOG_FILE}"
else
  {
    echo "# Changelog"
    echo
    echo "${HEADER}"
    echo
    echo "${ENTRIES}"
    echo
  } > "${CHANGELOG_FILE}"
fi

git add "${CHANGELOG_FILE}"
git commit -m "chore(release): add changelog for v${VERSION}"

# --- Open PR ---
TITLE="release: v${VERSION}"
BODY="$(cat <<EOF
Automates release prep:

- Changelog generated from merged PRs since ${SINCE}.
- UI upgrade attempted automatically where possible.
- See job logs for suggested tag commands.

Post-merge manual steps:
1) Create GitHub Release for v${VERSION}
2) Kick artifacts build workflow
EOF
)"

gh pr create \
  --title "${TITLE}" \
  --body "${BODY}" \
  --base "${BASE_BRANCH}" \
  --head "${BRANCH}"

# --- Print tag commands for human confirmation ---
echo
echo "== Tag commands to run after merging =="
echo "git checkout ${BASE_BRANCH}"
echo "git pull --ff-only origin ${BASE_BRANCH}"
echo "git tag -a v${VERSION} -m \"Jaeger v${VERSION}\""
echo "git push origin v${VERSION}"
