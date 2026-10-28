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

Additionally, a standard Jaeger binary defaults to an embedded all-in-one
configuration when no `--config` flag is supplied.

The **Workspace Overwrite** pattern solves this by letting OCB act as a
*code generator only*, removing the generated entrypoint files, and then
providing a custom `main.go` that calls into Jaeger's exported `jaegercli` builder.

## Prerequisites

- [OCB installed](https://github.com/open-telemetry/opentelemetry-collector/releases)
  (`ocb` binary on your `$PATH`)
- Go 1.26+

## Step 1 — Create a `builder-config.yaml`

Start from Jaeger's reference builder manifest (`cmd/jaeger/builder.yaml`).
Use a current release version (e.g. `v2.22.0`) across all `gomod:` entries so
that OCB resolves all component facades and telemetry through Jaeger's curated dependencies.
Do not include in-repo `replaces:`.

To include your own custom components, append them to the appropriate section
(`extensions`, `receivers`, `exporters`, `processors`, or `connectors`).

```yaml
dist:
  module: github.com/jaegertracing/jaeger/cmd/jaeger/ocb-build
  name: jaeger-custom
  description: Custom Jaeger distribution
  output_path: ./dist

telemetry:
  gomod: github.com/jaegertracing/jaeger v2.22.0
  import: github.com/jaegertracing/jaeger/components/telemetry

extensions:
  - gomod: github.com/jaegertracing/jaeger v2.22.0
    import: github.com/jaegertracing/jaeger/components/ext/extension/healthcheckv2extension
  - gomod: github.com/jaegertracing/jaeger v2.22.0
    import: github.com/jaegertracing/jaeger/components/ext/extension/pprofextension
  - gomod: github.com/jaegertracing/jaeger v2.22.0
    import: github.com/jaegertracing/jaeger/components/ext/extension/zpagesextension
  - gomod: github.com/jaegertracing/jaeger v2.22.0
    import: github.com/jaegertracing/jaeger/components/ext/extension/basicauthextension
  - gomod: github.com/jaegertracing/jaeger v2.22.0
    import: github.com/jaegertracing/jaeger/components/ext/extension/sigv4authextension
  - gomod: github.com/jaegertracing/jaeger v2.22.0
    import: github.com/jaegertracing/jaeger/components/extension/jaegerquery
  - gomod: github.com/jaegertracing/jaeger v2.22.0
    import: github.com/jaegertracing/jaeger/components/extension/jaegerstorage
  - gomod: github.com/jaegertracing/jaeger v2.22.0
    import: github.com/jaegertracing/jaeger/components/extension/remotesampling
  - gomod: github.com/jaegertracing/jaeger v2.22.0
    import: github.com/jaegertracing/jaeger/components/extension/expvar
  - gomod: github.com/jaegertracing/jaeger v2.22.0
    import: github.com/jaegertracing/jaeger/components/extension/remotestorage

receivers:
  - gomod: github.com/jaegertracing/jaeger v2.22.0
    import: github.com/jaegertracing/jaeger/components/ext/receiver/otlpreceiver
  - gomod: github.com/jaegertracing/jaeger v2.22.0
    import: github.com/jaegertracing/jaeger/components/ext/receiver/nopreceiver
  - gomod: github.com/jaegertracing/jaeger v2.22.0
    import: github.com/jaegertracing/jaeger/components/ext/receiver/jaegerreceiver
  - gomod: github.com/jaegertracing/jaeger v2.22.0
    import: github.com/jaegertracing/jaeger/components/ext/receiver/kafkareceiver
  - gomod: github.com/jaegertracing/jaeger v2.22.0
    import: github.com/jaegertracing/jaeger/components/ext/receiver/zipkinreceiver

exporters:
  - gomod: github.com/jaegertracing/jaeger v2.22.0
    import: github.com/jaegertracing/jaeger/components/ext/exporter/debugexporter
  - gomod: github.com/jaegertracing/jaeger v2.22.0
    import: github.com/jaegertracing/jaeger/components/ext/exporter/otlpexporter
  - gomod: github.com/jaegertracing/jaeger v2.22.0
    import: github.com/jaegertracing/jaeger/components/ext/exporter/otlphttpexporter
  - gomod: github.com/jaegertracing/jaeger v2.22.0
    import: github.com/jaegertracing/jaeger/components/ext/exporter/nopexporter
  - gomod: github.com/jaegertracing/jaeger v2.22.0
    import: github.com/jaegertracing/jaeger/components/exporter/storageexporter
  - gomod: github.com/jaegertracing/jaeger v2.22.0
    import: github.com/jaegertracing/jaeger/components/ext/exporter/kafkaexporter
  - gomod: github.com/jaegertracing/jaeger v2.22.0
    import: github.com/jaegertracing/jaeger/components/ext/exporter/prometheusexporter

processors:
  - gomod: github.com/jaegertracing/jaeger v2.22.0
    import: github.com/jaegertracing/jaeger/components/ext/processor/batchprocessor
  - gomod: github.com/jaegertracing/jaeger v2.22.0
    import: github.com/jaegertracing/jaeger/components/ext/processor/memorylimiterprocessor
  - gomod: github.com/jaegertracing/jaeger v2.22.0
    import: github.com/jaegertracing/jaeger/components/ext/processor/tailsamplingprocessor
  - gomod: github.com/jaegertracing/jaeger v2.22.0
    import: github.com/jaegertracing/jaeger/components/ext/processor/attributesprocessor
  - gomod: github.com/jaegertracing/jaeger v2.22.0
    import: github.com/jaegertracing/jaeger/components/ext/processor/filterprocessor
  - gomod: github.com/jaegertracing/jaeger v2.22.0
    import: github.com/jaegertracing/jaeger/components/processor/adaptivesampling

connectors:
  - gomod: github.com/jaegertracing/jaeger v2.22.0
    import: github.com/jaegertracing/jaeger/components/ext/connector/forwardconnector
  - gomod: github.com/jaegertracing/jaeger v2.22.0
    import: github.com/jaegertracing/jaeger/components/ext/connector/spanmetricsconnector
```

## Step 2 — Run OCB in generator-only mode

Run `ocb` with `--skip-compilation` so it only acts as a code generator:

```bash
ocb --config builder-config.yaml --skip-compilation
```

OCB generates the source tree in `./dist`, including `components.go`, `main.go`,
and platform helper files (`main_others.go`, `main_windows.go`).

## Step 3 — Replace the generated entrypoint

OCB generates `main.go` alongside platform files `main_others.go` and `main_windows.go`.
Because both platform files call `runInteractive` (which is defined in OCB's generated `main.go`),
simply overwriting `main.go` leaves dangling references that break `go build`.

Delete all generated `main*.go` files first:

```bash
rm -f dist/main*.go
```

Then create a replacement `dist/main.go` that invokes `jaegercli.NewCommand`:

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

When run with no arguments or `--config` flag, it starts in default All-in-One
mode just like the official Jaeger binary.

## Alternative: Programmatic Extension with `jaegercli.Components()`

If you are developing a custom binary directly in Go rather than using OCB, you can
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
  OCB (--skip-compilation)
        │  generates
        ├─▶ dist/components.go      (component factories)
        └─▶ dist/main*.go           (deleted: main.go, main_others.go, main_windows.go)
                                            │
                                            ▼ replaced with
  Your main.go
        │  calls
        ├─▶ components()            (OCB-generated factories)
        └─▶ jaegercli.NewCommand()  (Jaeger's full CLI engine)
                │
                ├─▶ embedded all-in-one config fallback
                └─▶ version, docs, elasticsearch-mappings, ...
```

This approach gives you:

- ✅ A conflict-free dependency graph using Jaeger's curated facade modules
- ✅ No need to clone the Jaeger repository
- ✅ All Jaeger-specific CLI tooling and default All-in-One behavior preserved
- ✅ Freedom to add custom OpenTelemetry Collector components via OCB
