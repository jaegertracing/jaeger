# How to Contribute to Jaeger

We'd love your help!

General contributing guidelines are described in [Contributing Guidelines](./CONTRIBUTING_GUIDELINES.md).

Jaeger is [Apache 2.0 licensed](LICENSE) and accepts contributions via GitHub
pull requests. This document outlines some of the conventions on development
workflow, commit message formatting, contact points and other resources to make
it easier to get your contribution accepted.

We gratefully welcome improvements to documentation as well as to code.

## Getting Started

This library uses [glide](https://github.com/Masterminds/glide) to manage dependencies.

To get started, make sure you clone the Git repository into the correct location `github.com/jaegertracing/jaeger` relative to `$GOPATH`:

```
mkdir -p $GOPATH/src/github.com/jaegertracing
cd $GOPATH/src/github.com/jaegertracing
git clone git@github.com:jaegertracing/jaeger.git jaeger
cd jaeger
```

Then install dependencies and run the tests:

```
git submodule update --init --recursive
glide install
make test
```

### Running local build with the UI

The `jaeger-ui` submodule contains the source code for the UI assets (requires Node.js 6+). The assets must be compiled first with `make build_ui`, which runs Node.js build and then packages the assets into a Go file that is `.gitignore`-ed. The packaged assets can be enabled by providing a build tag `ui`, e.g.:

```
$ go run -tags ui ./cmd/all-in-one/main.go
```

Alternatively, the path to the built UI assets can be provided via `--query.static-files` flag:

```
$ go run ./cmd/all-in-one/main.go --query.static-files jaeger-ui/build
```

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
  docs/                     - Documentation
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
  glide.yaml                - Glide is the project's dependency manager
  mkdocs.yml                - MkDocs builds the documentation in docs/
```

  * Note 1: `pkg` is a collection of utility packages used by the Jaeger components
    without being specific to its internals. Utility packages are kept separate from
    the Jaeger core codebase to keep it as small and concise as possible. If some
    utilities grow larger and their APIs stabilize, they may be moved to their own
    repository, to facilitate re-use by other projects.

## Imports grouping

This projects follows the following pattern for grouping imports in Go files:
  * imports from standard library
  * imports from other projects
  * imports from `jaeger` project
  
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

