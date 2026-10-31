#!/bin/bash

# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0
#
# shunit2 tests for prepare.sh, rotate-managers.py and start.sh.
# They run against throwaway local git repos, with gh, make and wget stubbed out.

SHUNIT2="${SHUNIT2:?'expecting SHUNIT2 env var pointing to a dir with https://github.com/kward/shunit2 clone'}"

RELEASE_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$RELEASE_DIR/../.." && pwd)"

setUp() {
  T=$(mktemp -d)
  mkdir -p "$T/bin"
  export GH_LOG="$T/gh.log"
  ORIG_PATH="$PATH"
  export PATH="$T/bin:$PATH"
  export GIT_AUTHOR_NAME=test GIT_AUTHOR_EMAIL=test@example.com
  export GIT_COMMITTER_NAME=test GIT_COMMITTER_EMAIL=test@example.com
  export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1
}

tearDown() {
  export PATH="$ORIG_PATH"
  rm -rf "$T"
}

stub() {
  printf '#!/bin/bash\n%s\n' "$2" > "$T/bin/$1"
  chmod +x "$T/bin/$1"
}

write_release_md() {
  cat > "$1/RELEASE.md" <<'EOF'
# Release

| Version | Release Manager | Tentative release date    |
|---------|-----------------|---------------------------|
| 2.23.0  | @albertteoh     | 4 November 2026           |
| 2.24.0  | @pavolloffay    | 2 December 2026           |
EOF
}

# Builds an official remote (path matches github.com/jaegertracing/jaeger), a fork
# remote, a jaeger-ui remote with tags, and a working clone whose local main has a
# commit that is not on the official main.
setup_prepare_repos() {
  local up="$T/github.com/jaegertracing/jaeger"
  git init -q --bare -b main "$up"
  git init -q --bare "$T/fork.git"

  git init -q -b main "$T/seed"
  mkdir -p "$T/seed/scripts/utils" "$T/seed/scripts/release"
  cp "$REPO_ROOT/scripts/utils/find-official-remote.sh" "$T/seed/scripts/utils/"
  cp "$RELEASE_DIR/prepare.sh" "$RELEASE_DIR/update-changelog.py" "$RELEASE_DIR/rotate-managers.py" "$T/seed/scripts/release/"
  printf 'Changes\n<details>\ntemplate\n</details>\n' > "$T/seed/CHANGELOG.md"
  write_release_md "$T/seed"
  git -C "$T/seed" add -A && git -C "$T/seed" commit -q -m base
  git -C "$T/seed" push -q "$up" main

  git clone -q "$up" "$T/work"
  git -C "$T/work" remote rename origin upstream
  git -C "$T/work" remote add origin "$T/fork.git"
  echo local > "$T/work/local-only.txt"
  git -C "$T/work" add local-only.txt && git -C "$T/work" commit -q -m local-only

  echo newer > "$T/seed/newer.txt"
  git -C "$T/seed" add newer.txt && git -C "$T/seed" commit -q -m newer-upstream
  git -C "$T/seed" push -q "$up" main

  git init -q -b main "$T/ui-up"
  printf '## v2.23.0\n\nui change\n' > "$T/ui-up/CHANGELOG.md"
  git -C "$T/ui-up" add -A && git -C "$T/ui-up" commit -q -m ui
  git -C "$T/ui-up" tag v2.23.0 && git -C "$T/ui-up" tag v2.23.1
  git clone -q "$T/ui-up" "$T/work/jaeger-ui"

  # shellcheck disable=SC2016
  stub make 'case "$*" in "-s changelog") echo "* a change";; esac'
  # shellcheck disable=SC2016
  stub gh 'printf "%s\n" "$@" >> "$GH_LOG"; echo https://github.com/jaegertracing/jaeger/pull/1'
}

run_prepare() {
  (cd "$T/work" && bash scripts/release/prepare.sh "$@" 2>&1)
}

pushed_branch() {
  git -C "$T/fork.git" for-each-ref --format='%(refname:short)' 'refs/heads/prepare-release-*'
}

testPrepareBranchesFromOfficialMain() {
  setup_prepare_repos
  out=$(run_prepare 2.23.0 42)
  assertEquals "prepare.sh failed: $out" 0 $?
  branch=$(pushed_branch)
  assertNotNull "no branch pushed to fork" "$branch"
  upstream_main=$(git -C "$T/github.com/jaegertracing/jaeger" rev-parse main)
  parent=$(git -C "$T/fork.git" rev-parse "$branch^")
  assertEquals "release commit is not on top of official main" "$upstream_main" "$parent"
  assertFalse "local-only commit leaked into release branch" \
    "git -C '$T/fork.git' cat-file -e '$branch:local-only.txt'"
}

testPrepareRefusesDirtyTree() {
  setup_prepare_repos
  echo dirty >> "$T/work/CHANGELOG.md"
  out=$(run_prepare 2.23.0)
  assertEquals 1 $?
  assertContains "$out" "uncommitted changes"
  assertNull "branch was pushed" "$(pushed_branch)"
}

testPrepareStopsWhenBranchCheckoutFails() {
  setup_prepare_repos
  echo untracked > "$T/work/newer.txt"
  local_main=$(git -C "$T/work" rev-parse main)
  out=$(run_prepare 2.23.0)
  assertEquals 1 $?
  assertEquals "local main was modified" "$local_main" "$(git -C "$T/work" rev-parse main)"
  assertNull "branch was pushed" "$(pushed_branch)"
  assertFalse "gh was called" "[ -s '$GH_LOG' ]"
}

testPrepareFailsWhenUITagMissing() {
  setup_prepare_repos
  out=$(run_prepare 2.24.0 </dev/null)
  assertEquals 1 $?
  assertContains "$out" "jaeger-ui tag v2.24.0 not found"
  assertNull "branch was pushed" "$(pushed_branch)"
  assertFalse "gh was called" "[ -s '$GH_LOG' ]"
}

testPrepareCreatesPRInOfficialRepo() {
  setup_prepare_repos
  run_prepare 2.23.0 42 >/dev/null
  assertContains "$(tr '\n' ' ' < "$GH_LOG")" "pr create -R jaegertracing/jaeger"
  assertContains "$(cat "$GH_LOG")" "Part of #42"
}

testPrepareRotatesOnMatchingMinorRelease() {
  setup_prepare_repos
  run_prepare 2.23.0 >/dev/null
  release_md=$(git -C "$T/fork.git" show "$(pushed_branch):RELEASE.md")
  assertEquals "| 2.24.0  | @pavolloffay    | 2 December 2026           |" "$(sed -n 5p <<< "$release_md")"
  assertContains "$(sed -n 6p <<< "$release_md")" "@albertteoh"
  assertContains "$(cat "$GH_LOG")" "Rotated release managers table"
}

testPrepareSkipsRotationOnPatchRelease() {
  setup_prepare_repos
  out=$(run_prepare 2.23.1)
  assertEquals "prepare.sh failed: $out" 0 $?
  assertContains "$out" "2.23.1 is not a minor release"
  assertEquals "$(cat "$T/seed/RELEASE.md")" "$(git -C "$T/fork.git" show "$(pushed_branch):RELEASE.md")"
  assertNotContains "$(cat "$GH_LOG")" "Rotated release managers table"
}

testRotateSkipsMinorReleaseNotInFirstRow() {
  write_release_md "$T"
  out=$(cd "$T" && python3 "$RELEASE_DIR/rotate-managers.py" v2.24.0)
  assertEquals 0 $?
  assertContains "$out" "does not match the table's first row (2.23.0)"
  mkdir "$T/expected" && write_release_md "$T/expected"
  assertEquals "$(cat "$T/expected/RELEASE.md")" "$(cat "$T/RELEASE.md")"
}

testRotateRequiresVersion() {
  out=$(cd "$T" && python3 "$RELEASE_DIR/rotate-managers.py" 2>&1)
  assertEquals 1 $?
  assertContains "$out" "Usage:"
}

setup_start_stubs() {
  stub make 'echo v2.22.0'
  stub wget 'echo "wget called"; exit 1'
  # shellcheck disable=SC2016
  stub gh 'printf "%s\n" "$@" >> "$GH_LOG"; [ "$1 $2" = "issue list" ] && echo "$EXISTING_ISSUE"; exit 0'
}

testStartStopsOnExistingIssue() {
  setup_start_stubs
  out=$(cd "$T" && EXISTING_ISSUE=https://github.com/jaegertracing/jaeger/issues/7757 \
    bash "$RELEASE_DIR/start.sh" <<< "2.23.0" 2>&1)
  assertEquals 1 $?
  assertContains "$out" 'open issue "Prepare Jaeger Release v2.23.0" already exists: https://github.com/jaegertracing/jaeger/issues/7757'
  assertNotContains "$out" "wget called"
  assertNotContains "$(cat "$GH_LOG")" "create"
  assertContains "$(tr '\n' ' ' < "$GH_LOG")" "--state open"
}

testStartContinuesWithoutExistingIssue() {
  setup_start_stubs
  out=$(cd "$T" && EXISTING_ISSUE='' bash "$RELEASE_DIR/start.sh" -d <<< "2.23.0" 2>&1)
  assertContains "$out" "wget called"
  assertNotContains "$out" "already exists"
}

# shellcheck disable=SC1091
source "${SHUNIT2}/shunit2"
