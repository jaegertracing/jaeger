# How to Contribute to Jaeger

We'd love your help!

General contributing guidelines are described in [Contributing Guidelines](./CONTRIBUTING_GUIDELINES.md).

Jaeger is [Apache 2.0 licensed](LICENSE) and accepts contributions via GitHub
pull requests. This document outlines some of the conventions on development
workflow, commit message formatting, contact points and other resources to make
it easier to get your contribution accepted.

We gratefully welcome improvements to documentation as well as to code.

## Getting Started

### Pre-requisites
* Install [Go](https://golang.org/doc/install) and setup GOPATH and add $GOPATH/bin in PATH

This library uses Go modules to manage dependencies.

If you are running `make test` or other Makefile targets on macOS, please ensure that you have GNU `sed` installed.

To install GNU `sed`:

```bash
brew install gnu-sed
```

```
git clone git@github.com:jaegertracing/jaeger.git jaeger
cd jaeger
```

Then install dependencies and run the tests:

```
# Adds the jaeger-ui submodule
git submodule update --init --recursive

# Installs required tools
make install-tools

# Runs all unit tests:
make test
```

### Contributing Code

We accept new changes as pull requests on GitHub. Please make sure the following conditions are met before submitting PRs:

1. Use a named branch in your fork, not the `main` branch, otherwise the CI jobs will fail and we won't be able to merge the PR.
2. All commits in the PR must be signed (verified by the DCO check on GitHub).
3. Before submitting a PR, make sure to run:
```
make fmt  # commit all changes from auto-format
make lint
make test
```


### Auto-format

We are currently using `gofumpt`, which is installed automatically by `make install-tools` as part of `golangci-lint` installation. We recommend configuring your IDE to run `gofumpt` on file saves, e.g. in VSCode:

```json
"go.formatTool": "gofumpt",
"gopls": {
    "formatting.gofumpt": true,
}
```

### Running local build with the UI

```
$ go run ./cmd/jaeger --config ./cmd/jaeger/config.yaml
```

#### What does this command do?

The Jaeger binary runs with the default configuration file (config.yaml) that includes
the UI configuration via the `jaeger_query` extension. The `jaeger-ui` submodule, which was added from the Pre-requisites step above, contains the source code for the UI assets (requires Node.js 24+). The assets must be compiled first with `make build-ui`, which normally downloads them from the latest UI release, but can also build them from source.

## Project Structure

These are general guidelines on how to organize source code in this repository.

```
github.com/jaegertracing/jaeger
  cmd/                      - All binaries go here
    jaeger/                 - The main Jaeger binary (v2) that combines collector, query, and ingester
    anonymizer/             - Utility to anonymize traces from Jaeger query and save to file
    tracegen/               - Utility to generate a steady flow of simple traces
    es-index-cleaner/       - Utility to purge old indices from Elasticsearch
    es-rollover/            - Utility to manage Elastic Search indices
    esmapping-generator/    - Utility to generate Elasticsearch mapping
    remote-storage/         - Component to enable sharing single-node storage implementations via Remote Storage API v2
  examples/
    grafana-integration/    - Demo application combining Jaeger, Grafana, Loki, Prometheus
    hotrod/                 - Demo application demonstrating tracing instrumentation
    otel-demo/              - Demo application using OpenTelemetry Collector and Jaeger
  docker-compose/           - Docker-compose recipes to simulate different Jaeger deployments
    monitor/                - Service Performance Monitoring (SPM) Development/Demo Environment
  idl/                      - (submodule) https://github.com/jaegertracing/jaeger-idl
  jaeger-ui/                - (submodule) https://github.com/jaegertracing/jaeger-ui
  internal/                 - Internal modules that make up Jaeger
    storage/                - Trace/Metrics Storage interfaces and implementations
      metricstore/          - Metrics Storage interface and implementations (e.g. Prometheus, Elasticsearch)
      v1/                   - Trace Storage v1 interfaces and implementations (Cassandra, Elasticsearch, Badger, etc.)
      v2/                   - Trace Storage v2 interfaces and implementations (gRPC, ClickHouse, etc.)
  monitoring/               - Jaeger monitoring assets (e.g. jaeger-mixin)
  ports/                    - Centralized port definitions
  scripts/                  - Miscellaneous project scripts, e.g. github action and license update script
  go.mod                    - Go module file to track dependencies
  Makefile                  - Define various recipes to automate build, test, and deployment tasks
```

## Imports grouping

This project follows the following pattern for grouping imports in Go files:

- imports from standard library
- imports from other projects
- imports from `jaeger` project

For example:

```go
import (
	"fmt"

	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/agent/app"
	"github.com/jaegertracing/jaeger/cmd/collector/app/builder"
)
```

## Testing guidelines

**Policy**: All new functionality must include tests. Bug fixes should include regression tests that would have caught the bug, where feasible. Pull requests without adequate test coverage will not be merged.

We strive to maintain as high code coverage as possible. The current repository limit is set at 95%,
with some exclusions discussed below.

### Packages with no tests

Since `go test` command does not generate
code coverage information for packages that have no test files, we have a build step (`make nocover`)
that breaks the build when such packages are discovered, with the following error:

```
error: at least one *_test.go file must be in all directories with go files
       so that they are counted for code coverage.
       If no tests are possible for a package (e.g. it only defines types), create empty_test.go
```

As the message says, all packages are required to have at least one `*_test.go` file.

### Excluding packages from testing

There are conditions that cannot be tested without external dependencies, such as a function that
creates a `gocql.Session`, because it requires an active connection to Cassandra database. It is
recommended to isolate such functions in a separate package with bare minimum of code and add a
file `.nocover` to exclude the package from coverage calculations. The file should contain
a comment explaining why it is there, for example:

```
$ cat ./pkg/cassandra/config/.nocover
requires connection to Cassandra
```

## Code review requirements

Jaeger changes are reviewed through GitHub pull requests. Reviewers are expected to evaluate whether a change is appropriate for the project, whether the implementation is correct and maintainable, and whether it is covered by suitable tests and documentation.

Before approving a pull request, reviewers should check that:

* the change is understandable, scoped to the stated problem, and consistent with the project's architecture and coding style,
* new behavior has tests, bug fixes include regression coverage where feasible, and required CI checks are passing,
* security-sensitive changes, dependency changes, authentication or authorization changes, network-facing behavior, release tooling, and configuration defaults receive extra scrutiny from maintainers familiar with the affected area,
* generated files are not edited manually; contributors must update the source definitions and regenerate files with the documented targets,
* the pull request title and commit history are suitable for the project's squash-merge workflow.

Non-trivial pull requests should be reviewed and approved by a maintainer or knowledgeable contributor other than the author before merge. Trivial documentation, typo, formatting, or mechanical follow-up changes may be merged by a maintainer without a separate approval when the risk is low and CI is passing.

## Merging PRs
**For maintainers:** before merging a PR make sure the title is descriptive and follows [a good commit message](./CONTRIBUTING_GUIDELINES.md)

Merge the PR by using "Squash and merge" option on Github. Avoid creating merge commits.
After the merge make sure referenced issues were closed.

## Deprecating CLI Flags

* If a flag is deprecated in release N, it can be removed in release N+2 or three months later, whichever is later.
* When adding a (deprecated) prefix to the flags, indicate via a deprecation message that the flag could be removed in the future. For example:
  ```
  (deprecated, will be removed after 2020-03-15 or in release v1.19.0, whichever is later)
  ```
* At the top of the file where the flag name is defined, add a constant and a comment, e.g.
  ```
  // TODO deprecated flag to be removed
  healthCheckHTTPPortWarning = "(deprecated, will be removed after 2020-03-15 or in release v1.19.0, whichever is later)"
  ```
* Use that constant as the prefix to the help text, e.g.
  ```
  flagSet.Int(healthCheckHTTPPort, 0, healthCheckHTTPPortWarning+" see --"+adminHTTPHostPort)
  ```
* When parsing a deprecated flag into config, log a warning with the same deprecation message
* Take care of deprecated flags in `initFromViper` functions, do not pass them to business functions.

### Removing Deprecated CLI Flags
* Ensure all references to the flag's variables have been removed in code.
* Ensure a "Breaking Changes" entry is added in the [CHANGELOG](./CHANGELOG.md) indicating which CLI flag
is being removed and which CLI flag should be used in favor of this removed flag.

For example:
```
* Remove deprecated flags `--old-flag`, please use `--new-flag` ([#1234](<pull-request URL>), [@myusername](https://github.com/myusername))
```

## Using Feature Gates for Breaking Changes

As much as possible, use OTel Collector's [feature gates][feature_gates] to manage breaking changes. For example, consider that we discovered a bug in the existing behavior, such as https://github.com/jaegertracing/jaeger/issues/5270. Simply changing the behavior might be a breaking change, so we implement a new behavior and create an internal config setting that enables or disables it. But how will users ever know and be encouraged to migrate to the new behavior? For that we can create a feature gate (without even creating any additional user-facing configuration), as follows:
  * Introduce a new feature gate, with the name `jaeger.***`.
  * If we don't want to change the default behavior right away, we can start the feature in the Alpha state, where it is disabled by default. No breaking changes need to be called out in the changelog.
  * If we do want to change the default behavior right away, we can start the feature in the Beta state, where it is enabled by default, but the user can still disable it. Call out a breaking change in the changelog.
  * Two releases later change the gate to Stable, where it is not only enabled by default, but trying to disable it will cause a runtime error. The code for the old behavior should be removed. Call out a breaking change in the changelog.
  * Two releases later remove the feature gate as unused. Call out a breaking change in the changelog.

See https://github.com/jaegertracing/jaeger/pull/6441 for an example of this workflow.

Conventions for Jaeger feature gates:
  * **Naming.** Every new gate ID MUST use the `jaeger.` prefix (e.g. `jaeger.es.config.rejectLegacyRotationFlags`). Jaeger shares the process-wide OTel `featuregate.GlobalRegistry()` with the embedded Collector and its contrib components, and the prefix avoids ID collisions with their gates. A legacy ID that predates this convention (e.g. `storage.clickhouse`) may stay registered without the prefix for the duration of its deprecation/removal window, but it must not be the canonical ID for new work: introduce the `jaeger.`-prefixed name and treat the old one as a deprecated alias per the renaming cycle below.
  * **`FromVersion` records introduction, not stage.** Set `WithRegisterFromVersion` to the release in which the gate ID is first added, and do not change it when the gate graduates between stages — it is not a "current stage since" marker.
  * **`ToVersion` is the removal release.** It is required once a gate is Stable or Deprecated (registration panics otherwise) and names the release in which the gate ID is removed. A Stable gate can no longer be disabled; explicitly enabling one logs that it will be removed in `ToVersion`.
  * **Renaming a gate is itself a breaking change.** A gate ID is user-facing config (`--feature-gates=<id>`) and the OTel API has no built-in ID alias, so a rename needs a deprecation cycle: register the new ID and keep the old one working as a deprecated alias via `internal/featuregate.RenamedGate` for a window, then remove the old ID in a later release. `RenamedGate` supports only Alpha and Beta, so a renamed gate must have its legacy alias removed before it can be promoted to Stable.

The current inventory of registered gates and their recommended transitions is tracked in https://github.com/jaegertracing/jaeger/issues/9057.

[feature_gates]: https://github.com/open-telemetry/opentelemetry-collector/blob/main/featuregate/README.md
