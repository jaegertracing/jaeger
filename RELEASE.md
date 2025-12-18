# Jaeger Overall Release Process

## Start Here: Create Release Issue

**This is the main entry point for the release process.**

Run the following command to create a release issue with the full checklist:

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
```

## Release Steps

1. Perform UI release according to https://github.com/jaegertracing/jaeger-ui/blob/main/RELEASE.md
2. Perform Backend release (see below)
3. [Publish documentation](https://github.com/jaegertracing/documentation/blob/main/RELEASE.md) for the new version on `jaegertracing.io`.
   1. Check that [jaegertracing.io](https://www.jaegertracing.io/docs/latest) redirects to the new documentation release version URL.
   2. For monitoring and troubleshooting, refer to the [jaegertracing/documentation Github Actions tab](https://github.com/jaegertracing/documentation/actions).
4. Announce the release on the [mailing list](https://groups.google.com/g/jaeger-tracing), [slack](https://cloud-native.slack.com/archives/CGG7NFUJ3), and [twitter](https://twitter.com/JaegerTracing?lang=en).

# Jaeger Backend Release Process

<!-- BEGIN_CHECKLIST -->

1. Create a PR "Prepare release vX.Y.Z" against main or maintenance branch ([example](https://github.com/jaegertracing/jaeger/pull/6826)).
    * **Automated**: `make prepare-release VERSION=X.Y.Z`
      * Updates CHANGELOG.md (generates content via `make changelog`)
      * Upgrades jaeger-ui submodule to the corresponding version
      * Rotates release managers table
      * Creates PR with label `changelog:skip`
    * **Manual**: Update CHANGELOG.md to include:
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
      * Add label `changelog:skip` to the pull request.
2. After the PR is merged, create new release tag (command is in the PR description if using automation):
    ```
    git checkout main
    git pull
    git tag vX.Y.Z -s -m "Release vX.Y.Z" # use the new version
    git push upstream vX.Y.Z
    ```
3. Create a release on Github:
    * Automated:
       * `make draft-release`
4. Go to [Publish Release](https://github.com/jaegertracing/jaeger/actions/workflows/ci-release.yml) workflow on GitHub
   and run it manually using Run Workflow button on the right.
   1. For monitoring and troubleshooting, open the logs of the workflow run from above URL.
   2. Check the images are available on [Docker Hub](https://hub.docker.com/r/jaegertracing/)
      and binaries are uploaded [to the release](https://github.com/jaegertracing/jaeger/releases)

<!-- END_CHECKLIST -->

## Manual release

    * Manual:
       * Title "Prepare Release v1.x.x / v2.x.x"
       * Tag `v1.x.x` (note the `v` prefix) and choose appropriate branch (usually `main`)
       * Copy the new CHANGELOG.md section into the release notes
       * Extra: GitHub has a button "generate release notes". Those are not formatted as we want,
         but it has a nice feature of explicitly listing first-time contributors.
         Before doing the previous step, you can click that button and then remove everything
         except the New Contributors section. Change the header to `### 👏 New Contributors`,
         then copy the main changelog above it. [Example](https://github.com/jaegertracing/jaeger/releases/tag/v1.55.0).

## Patch Release

Sometimes we need to do a patch release, e.g. to fix a newly introduced bug. If the main branch already contains newer changes, it is recommended that a patch release is done from a version branch.

Maintenance branches should follow naming convention: `release-major.minor` (e.g.`release-1.8`).

1. Find the commit in `main` for the release you want to patch (e.g., `a49094c2` for v1.34.0).
2. `git checkout ${commit}; git checkout -b ${branch-name}`. The branch name should be in the form `release-major.minor`, e.g., `release-1.34`. Push the branch to the upstream repository.
3. Apply fixes to the branch. The recommended way is to merge the fixes into `main` first and then cherry-pick them into the version branch (e.g., `git cherry-pick c733708c` for the fix going into `v1.34.1`).
4. Follow the regular process for creating a release (except for the Documentation step).
   * When creating a release on GitHub, pick the version branch when applying the new tag.
   * Once the release tag is created, the `ci-release` workflow will kick in and deploy the artifacts for the patch release.
5. Do not perform a new release of the documentation since the major.minor is not changing. The one change that may be useful is bumping the `binariesLatest` variable in the `config.toml` file ([example](https://github.com/jaegertracing/documentation/commit/eacb52f332a7e069c254e652a6b4a58ea5a07b32)).

## Release managers

A Release Manager is the person responsible for ensuring that a new version of Jaeger is released. This person will coordinate the required changes, including to the related components such as UI, IDL, and jaeger-lib and will address any problems that might happen during the release, making sure that the documentation above is correct.

In order to ensure that knowledge about releasing Jaeger is spread among maintainers, we rotate the role of Release Manager among maintainers.

Here are the release managers for future versions with the tentative release dates. The release dates are the first Wednesday of the month, and we might skip a release if not enough changes happened since the previous release. In such case, the next tentative release date is the first Wednesday of the subsequent month.

| Version | Release Manager | Tentative release date |
|---------|-----------------|------------------------|
| 2.14.0  | @mahadzaryab1   | 7 January   2026       |
| 2.15.0  | @jkowall        | 4 February  2026       |
| 2.16.0  | @yurishkuro     | 5 March     2026       |
| 2.17.0  | @albertteoh     | 1 April     2026       |
| 2.18.0  | @pavolloffay    | 6 May       2026       |
| 2.19.0  | @joe-elliott    | 3 June      2026       |
