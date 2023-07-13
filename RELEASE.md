# Jaeger Backend Release Process

1. Create a PR "Prepare release X.Y.Z" against main or maintenance branch ([example](https://github.com/jaegertracing/jaeger/pull/543/files)) by updating CHANGELOG.md to include:
    * A new section with the header `<X.Y.Z> (YYYY-MM-DD)` (copy the template at the top)
    * A curated list of notable changes and links to PRs. Do not simply dump git log, select the changes that affect the users.
      To obtain the list of all changes run `make changelog` or use `scripts/release-notes.py`.
    * The section can be split into sub-section if necessary, e.g. UI Changes, Backend Changes, Bug Fixes, etc.
    * If the submodules have new releases, please also upgrade the submodule versions then commit, for example:
        ```
        cd jaeger-ui
        git ls-remote --tags origin
        git fetch
        git checkout {new_version} //e.g. v1.5.0
        ```
      * Even if a submodule does not have a new release, it should be checked to see if there were any changes warranting cutting a new release and then including it.
      * If there are no changes, indicate this with "No changes" ([example](https://github.com/jaegertracing/jaeger/pull/4131/files)).
    * Rotate the below release managers table placing yourself at the bottom. The date should be the first Wednesday of the month.
2. After the PR is merged, create a release on Github:
    * Automated:
       * `make draft-release`
    * Manual:
       * Title "Release X.Y.Z"
       * Tag `vX.Y.Z` (note the `v` prefix) and choose appropriate branch
       * Copy the new CHANGELOG.md section into the release notes
3. The release tag will trigger a build of the docker images. Since forks don't have jaegertracingbot dockerhub token, they can never publish images to jaegertracing organisation.
   1. Check the images are available on [Docker Hub](https://hub.docker.com/r/jaegertracing/).
   2. For monitoring and troubleshooting, refer to the [jaegertracing/jaeger GithubActions tab](https://github.com/jaegertracing/jaeger/actions).
4. [Publish documentation](https://github.com/jaegertracing/documentation/blob/main/RELEASE.md) for the new version in [jaegertracing.io](https://www.jaegertracing.io/docs/latest).
   1. Check [jaegertracing.io](https://www.jaegertracing.io/docs/latest) redirects to the new documentation release version URL.
   2. For monitoring and troubleshooting, refer to the [jaegertracing/documentation GithubActions tab](https://github.com/jaegertracing/documentation/actions).
5. Announce the release on the [mailing list](https://groups.google.com/g/jaeger-tracing), [slack](https://cloud-native.slack.com/archives/CGG7NFUJ3), and [twitter](https://twitter.com/JaegerTracing?lang=en).

Maintenance branches should follow naming convention: `release-major.minor` (e.g.`release-1.8`).

## Patch Release

Sometimes we need to do a patch release, e.g. to fix a newly introduced bug. If the main branch already contains newer changes, it is recommended that a patch release is done from a version branch.

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
| 1.48.0  | @pavolloffay    | 2 August 2023          |
| 1.49.0  | @joe-elliott    | 6 September 2023       |
| 1.50.0  | @albertteoh     | 4 October 2023         |
| 1.51.0  | @yurishkuro     | 5 November 2023        |
| 1.52.0  | @jkowall        | 5 December 2023        |

