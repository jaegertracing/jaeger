# Jaeger Backend Release Process

1. Create a PR "Preparing release X.Y.Z" against master or maintenance branch ([example](https://github.com/jaegertracing/jaeger/pull/543/files)) by updating CHANGELOG.md to include:
    * A new section with the header `<X.Y.Z> (YYYY-MM-DD)`
    * The section can be split into sub-section if necessary, e.g. UI Changes, Backend Changes, Bug Fixes, etc.
    * A curated list of notable changes and links to PRs. Do not simply dump git log, select the changes that affect the users. To obtain the list of all changes run `make changelog`.
2. After the PR is merged, create a release on Github:
    * Title "Release X.Y.Z" 
    * Tag `vX.Y.Z` (note the `v` prefix) and choose appropriate branch
    * Copy the new CHANGELOG.md section into the release notes
3. The release tag will trigger a build of the docker images
4. Once the images are available on [Docker Hub](https://hub.docker.com/r/jaegertracing/), announce the release on the mailing list, gitter, and twitter.
5. Publish documentation for the new version in [jaegertracing.io](https://github.com/jaegertracing/documentation).

Maintenance branches should follow naming convention: `release-major.minor` (e.g.`release-1.8`).
