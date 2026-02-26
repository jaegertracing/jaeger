# Jaeger Overall Release Process

## ⭐ Start Here: Create Tracking Issue for Release ⭐

Run the following command to create a tracking issue with the full checklist:

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

1. Perform UI release according to <https://github.com/jaegertracing/jaeger-ui/blob/main/RELEASE.md>
2. Perform Backend release (see below)
3. [Publish documentation](https://github.com/jaegertracing/documentation/blob/main/RELEASE.md) for the new version on `jaegertracing.io`.

# ⚙️ Jaeger Backend Release Process

<!-- BEGIN_CHECKLIST -->

1. Create a PR "Prepare release vX.Y.Z" against main or maintenance branch ([example](https://github.com/jaegertracing/jaeger/pull/6826)).
  * **Automated**:
    ```bash
    make prepare-release VERSION=X.Y.Z
    ```
    * Updates CHANGELOG.md (generates content via `make changelog`)
    * Upgrades jaeger-ui submodule to the corresponding version
    * Rotates release managers table
    * Creates PR with label `changelog:skip`
  * Manual: See [Manual release pull request](https://github.com/jaegertracing/jaeger/blob/main/RELEASE.md#manual-release-pull-request).
2. After the PR is merged, create a release on Github:
  * **Automated**:
    ```bash
    make draft-release
    ```
  * Manual: See [Manual release](https://github.com/jaegertracing/jaeger/blob/main/RELEASE.md#manual-release).
3. Once the release is created, the [Publish Release workflow](https://github.com/jaegertracing/jaeger/actions/workflows/ci-release.yml) will run to build artifacts.
  * Wait for the workflow to finish. For monitoring and troubleshooting, open the logs of the workflow run from above URL.
  * Check the images are available on [Docker Hub](https://hub.docker.com/r/jaegertracing/) and binaries are uploaded [to the release](https://github.com/jaegertracing/jaeger/releases).

<!-- END_CHECKLIST -->

## Manual release pull request

Create a PR "Prepare release vX.Y.Z" against main or maintenance branch ([example](https://github.com/jaegertracing/jaeger/pull/6826)).

  * Update CHANGELOG.md to include:
    * A new section with the header `vX.Y.Z (YYYY-MM-DD)` (copy the template at the top)
    * A curated list of notable changes and links to PRs. Do not simply dump git log, select the changes that affect the users.
      To obtain the list of all changes run `make changelog`.
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
| 2.16.0  | @mahadzaryab1   | 4 March 2026              |
| 2.17.0  | @albertteoh     | 1 April 2026              |
| 2.18.0  | @pavolloffay    | 6 May 2026                |
| 2.19.0  | @joe-elliott    | 3 June 2026               |
| 2.20.0  | @yurishkuro     | 1 July 2026               |
| 2.21.0  | @jkowall        | 5 August 2026             |
