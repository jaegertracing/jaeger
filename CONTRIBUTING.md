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
$ make run-all-in-one
```

#### What does this command do?

The `jaeger-ui` submodule, which was added from the Pre-requisites step above, contains
the source code for the UI assets (requires Node.js 6+).

The assets must be compiled first with `make build-ui`, which runs Node.js build and then
packages the assets into a Go file that is `.gitignore`-ed.

`make run-all-in-one` essentially runs Jaeger all-in-one by combining both of the above
steps into a single `make` command.

## Project Structure

These are general guidelines on how to organize source code in this repository.

```
github.com/jaegertracing/jaeger
  cmd/                      - All binaries go here
    all-in-one/             - Jaeger all-in-one application, designed for quick local testing
    jaeger/                 - Jaeger V2 binary
    collector/              - Component to receive spans (from agents or directly from clients) and saves them in Trace Storage
    ingester/               - Component to read spans from Kafka topic and save them to storage
    query/                  - Component to serve Jaeger UI and an API that retrieves traces from storage
    remote-storage/         - Component to enable sharing single-node storage implementations like memstore by implementing Remote Storage API
    anonymizer/             - Utility to anonymize traces from Jaeger query and save to file
    tracegen/               - Utility to generate a steady flow of simple traces
    es-index-cleaner/       - Utility to purge old indices from Elasticsearch
    es-rollover/            - Utility to manage Elastic Search indices
  crossdock/                - Cross-repo integration test configuration
  examples/
    grafana-integration/    - Demo application that combine Jaeger, Grafana, Loki, Prometheus to demonstrate logs, metrics and traces correlation
    hotrod/                 - Demo application that demonstrates the use of tracing instrumentation
  docker-compose/           - Docker-compose recipes to simulate different Jaeger deployments
    kafka/                  - Jaeger depoyment utilizing collector-Kafka-injester pipeline
    monitor/                - Service Performance Monitoring (SPM) Development/Demo Environment
  idl/                      - (submodule) https://github.com/jaegertracing/jaeger-idl
  jaeger-ui/                - (submodule) https://github.com/jaegertracing/jaeger-ui
  internal/                 - Internal modules that make up Jaeger
    storage/                - Define and implement Trace/Metrics Storage interface 
      metricstore/          - Define and implement Metrics Storage interface
        prometheus/         - Prometheus implementation of Metrics Storage
      v1/                   - V1 Trace Storage interfaces and implementations
        memory/             - In-memory implementation
        elasticsearch/      - ElasticSearch implementation
      v2/                   - V2 Trace Storage interfaces and implementation
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

[feature_gates]: https://github.com/open-telemetry/opentelemetry-collector/blob/main/featuregate/README.md
