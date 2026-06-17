# components/

This directory provides the **public API** for building custom Jaeger distributions
using [ocb](https://github.com/open-telemetry/opentelemetry-collector/tree/main/cmd/builder)
(OpenTelemetry Collector Builder).

Each sub-package exports a single `NewFactory` variable (or function) that returns an
OTel Collector component factory. These are thin facades that delegate through
`cmd/jaeger/components/` (which has access to `cmd/jaeger/internal/`) via a
double-dispatch pattern. This keeps Jaeger's internal implementation private while
providing stable, importable paths for distribution assembly.

## Usage with ocb

In your `builder.yaml`, reference these packages:

```yaml
extensions:
  - gomod: github.com/jaegertracing/jaeger v1.72.0
    import: github.com/jaegertracing/jaeger/components/extension/jaegerstorage
```

See `cmd/jaeger/builder.yaml` for a complete reference manifest.

## Why two layers?

Go's `internal` visibility rule is enforced by directory tree: only packages rooted at
the parent of `internal/` can import from it. Since Jaeger's components live under
`cmd/jaeger/internal/`, only packages under `cmd/jaeger/` can access them. The
intermediate layer at `cmd/jaeger/components/` bridges that gap, and this top-level
`components/` directory provides clean user-facing import paths.
