# Jaeger Overall Release Process

## ⭐ Start Here: Create Tracking Issue for Release ⭐

From an up-to-date `main` (`git switch main && git pull --ff-only`), run the following command to create a tracking issue with the full checklist:

```bash
bash scripts/release/start.sh
```

This script will:
- Automatically determine the next version number (e.g., v2.14.0)
- Create a GitHub issue with the complete release checklist
- Include the exact automation commands with the correct version numbers

Example output:
```
Current version: v2.13.0
New version: v2.14.0
...
Creating issue in jaegertracing/jaeger
https://github.com/jaegertracing/jaeger/issues/7757
```

## 📝 Release Steps

Follow the checklist in the created tracking issue. The high level steps are:

1. Run the [preflight checks](#preflight) (below).
2. Perform UI release according to <https://github.com/jaegertracing/jaeger-ui/blob/main/RELEASE.md>
3. Perform Backend release (see below)
4. [Publish documentation](https://github.com/jaegertracing/documentation/blob/main/RELEASE.md) for the new version on `jaegertracing.io`.
5. [Wrap up](#wrap-up): update the feature gate tracker and close the tracking issue.

# ⚙️ Jaeger Backend Release Process

## Preflight

Do these in your clone of `jaegertracing/jaeger` before the UI release.

<!-- BEGIN_PREFLIGHT -->

Throughout this issue, tick each item when it is done, or when it does not apply to this release. An item that starts with "If" or "Only if", and everything nested under it, applies only when that condition happens.

1. Update your clone: `git switch main && git pull --ff-only && git fetch --tags`. Your `main` must track `jaegertracing/jaeger`, not a fork; `git rev-parse --abbrev-ref main@{upstream}` names the remote it tracks.
2. Land the feature gate transitions due in vX.Y.Z before preparing the release.
  * Review the [feature gate lifecycle tracker](https://github.com/jaegertracing/jaeger/issues/9057), then merge PRs for every gate stage advancement and legacy-ID removal due in vX.Y.Z.
  * Confirm no gate on main still carries `WithRegisterToVersion("vX.Y.Z")`. This must print nothing: `git grep -n 'WithRegisterToVersion("vX.Y.Z")' -- '*.go' ':!*_test.go'`.
  * The tracker is maintained by hand, so it can miss gates. Compare the gate IDs registered in the code with the tracker's rows. This prints nothing when they match. A `<` line is a gate in the code with no tracker row; a `>` line is a tracker row whose gate is gone. Raise any difference with the maintainers before you prepare the release.
    ```bash
    diff <(git grep -h -A1 'GlobalRegistry().MustRegister(' -- '*.go' ':!*_test.go' | grep -o '"[^"]*"' | tr -d '"' | sort -u) \
      <(gh issue view 9057 -R jaegertracing/jaeger --json body --jq .body | grep -o '^| `[^`]*`' | tr -d '|` ' | sort -u)
    ```
3. Run `make changelog`. It takes about 2 minutes. It fails when a PR in the release has no `changelog:` label or more than one; fix the labels on those PRs and rerun it.
4. Check that CI on `main` is green. This must print `success`; nothing or `null` means CI has not finished. Keep `--paginate`: without it the API returns only the first 30 of about 100 checks, so the command can print nothing even when CI is green.
    ```bash
    gh api --paginate repos/jaegertracing/jaeger/commits/main/check-runs --jq '.check_runs[] | select(.name == "All CI Checks Passed") | .conclusion'
    ```
5. Check that [GitHub status](https://www.githubstatus.com/) shows no incident for Git Operations or Pull Requests. If it does, see [GitHub incident](https://github.com/jaegertracing/jaeger/blob/main/RELEASE.md#github-incident).

<!-- END_PREFLIGHT -->

## Release

<!-- BEGIN_CHECKLIST -->

1. Create a PR "Prepare release vX.Y.Z" against main or maintenance branch ([example](https://github.com/jaegertracing/jaeger/pull/6826)).
  * `main` may have moved since the preflight. Run `make changelog` and the CI check from [Preflight](https://github.com/jaegertracing/jaeger/blob/main/RELEASE.md#preflight) again.
  * **Automated** (if it fails, see [Troubleshooting](https://github.com/jaegertracing/jaeger/blob/main/RELEASE.md#troubleshooting)):
    ```bash
    make prepare-release VERSION=X.Y.Z ISSUE=__RELEASE_ISSUE__
    ```
    * Updates CHANGELOG.md (generates content via `make changelog`)
    * Upgrades jaeger-ui submodule to the corresponding version
    * Rotates release managers table (only for the X.Y.0 release in its first row)
    * Creates PR with label `changelog:skip`
  * Only if the automated command can't be used: see [Manual release pull request](https://github.com/jaegertracing/jaeger/blob/main/RELEASE.md#manual-release-pull-request).
  * Once another maintainer approves the PR and the required checks pass, merge it if it is still open. The approver may have merged it already; this prints the state and, once merged, the merge commit SHA. Note the SHA for the next steps.
    ```bash
    gh pr view <PR number> -R jaegertracing/jaeger --json state,mergeCommit --jq '[.state, .mergeCommit.oid] | @tsv'
    ```
  * Check that the merge commit's subject starts with `Prepare release v`. If it does not, the draft release workflow in the next step does not run.
    ```bash
    gh api repos/jaegertracing/jaeger/commits/<merge commit SHA> --jq '.commit.message | split("\n")[0]'
    ```
2. After the PR is merged, the [Draft release workflow](https://github.com/jaegertracing/jaeger/actions/workflows/ci-release-draft.yml) creates a draft release from the merge commit.
  * Open the draft on [GitHub Releases](https://github.com/jaegertracing/jaeger/releases), review the notes, and publish it.
  * Only if no draft appears (the workflow failed, or the PR targeted a maintenance branch): run `./scripts/release/draft.py --target <merge commit SHA>` instead, or see [Manual release](https://github.com/jaegertracing/jaeger/blob/main/RELEASE.md#manual-release).
3. Once the release is created, the [Publish Release workflow](https://github.com/jaegertracing/jaeger/actions/workflows/ci-release.yml) will run to build artifacts.
  * Wait for the workflow to finish. For monitoring and troubleshooting, open the logs of the workflow run from above URL.
  * Check that the release is complete:
    * The tag points at the merge commit SHA you noted when the "Prepare release" PR merged. This prints the commit the tag points at: `gh api repos/jaegertracing/jaeger/commits/vX.Y.Z --jq .sha`.
    * The workflow's `verify-release` job passed. It fails if a binary archive is missing from the release or an image lacks the version tag on docker.io or quay.io, and it warns (without failing) if the linux-amd64 binaries were built from a commit other than the tag. Its run summary lists what is missing. To check again after a fix, run `bash scripts/release/verify-published.sh vX.Y.Z` from the root of your clone (needs `gh`, `docker`, `go`, and `make`).
  * Only if the workflow fails:
    * Only `generate-provenance` failed (`publish-release` passed and uploaded the binaries): re-run the provenance jobs, starting from `generate-provenance / detect-env`. "Re-run failed jobs" does not help here: `detect-env` reports success even when it is the job that went wrong, so it is not re-run.
      ```bash
      JOB_ID=$(gh run view <run ID> -R jaegertracing/jaeger --json jobs --jq '.jobs[] | select(.name == "generate-provenance / detect-env") | .databaseId')
      gh run rerun <run ID> -R jaegertracing/jaeger --job "$JOB_ID"
      ```
    * Failed before uploading anything (the release has no assets) for a temporary reason such as a network error: `gh run rerun <run ID> -R jaegertracing/jaeger --failed`. It rebuilds the same tag.
    * The fix is only in `.github/workflows/ci-release.yml` on `main`: run the workflow from `main` and build the tag. The workflow file comes from `main`; everything else, build scripts included, comes from the tag. `-f ref` also takes a full commit SHA.
      ```bash
      gh workflow run ci-release.yml -R jaegertracing/jaeger --ref main -f ref=vX.Y.Z -f dry_run=false -f overwrite=true
      ```
    * The fix is in the build scripts or other code on `main`: run the workflow from `main` without `-f ref`, so it builds `main` and includes the fix: `gh workflow run ci-release.yml -R jaegertracing/jaeger --ref main -f dry_run=false -f overwrite=true`. The build then also includes the other commits after the tag; the run summary lists them. If the fix is the only commit after the tag and the release is not announced, moving the tag (next item) gives a build that matches the tag.
    * Code fix, release not announced, and `git log --oneline vX.Y.Z..<upstream-remote>/main` lists only the fix: move the tag (`git tag -f vX.Y.Z <fix-sha> && git push -f <upstream-remote> vX.Y.Z`) and run the workflow with `-f ref=vX.Y.Z` (safe for Go: the module path has no `/v2`).
    * Anything else: cut a patch release.

<!-- END_CHECKLIST -->

## Wrap-up

Do these after the documentation is published.

<!-- BEGIN_WRAPUP -->

1. Update the [feature gate lifecycle tracker](https://github.com/jaegertracing/jaeger/issues/9057): set "Latest released version" to vX.Y.Z and "Next release" to the version after it, drop the rows for gate IDs this release removed, and reword notes that say a change ships in vX.Y.Z to say it shipped.
2. Close the tracking issue with a comment that links the UI release, the backend release, and the documentation PR.

<!-- END_WRAPUP -->

## Troubleshooting

### GitHub incident

Pushes rejected with `Internal Server Error`, or SSH logins to GitHub that fail some of the time, can mean GitHub itself is having an incident. Check [GitHub status](https://www.githubstatus.com/). This prints the open incidents, or nothing:

```bash
curl -s https://www.githubstatus.com/api/v2/incidents/unresolved.json | jq -r '.incidents[] | "\(.name): \(.status)"'
```

While an incident affects Git Operations or Pull Requests, stop pushing, merging, and publishing. Resume once it is resolved and a test `git fetch` from GitHub works.

### `make prepare-release` failed at the push

The script commits on a local branch named `prepare-release-vX.Y.Z-<unix time>` before it pushes. If the push fails with a server error, also check for a [GitHub incident](#github-incident). First find out whether the branch reached GitHub:

```bash
git ls-remote origin 'refs/heads/prepare-release-vX.Y.Z-*'
```

* It prints nothing: delete the local branch and reset the `jaeger-ui` submodule, which the failed run left at the new UI version:
  ```bash
  git switch main
  git branch --list 'prepare-release-vX.Y.Z-*'   # delete each one with: git branch -D <branch>
  git submodule update
  ```
  Then run `make prepare-release` again.
* It prints a branch, and there is no "Prepare release vX.Y.Z" PR: do not rerun the command. Open the PR from that branch with the same title and label the script uses; the title must start with `Prepare release v` for the draft release workflow to run.
  ```bash
  git switch prepare-release-vX.Y.Z-<unix time>
  gh pr create -R jaegertracing/jaeger --base main --title "Prepare release vX.Y.Z" \
    --label changelog:skip --body "Part of #<tracking issue number>"
  ```
* The PR exists: go on to its approval.

### SSH to GitHub fails but HTTPS works

`make prepare-release` fetches and pushes with your clone's remote URLs. If those are SSH (`git@github.com:…`) and SSH fails, run the command over HTTPS without editing your git config:

```bash
GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=url.https://github.com/.insteadOf GIT_CONFIG_VALUE_0=git@github.com: \
  make prepare-release VERSION=X.Y.Z ISSUE=<tracking issue number>
```

This needs git to authenticate to GitHub over HTTPS, for example after `gh auth setup-git`.

### A required check failed on an artifact upload

Sometimes a CI job passes its tests but fails to upload its artifact (`Failed to FinalizeArtifact: Unable to make request: ECONNRESET`). `Coverage Gate` then fails with `ci-summary artifact missing or unparseable`, and the failed job also fails `All CI Checks Passed`. That is a network error, not a code problem. Re-run the failed jobs once:

```bash
gh run rerun <run ID> -R jaegertracing/jaeger --failed
```

If the same jobs fail again, look at the logs before going on.

## Manual release pull request

Create a PR "Prepare release vX.Y.Z" against main or maintenance branch ([example](https://github.com/jaegertracing/jaeger/pull/6826)).

  * Update CHANGELOG.md to include:
    * A new section with the header `vX.Y.Z (YYYY-MM-DD)` (copy the template at the top)
    * A curated list of notable changes and links to PRs. Do not simply dump git log, select the changes that affect the users.
      To obtain the list of all changes run `make changelog`. The script fails when a PR in the release
      has no `changelog:` label or has more than one; fix the labels on those PRs and rerun it.
    * The section can be split into sub-section if necessary, e.g. UI Changes, Backend Changes, Bug Fixes, etc.
  * Then upgrade the submodule versions and finally commit. For example:
      ```
      git submodule init
      git submodule update
      pushd jaeger-ui
      git checkout main
      git pull
      git checkout vX.Y.Z  # use the new version
      popd
      ```
      * If there are only dependency bumps, indicate this with "Dependencies upgrades only" ([example](https://github.com/jaegertracing/jaeger-ui/pull/2431/files)).
      * If there are no changes, indicate this with "No changes" ([example](https://github.com/jaegertracing/jaeger/pull/4131/files)).
  * Rotate the below release managers table placing yourself at the bottom. The date should be the first Wednesday of the month.
  * Commit, push and open a PR.
  * Add label `changelog:skip` to the pull request.

## Manual release

Create a release on [GitHub Releases](https://github.com/jaegertracing/jaeger/releases/):

  * Title "Prepare Release v2.x.x"
  * Tag `v2.x.x` (note the `v` prefix) and choose appropriate branch (usually `main`)
  * Copy the new CHANGELOG.md section into the release notes
  * Extra: GitHub has a button "generate release notes". Those are not formatted as we want,
    but it has a nice feature of explicitly listing first-time contributors.
    Before doing the previous step, you can click that button and then remove everything
    except the New Contributors section. Change the header to `### 👏 New Contributors`,
    then copy the main changelog above it. [Example](https://github.com/jaegertracing/jaeger/releases/tag/v1.55.0).

## 🔧 Patch Release

Sometimes we need to do a patch release, e.g. to fix a newly introduced bug. If the main branch already contains newer changes, it is recommended that a patch release is done from a version branch.

Maintenance branches should follow naming convention: `release-major.minor` (e.g.`release-1.8`).

1. Find the commit in `main` for the release you want to patch (e.g., `a49094c2` for v1.34.0).
2. `git checkout ${commit}; git checkout -b ${branch-name}`. The branch name should be in the form `release-major.minor`, e.g., `release-1.34`. Push the branch to the upstream repository.
3. Apply fixes to the branch. The recommended way is to merge the fixes into `main` first and then cherry-pick them into the version branch (e.g., `git cherry-pick c733708c` for the fix going into `v1.34.1`).
4. Follow the regular process for creating a release (except for the Documentation step).
   * When creating a release on GitHub, pick the version branch when applying the new tag.
   * Once the release tag is created, the `ci-release` workflow will kick in and deploy the artifacts for the patch release.
5. Do not perform a new release of the documentation since the major.minor is not changing. The one change that may be useful is bumping the `binariesLatest` variable in the `config.toml` file ([example](https://github.com/jaegertracing/documentation/commit/eacb52f332a7e069c254e652a6b4a58ea5a07b32)).

## 👥 Release managers

A Release Manager is the person responsible for ensuring that a new version of Jaeger is released. This person will coordinate the required changes, including to the related components such as UI, IDL, and jaeger-lib and will address any problems that might happen during the release, making sure that the documentation above is correct.

In order to ensure that knowledge about releasing Jaeger is spread among maintainers, we rotate the role of Release Manager among maintainers.

Here are the release managers for future versions with the tentative release dates. The release dates are the first Wednesday of the month, and we might skip a release if not enough changes happened since the previous release. In such case, the next tentative release date is the first Wednesday of the subsequent month.

| Version | Release Manager | Tentative release date    |
|---------|-----------------|---------------------------|
| 2.24.0  | @pavolloffay    | 2 December 2026           |
| 2.25.0  | @joe-elliott    | 6 January 2027            |
| 2.26.0  | @yurishkuro     | 3 February 2027           |
| 2.27.0  | @jkowall        | 3 March 2027              |
| 2.28.0  | @mahadzaryab1   | 7 April 2027              |
| 2.29.0  | @albertteoh     | 5 May 2027                |
