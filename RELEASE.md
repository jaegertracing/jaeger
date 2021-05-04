# Jaeger Backend Release Process

1. Create a PR "Preparing release X.Y.Z" against master or maintenance branch ([example](https://github.com/jaegertracing/jaeger/pull/543/files)) by updating CHANGELOG.md to include:
    * A new section with the header `<X.Y.Z> (YYYY-MM-DD)`
    * A curated list of notable changes and links to PRs. Do not simply dump git log, select the changes that affect the users. To obtain the list of all changes run `make changelog`.
    * The section can be split into sub-section if necessary, e.g. UI Changes, Backend Changes, Bug Fixes, etc.
    * If the submodules have new releases, please also upgrade the submodule versions then commit, for example:
        ```
        cd jaeger-ui
        git ls-remote --tags origin
        git fetch
        git checkout {new_version} //e.g. v1.5.0
        ```
2. Add all merged pull requests to the milestone for the release and create a new milestone for a next release e.g. `Release 1.16`.
3. After the PR is merged, create a release on Github:
    * Title "Release X.Y.Z" 
    * Tag `vX.Y.Z` (note the `v` prefix) and choose appropriate branch
    * Copy the new CHANGELOG.md section into the release notes
4. The release tag will trigger a build of the docker images. Since forks don't have jaegertracingbot dockerhub token, they can never publish images to jaegertracing organisation.
   1. Check the images are available on [Docker Hub](https://hub.docker.com/r/jaegertracing/).
   2. For monitoring and troubleshooting, refer to the [jaegertracing/jaeger GithubActions tab](https://github.com/jaegertracing/jaeger/actions).
5. [Publish documentation](https://github.com/jaegertracing/documentation/blob/master/RELEASE.md) for the new version in [jaegertracing.io](https://www.jaegertracing.io/docs/latest).
   1. Check [jaegertracing.io](https://www.jaegertracing.io/docs/latest) redirects to the new documentation release version URL.
   2. For monitoring and troubleshooting, refer to the [jaegertracing/documentation GithubActions tab](https://github.com/jaegertracing/documentation/actions).
6. Announce the release on the [mailing list](https://groups.google.com/g/jaeger-tracing), [slack](https://cloud-native.slack.com/archives/CGG7NFUJ3), and [twitter](https://twitter.com/JaegerTracing?lang=en).

Maintenance branches should follow naming convention: `release-major.minor` (e.g.`release-1.8`).
