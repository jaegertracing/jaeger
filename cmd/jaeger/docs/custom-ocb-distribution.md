# Building a Custom Jaeger Distribution with OCB

This guide explains how to use the
[OpenTelemetry Collector Builder (OCB)](https://github.com/open-telemetry/opentelemetry-collector/tree/main/cmd/builder)
to create a custom Jaeger binary that retains all Jaeger-specific CLI
commands and initialization hooks.

## Background

When OCB generates a `main.go`, it wraps components in a plain
`otelcol.NewCommand`, which omits Jaeger-specific subcommands such as:

- `version` — print the Jaeger version
- `docs` — generate CLI reference documentation
- `elasticsearch-mappings` — create/update Elasticsearch index mappings

Additionally, when all default components are included and no `--config`
flag is supplied, a standard Jaeger binary falls back to an embedded
all-in-one configuration.

The **Workspace Overwrite** pattern solves this by letting OCB act as a
*code generator only*, removing the generated entrypoint files, and then
providing a custom `main.go` that calls into Jaeger's exported `jaegercli` builder.

## Prerequisites

- [OCB installed](https://github.com/open-telemetry/opentelemetry-collector-releases/releases)
  (`ocb` binary on your `$PATH`, matched to the collector version in Jaeger's `go.mod` for your chosen release tag, e.g. `v0.160.0` for `v2.22.0`)
- Go 1.27+

## Step 1 — Create a `builder-config.yaml`

Rather than maintaining a hand-copied manifest that can drift between releases,
start by copying Jaeger's reference manifest [`cmd/jaeger/builder.yaml`](../builder.yaml)
from the desired release tag (for example, `v2.22.0`).

Make the following adjustments in your copied `builder-config.yaml`:

1. Set `dist.module` to a neutral module path for your project (e.g. `example.com/jaeger-custom`), and set `dist.output_path` (e.g. `./dist`).
2. Replace `v0.0.0` across all `gomod:` entries with the release commit's pseudo-version, such as `v1.76.1-0.20261006114434-462bb49fcff9` for `v2.22.0`.
   > **Note on module versions:** Jaeger's repository module path is `github.com/jaegertracing/jaeger` without a `/v2` suffix. Because Go module tooling requires a `/vN` suffix for major versions v2 or higher, `go mod tidy` rejects `v2.22.0` directly. To obtain the resolvable pseudo-version for any tag, query the tag's commit hash using `go list`:
   > ```bash
   > go list -m github.com/jaegertracing/jaeger@$(git rev-list -n1 v2.22.0)
   > # or by commit SHA directly:
   > go list -m github.com/jaegertracing/jaeger@462bb49fcff9
   > # prints: github.com/jaegertracing/jaeger v1.76.1-0.20261006114434-462bb49fcff9
   > ```
3. Remove the in-repo `replaces:` section at the bottom of the file.
4. Append any custom components to their respective sections (`extensions`, `receivers`, `exporters`, `processors`, or `connectors`).
   > **Rule on component versions:** Always match OCB and custom/contrib components to the exact OpenTelemetry Collector version pinned in Jaeger's `go.mod` for that release tag (e.g. `v0.160.0` for Jaeger `v2.22.0`). Mismatched versions can lead to compile-time API incompatibilities in Collector packages.

Here is an excerpt illustrating the modified sections:

```yaml
dist:
  module: example.com/jaeger-custom
  name: jaeger-custom
  description: Custom Jaeger distribution with additional components
  output_path: ./dist

telemetry:
  gomod: github.com/jaegertracing/jaeger v1.76.1-0.20261006114434-462bb49fcff9
  import: github.com/jaegertracing/jaeger/components/telemetry

extensions:
  # ... standard Jaeger extensions copied from builder.yaml ...
  - gomod: github.com/jaegertracing/jaeger v1.76.1-0.20261006114434-462bb49fcff9
    import: github.com/jaegertracing/jaeger/components/extension/jaegerquery

processors:
  # ... standard Jaeger processors copied from builder.yaml ...
  # Append your custom component (matching the collector version in Jaeger's go.mod, e.g. v0.160.0):
  - gomod: github.com/open-telemetry/opentelemetry-collector-contrib/processor/transformprocessor v0.160.0
    import: github.com/open-telemetry/opentelemetry-collector-contrib/processor/transformprocessor
```

## Step 2 — Run OCB in generator-only mode

Run `ocb` with `--skip-compilation` so it generates sources without compiling, and
include `--skip-strict-versioning` so OCB's version check does not fail on minor collector
version discrepancies:

```bash
ocb --config builder-config.yaml --skip-compilation --skip-strict-versioning
```

OCB writes the generated Go code into `./dist`, including `components.go`,
`main.go`, and platform helper files (`main_others.go`, `main_windows.go`).

## Step 3 — Replace the generated entrypoint

OCB generates `main.go` alongside platform files `main_others.go` and `main_windows.go`.
Because both platform files call `runInteractive` (which is defined in OCB's generated `main.go`),
leaving them in place while replacing only `main.go` leads to `undefined: runInteractive` errors.

Delete all generated `main*.go` files first:

```bash
rm -f dist/main*.go
```

Then provide a replacement `dist/main.go` that invokes `jaegercli.NewCommand`.

For releases that ship it, copy Jaeger's template directly rather than maintaining a duplicate copy:

```bash
cp cmd/jaeger/jaegercli/main.go.tmpl dist/main.go
```

For standalone or out-of-tree builds without a local Jaeger checkout, create `dist/main.go` with the following entrypoint:

```go
// main.go
package main

import (
	"log"

	"github.com/jaegertracing/jaeger/cmd/jaeger/jaegercli"
)

func main() {
	// components() is generated by OCB in dist/components.go
	factories, err := components()
	if err != nil {
		log.Fatalf("failed to build components: %v", err)
	}

	// NewCommand wires all Jaeger-specific subcommands, flags, and default configuration
	if err := jaegercli.NewCommand(factories).Execute(); err != nil {
		log.Fatal(err)
	}
}
```

> **Note:** `components()` (lowercase) is the function OCB generates in
> `dist/components.go`. Do not rename it.

## Step 4 — Build

```bash
cd dist
go mod tidy
go build -o jaeger-custom .
```

## Result

The resulting `jaeger-custom` binary exposes the full Jaeger CLI surface,
including all subcommands that are absent from a plain OCB-generated binary:

```bash
./jaeger-custom --help
./jaeger-custom version
./jaeger-custom docs
./jaeger-custom elasticsearch-mappings --help
```

When all default Jaeger components are retained and no `--config` flag is provided,
the binary falls back to the embedded All-in-One configuration with memory storage.
Note that UI static assets in custom distributions require building or serving separately,
as raw builds bundle placeholder assets. Also note that `./jaeger-custom version` outputs
`git-version=dev` unless compiled with Jaeger's version ldflags (as done in `make build-ocb`).

## Alternative: Programmatic Extension with `jaegercli.Components()`

If you are building a custom binary directly in Go rather than using OCB, you can
start from Jaeger's default components and modify them programmatically using
`jaegercli.Components()`:

```go
package main

import (
	"log"

	"github.com/jaegertracing/jaeger/cmd/jaeger/jaegercli"
)

func main() {
	// Retrieve Jaeger's default set of component factories
	factories, err := jaegercli.Components()
	if err != nil {
		log.Fatalf("failed to retrieve components: %v", err)
	}

	// Add custom components to the factory maps:
	// factories.Extensions[myExtension.NewFactory().Type()] = myExtension.NewFactory()
	// factories.Receivers[myReceiver.NewFactory().Type()] = myReceiver.NewFactory()

	if err := jaegercli.NewCommand(factories).Execute(); err != nil {
		log.Fatal(err)
	}
}
```

## How It Works

```
builder-config.yaml
        │
        ▼
  OCB (--skip-compilation --skip-strict-versioning)
        │  generates
        ├─▶ dist/components.go      (component factories)
        └─▶ dist/main*.go           (deleted: main.go, main_others.go, main_windows.go)
                                            │
                                            ▼ replaced with
  Your main.go (or main.go.tmpl)
        │  calls
        ├─▶ components()            (OCB-generated factories)
        └─▶ jaegercli.NewCommand()  (Jaeger's full CLI engine)
                │
                ├─▶ embedded all-in-one config fallback
                └─▶ version, docs, elasticsearch-mappings, ...
```

This approach provides:
- All component dependencies resolve through Jaeger's module graph.
- No need to clone or fork the Jaeger repository.
- Full access to Jaeger-specific subcommands and default All-in-One configuration behavior.
- Complete freedom to append custom OpenTelemetry Collector components.
