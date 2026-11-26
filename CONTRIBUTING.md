# How to Contribute to Jaeger

We'd love your help!

General contributing guidelines are described in [Contributing Guidelines](./CONTRIBUTING_GUIDELINES.md).

If you use AI tools to help you contribute, read the [AI Usage Policy](./AI_POLICY.md) as well.

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

If you are regenerating protobuf code via `make proto`, you will also need to have Docker installed and running as well
as `uv` for the Python SDK stubs (see [`sdk/python/README.md`](sdk/python/README.md)).

To install `uv` on macOS, run:

```
brew install uv
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

[REVIEW.md](./REVIEW.md) states what a review reports, including how the operational risk of a change is assessed. It applies to a human reviewer and to an AI agent alike.

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

## Upgrading the Go Version

Jaeger builds with the latest Go minor release. The version is declared in the top-level `go.mod` and mirrored into the other `go.mod` files, `.golangci.yml`, and the shared `.github/actions/setup-go` action. That action is the only way CI installs Go, and it hardcodes the version rather than taking one from its callers, so no workflow names a Go version. `scripts/lint/check-go-version.sh` runs as part of `make lint` and fails the build when any of those copies disagrees with the top-level `go.mod`. No Dockerfile here names a `golang` image, because nothing in this repository compiles Go inside a container; the one Go-bearing image is the pre-built debug base image described in step 4.

To upgrade:

1. Change the top-level `go.mod` by hand, e.g. `go mod edit -go=1.27.0`. The script takes its target version from that file, so nothing else can be updated until this is done.
2. Run `./scripts/lint/check-go-version.sh -u` to propagate the version everywhere else. Pass `-v` as well to print a diff of each file it rewrites.
3. Run `make fmt`, `make lint`, and `make test`, and fix whatever the new compiler and the new linter complain about.
4. Upgrade Delve for the new Go version, following [Upgrading for a New Go Release](https://github.com/jaegertracing/base-image-with-debugger#upgrading-for-a-new-go-release). `dlv` refuses to attach to a binary built by a newer Go than it knows about, and the all-in-one integration test runs the debug image, so a Go bump without this step fails CI rather than merely degrading the debug image. Once that image is published, either repoint `scripts/build/docker/debug/Dockerfile` at it or leave it to Renovate, which manages that pin and gets the digest right without being told it. Renovate opens pull requests on a monthly schedule, but the update appears on the [dependency dashboard](https://github.com/jaegertracing/jaeger/issues/5586) ahead of that, and ticking its checkbox there creates the pull request straight away.

[#9478](https://github.com/jaegertracing/jaeger/pull/9478), the upgrade to Go 1.27, is a worked example of the whole sequence.

The `-u` mode only rewrites the minor version. A line that pins a patch version has to be updated manually; the script reports such a line and exits.

The `idl` submodule has its own `go.mod` and is deliberately excluded; its Go version is upgraded through a PR to the [jaeger-idl](https://github.com/jaegertracing/jaeger-idl) repository.

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

A change to an existing default breaks the deployments that rely on the old behavior. Make such a change behind an OTel Collector [feature gate][feature_gates]. A gate switches between the old behavior and the new one, and its stage says which of the two is the default and whether a user can still choose the other. A user enables a gate with `--feature-gates=<id>` and disables it with `--feature-gates=-<id>`. A gate needs no configuration setting of its own.

A gate moves through these stages:

| Stage | Default | Can a user change it? | Code change that moves a gate to this stage |
|---|---|---|---|
| Alpha | old behavior | yes | Register the gate and add the new behavior behind it. |
| Beta | new behavior | yes | Change the stage in the registration. A gate can skip Alpha and start in Beta, which makes the new behavior the default immediately. |
| Stable | new behavior | no, disabling the gate is a startup error | Change the stage, set `ToVersion`, and delete the code for the old behavior. |
| removed | new behavior | no, any mention of the gate is a startup error | Delete the registration. |

A gate moves from Alpha to Beta when the maintainers decide that the new behavior is ready to be the default. After that, the recommended time between stages is two releases. It is a recommendation and not a rule.

Every transition except the introduction of a gate in Alpha changes what a deployment does or what a `--feature-gates` value is accepted, so it is a breaking change. Label its PR `changelog:breaking-change`, which lists the PR under the breaking changes of the release notes, and put the migration instructions in the PR description.

### Conventions

  * **Every gate ID starts with `jaeger.`**, for example `jaeger.es.config.rejectLegacyRotationFlags`. Jaeger shares the process-wide `featuregate.GlobalRegistry()` with the embedded Collector and its contrib components, and the prefix keeps the IDs of Jaeger from colliding with theirs.
  * **`FromVersion` is the release that introduced the gate ID.** Set `WithRegisterFromVersion` once, and do not change it when the gate changes stage.
  * **`ToVersion` is the release that removes the gate ID.** Registration panics when a Stable gate has none. When a user explicitly enables a Stable gate, the log says that the gate will be removed in `ToVersion`.
  * **A rename is a breaking change.** A gate ID is user-facing configuration, and the OTel API has no alias for an ID. Register the new ID, keep the old ID working as an alias through `internal/featuregate.RenamedGate`, and remove the old ID in a later release; the recommended time is two releases. `RenamedGate` supports only Alpha and Beta, so the alias is removed before the gate becomes Stable.

### Applying a Gate Transition

The feature gate lifecycle tracker (https://github.com/jaegertracing/jaeger/issues/9057) lists the registered gates and the next transition of each. A transition that the tracker lists needs no issue of its own. The PR that applies it links the tracker with `Part of #9057` and not with a closing keyword, which would close the tracker.

1. **Read the stage from the code, not from the tracker.** The tracker is maintained by hand and can lag behind the code. This command prints the stage, `FromVersion` and `ToVersion` of every gate: `git grep -n -A5 'GlobalRegistry().MustRegister' -- '*.go' ':!*.pb.go' ':!*_test.go'`.
2. **Apply only what is due.** A transition is due when the tracker schedules it for the next release and does not mark it ✅. Do not advance a gate early, and do not apply a transition that the tracker does not list.
3. **Delete the code for the disabled gate in the PR that makes the gate Stable.** A Stable gate cannot be disabled, even in a test, so from that release the code that ran with the gate disabled is unreachable. The PR deletes every `IsEnabled()` check of the gate, the code that ran when the check was false, and the tests that disabled the gate. The code that ran when the check was true now runs unconditionally. The registration stays, with its `ToVersion`.
4. **In the release that `ToVersion` names, remove only the registration.** Step 3 left nothing else that depends on the gate.
5. **Remove a legacy alias together with its `RenamedGate` wiring.** The `jaeger.`-prefixed gate is then registered directly.
6. **Report a mismatch.** When the code and the tracker disagree, such as a gate at an unexpected stage or a tracker row whose gate is gone, say so in the PR description and do not reconcile them silently.
7. **A maintainer updates the tracker when the PR merges.** The maintainer marks the transition ✅ with a link to the PR, sets `Stage since`, and writes the next transition into the row. The row of a gate ID that the PR removed is deleted.

[feature_gates]: https://github.com/open-telemetry/opentelemetry-collector/blob/main/featuregate/README.md
