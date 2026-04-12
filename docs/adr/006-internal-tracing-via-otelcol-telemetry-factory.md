# Internal Tracing via OTel Collector TelemetryFactory

* **Status**: Implemented
* **Date**: 2026-03-19

## Context

Jaeger v2 is built as an OpenTelemetry Collector distribution. Like any well-instrumented service, it benefits from internal self-tracing: recording spans for query requests, MCP tool calls, and other extension-level operations. At the same time, self-tracing must not create recursive trace loops when Jaeger's own OTLP receiver is the export destination for internal telemetry.

Previously, two extensions (`jaegerquery` and `remotestorage`) each called `jtracer.NewProvider` manually at startup — a workaround for upstream Collector issue [#7532](https://github.com/open-telemetry/opentelemetry-collector/issues/7532), which is now closed. With the issue resolved, the Collector properly populates `component.TelemetrySettings.TracerProvider` for every component via `otelcol.Factories.Telemetry`.

### Problem

Three interlocking issues motivated this change:

1. **Recursive self-tracing loop.** When Jaeger's OTLP receiver is the export destination for internal telemetry (the common deployment), each trace batch processed by the receiver generates an internal span that is exported as a new batch — ad infinitum.

2. **Per-extension manual initialization.** Each extension that wanted self-tracing had to independently call `jtracer.NewProvider`, manage provider lifecycle, and override `telset.TracerProvider`. This is error-prone and bypasses the Collector framework's lifecycle management.

3. **No per-component provider differentiation.** The standard `otelconftelemetry` factory creates one `TracerProvider` shared by all components. Upstream issue [#10663](https://github.com/open-telemetry/opentelemetry-collector/issues/10663), which would allow per-component customization, has had no progress.

## Decision

Replace the `otelconftelemetry.NewFactory()` call in `components.go` with a custom `WrapFactory` that delegates everything to `otelconftelemetry` except `CreateTracerProvider`.

The custom factory creates one real `TracerProvider` (via the existing `jtracer` initialization) wrapped in a `FilteringTracerProvider`. This wrapper inspects the `otelcol.component.id` instrumentation attribute that the Collector framework injects into every `Tracer()` call, and routes to the real provider only for an explicit allowlist of extensions known to produce meaningful internal spans:

- `jaeger_query`
- `jaeger_mcp`

All other components — receivers, processors, exporters, connectors, and unlisted extensions — receive a noop tracer. This default-off / explicit-allowlist policy closes the recursive loop by design and prevents uninstrumented components from accidentally emitting spans when they add internal instrumentation in the future.

Manual `jtracer.NewProvider` calls are removed from `jaegerquery/server.go` and `remotestorage/server.go`. Both extensions now use `telset.TracerProvider` directly, populated by the framework.

The `enable_tracing: false` config field in `jaeger_query` is preserved as a per-extension opt-out applied after the framework provides the provider.

## Consequences

- Recursive self-tracing loop is closed by design, not by documentation.
- Extensions no longer manage tracer lifecycle; the Collector framework owns it.
- New extensions get internal tracing by being added to the allowlist — no per-extension boilerplate.
- The `otelcol.component.id` attribute injection is observed behavior from an internal Collector package, not a contractual API. An in-process test catches any upstream breakage at the next dependency bump.

## Alternatives Considered

**Use `otelconftelemetry` YAML tracing config as-is.** Requires users to add a `service.telemetry.traces` YAML block with an OTLP exporter config — a different and less familiar configuration paradigm compared to `OTEL_*` env vars. Does not solve the recursive loop by default (loop still occurs if the OTLP endpoint in the YAML points to Jaeger itself). Rejected.

**Keep per-extension `jtracer.NewProvider`.** Does not solve the loop for receivers (which never called `jtracer.NewProvider`, so they always got the framework's noop). Does not benefit new extensions automatically. Rejected as a dead end.

**Filter by instrumentation scope name prefix `go.opentelemetry.io/collector/receiver/`.** Fragile: depends on receiver authors following the naming convention, and would need updating as new receivers are added. Superseded by the component attribute approach.

**Allowlist by `otelcol.component.kind = "extension"`, denylist receivers.** Allows all extensions through rather than only the two that have meaningful internal tracing today. Any future extension that adds instrumentation would emit spans without an explicit decision. Rejected in favour of the more conservative component-id allowlist.

**Wait for upstream issue #10663.** No progress; no roadmap. Low confidence.
