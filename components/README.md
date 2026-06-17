# components/

This directory provides the **public API** for building custom Jaeger distributions
using [ocb](https://github.com/open-telemetry/opentelemetry-collector/tree/main/cmd/builder)
(OpenTelemetry Collector Builder).

## Layout

- **Top-level packages** (`extension/`, `exporter/`, `processor/`, `telemetry/`) — Jaeger-specific
  components. These use a double-dispatch pattern through `cmd/jaeger/components/` to access
  `cmd/jaeger/internal/` while keeping internal APIs private.

- **`ext/`** — Third-party OTel Collector components (from otel-contrib and collector core).
  These are simple one-line aliases that directly re-export the upstream `NewFactory`.
  They exist so that `builder.yaml` can reference all components via a single `gomod` entry
  without pinning individual upstream versions.

## Usage with ocb

In your `builder.yaml`, reference these packages:

```yaml
extensions:
  - gomod: github.com/jaegertracing/jaeger v1.72.0
    import: github.com/jaegertracing/jaeger/components/extension/jaegerstorage
  - gomod: github.com/jaegertracing/jaeger v1.72.0
    import: github.com/jaegertracing/jaeger/components/ext/extension/healthcheckv2extension
```

See `cmd/jaeger/builder.yaml` for a complete reference manifest.
