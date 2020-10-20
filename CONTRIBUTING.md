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

# Runs all unit tests
make test
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

The packaged assets can be enabled by providing a build tag `ui`, for example:

```
$ go run -tags ui ./cmd/all-in-one/main.go
```

`make run-all-in-one` essentially runs Jaeger all-in-one by combining both of the above
steps into a single `make` command.

## Project Structure

These are general guidelines on how to organize source code in this repository.

```
github.com/jaegertracing/jaeger
  cmd/                      - All binaries go here
    agent/
      app/                  - The actual code for the binary
      main.go
    collector/
      app/                  - The actual code for the binary
      main.go
  crossdock/                - Cross-repo integration test configuration
  examples/
      hotrod/               - Demo application that uses OpenTracing API
  idl/                      - (submodule) https://github.com/jaegertracing/jaeger-idl
  jaeger-ui/                - (submodule) https://github.com/jaegertracing/jaeger-ui
  model/                    - Where models are kept, e.g. Process, Span, Trace
  pkg/                      - (See Note 1)
  plugin/                   - Swappable implementations of various components
    storage/
      cassandra/            - Cassandra implementations of storage APIs
        .                   - Shared Cassandra stuff
        spanstore/          - SpanReader / SpanWriter implementations
        dependencystore/
      elasticsearch/        - ES implementations of storage APIs
  scripts/                  - Miscellaneous project scripts, e.g. license update script
    travis/                 - Travis scripts called in .travis.yml
  storage/
    spanstore/              - SpanReader / SpanWriter interfaces
    dependencystore/
  thrift-gen/               - Generated Thrift types
    agent/
    jaeger/
    sampling/
    zipkincore/
  go.mod                    - Go module file to track dependencies
```

- Note 1: `pkg` is a collection of utility packages used by the Jaeger components
  without being specific to its internals. Utility packages are kept separate from
  the Jaeger core codebase to keep it as small and concise as possible. If some
  utilities grow larger and their APIs stabilize, they may be moved to their own
  repository, to facilitate re-use by other projects.

## Imports grouping

This projects follows the following pattern for grouping imports in Go files:

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

### Combining code coverage

We use [cover.sh](./scripts/cover.sh) script to run tests and combine code coverage from all packages
(see also [issue # 797](https://github.com/jaegertracing/jaeger/issues/797)).

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
Before merging a PR make sure:
* the title is descriptive and follows [a good commit message](./CONTRIBUTING_GUIDELINES.md)
* pull request is assigned to the current release milestone
* add `changelog:*` and other labels

Merge the PR by using "Squash and merge" option on Github. Avoid creating merge commits.
After the merge make sure referenced issues were closed.
