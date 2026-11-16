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
  (`github.com/jaegertracing/jaeger`) without pinning individual upstream versions. Their
  actual versions are resolved transitively through Jaeger's `go.mod`, ensuring version
  consistency across the distribution.

## Usage with ocb

In your `builder.yaml`, reference these packages:

```yaml
extensions:
  - gomod: github.com/jaegertracing/jaeger v2.19.0
    import: github.com/jaegertracing/jaeger/components/extension/jaegerstorage
  - gomod: github.com/jaegertracing/jaeger v2.19.0
    import: github.com/jaegertracing/jaeger/components/ext/extension/healthcheckv2extension
```

See `cmd/jaeger/builder.yaml` for a complete reference manifest.

## Batching processors

Jaeger exposes both the legacy `batch` processor and its upstream replacement,
`queue_batch`, in the standard binary and OCB manifest. Custom distributions can
reference `github.com/jaegertracing/jaeger/components/ext/processor/queuebatchprocessor`
through their Jaeger `gomod` entry.

OpenTelemetry has [deprecated the batch processor](https://opentelemetry.io/blog/2026/collector-batching-migration/),
with exporter batching defaults planned for Collector v0.171.0 (February 2027)
and processor removal for v0.177.0 (May 2027). Exporter-level batching is the
preferred migration for most pipelines; `queue_batch` supports pipelines that
still need batching before downstream processors or multiple exporters.

Existing `batch` configurations remain supported. Adding the replacement does
not change Jaeger's shipped pipeline configurations or batching defaults.
