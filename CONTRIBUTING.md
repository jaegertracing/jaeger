# How to Contribute to Jaeger

We'd love your help!

Jaeger is [Apache 2.0 licensed](LICENSE) and accepts contributions via GitHub
pull requests. This document outlines some of the conventions on development
workflow, commit message formatting, contact points and other resources to make
it easier to get your contribution accepted.

We gratefully welcome improvements to documentation as well as to code.

# Certificate of Origin

By contributing to this project you agree to the [Developer Certificate of
Origin](https://developercertificate.org/) (DCO). This document was created
by the Linux Kernel community and is a simple statement that you, as a
contributor, have the legal right to make the contribution. See the [DCO](DCO)
file for details.

## Getting Started

This library uses [glide](https://github.com/Masterminds/glide) to manage dependencies.

To get started:

```bash
git submodule update --init --recursive
glide install
make test
```

## Project Structure

These are general guidelines on how to organize source code in this repository.

```
github.com/uber/jaeger
  cmd/                      - All binaries go here
    agent/
      app/                  - The actual code for the binary
      main.go
    collector/
      app/                  - The actual code for the binary
      main.go
  pkg/                      - See Note 1
  plugin/                   - Swappable implementations of various components
    storage/
      cassandra/            - Cassandra implementations of storage APIs
        .                   - Shared Cassandra stuff
        spanstore/          - SpanReader / SpanWriter implementations
        dependencystore/
      elasticsearch/        - ES implementations of storage APIs
  storage/
    spanstore/              - SpanReader / SpanWriter interfaces
    dependencystore/
  idl/                      - (submodule)
  jaeger-ui/                - (submodule)
  thrift-gen/               - Generated Thrift types
    agent/
    jaeger/
    sampling/
    zipkincore/
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

	"github.com/uber/jaeger/cmd/agent/app"
	"github.com/uber/jaeger/cmd/collector/app/builder"
)
```

## Making A Change

*Before making any significant changes, please [open an
issue](https://github.com/uber/jaeger/issues).* Discussing your proposed
changes ahead of time will make the contribution process smooth for everyone.

Once we've discussed your changes and you've got your code ready, make sure
that tests are passing (`make test` or `make cover`) and open your PR. Your
pull request is most likely to be accepted if it:

* Includes tests for new functionality.
* Follows the guidelines in [Effective
  Go](https://golang.org/doc/effective_go.html) and the [Go team's common code
  review comments](https://github.com/golang/go/wiki/CodeReviewComments).
* Has a [good commit
  message](http://tbaggery.com/2008/04/19/a-note-about-git-commit-messages.html).

## License

By contributing your code, you agree to license your contribution under the terms
of the [Apache License](LICENSE).

If you are adding a new file it should have a header like below.  The easiest
way to add such header is to run `make fmt`.

```
// Copyright (c) 2017 Uber Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
```
