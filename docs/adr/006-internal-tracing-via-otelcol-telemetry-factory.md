# Internal Tracing via OTel Collector TelemetryFactory

* **Status**: Proposed
* **Date**: 2026-03-19

## Context

Jaeger v2 is built as an OpenTelemetry Collector distribution. Like any well-instrumented service, it benefits from internal self-tracing: recording spans for query requests, MCP tool calls, and other extension-level operations. At the same time, self-tracing must not create recursive trace loops when Jaeger's own OTLP receiver is the export destination for internal telemetry.

### Current State

Two extensions — `jaegerquery` and `remotestorage` — each call `jtracer.NewProvider` manually at startup:

```go
// jaegerquery/server.go
// TODO OTel-collector does not initialize the tracer currently
// https://github.com/open-telemetry/opentelemetry-collector/issues/7532
tracerProvider, tracerCloser, err := jtracer.NewProvider(ctx, "jaeger")
```

This was the right workaround when the upstream issue was open. Issue [#7532](https://github.com/open-telemetry/opentelemetry-collector/issues/7532) is now **closed**: the Collector does populate `component.TelemetrySettings.TracerProvider`, initialized via `otelcol.Factories.Telemetry`.

Jaeger already sets:

```go
// cmd/jaeger/internal/components.go
Telemetry: otelconftelemetry.NewFactory(),
```

The `otelconftelemetry` factory creates a real `TracerProvider` from the `service.telemetry.traces` YAML block, and a `noopNoContextTracerProvider` when that block is absent (the default). The custom noop intentionally suppresses incoming context propagation to prevent recursive loops in receivers.

### Problem

There are three interlocking issues:

**1. Recursive self-tracing loop.** When Jaeger receives traces over OTLP (the common deployment), enabling internal tracing with the same OTLP endpoint as the export destination creates an infinite loop: each trace batch processed by the receiver generates an internal span, which is exported as a new batch, which is processed again, ad infinitum. This is not theoretical — most users do not reconfigure where internal telemetry goes, so it ends up at `localhost:4317`, which is Jaeger's own OTLP receiver.

**2. Per-extension manual initialization.** Each extension that wants self-tracing must independently call `jtracer.NewProvider`, manage the provider lifecycle (shutdown), and override `telset.TracerProvider`. This is error-prone, duplicative, and means the Collector framework's lifecycle management is bypassed.

**3. Single shared provider with no per-component differentiation.** The `otelconftelemetry` factory creates one `TracerProvider` used by all components. Upstream issue [#10663](https://github.com/open-telemetry/opentelemetry-collector/issues/10663), which would allow per-component provider customization, has had no progress and no clear timeline.

### Why Not Use `otelconf` YAML Configuration?

`go.opentelemetry.io/contrib/otelconf` (v0.22.0, March 2026) is the Go implementation of the [OpenTelemetry Configuration Standard](https://github.com/open-telemetry/opentelemetry-configuration). The spec itself reached v1.0.0 in February 2026. However:

- The Go package remains pre-v1 with 17 of 19 prerequisite items still open ([go-contrib#8422](https://github.com/open-telemetry/opentelemetry-go-contrib/issues/8422)).
- The env var trigger is `OTEL_EXPERIMENTAL_CONFIG_FILE` — explicitly experimental.
- The sampler set is closed: `always_on`, `always_off`, `trace_id_ratio_based`, `parent_based`. The `jaeger_remote` sampler is defined in the schema spec but not implemented in Go. There is no per-operation or per-component sampler.

### A Usable Hook Already Exists

The Collector's `otelcol.Factories.Telemetry` field accepts a `telemetry.Factory` interface. The `telemetry.NewFactory` constructor accepts `telemetry.WithCreateTracerProvider(func)` to override only the tracer creation while delegating everything else (logger, metrics, resource) to the standard `otelconftelemetry` implementation.

Additionally, the Collector's graph layer wraps the `TracerProvider` for every component with `componentattribute.tracerProviderWithAttributes` (internal to the collector, but stable in behavior). This wrapper prepends component-identity attributes to every `Tracer(name, opts...)` call, including:

- `otelcol.component.kind` — one of `"receiver"`, `"processor"`, `"exporter"`, `"connector"`, `"extension"`
- `otelcol.component.id` — e.g., `"otlp"`, `"jaeger_query"`

Because these attributes are injected by the framework before our `TracerProvider.Tracer()` is called, a custom `TracerProvider` returned from `CreateTracerProvider` can inspect them and make a filtering decision — without any knowledge of individual component names or instrumentation scope names.

## Decision

Replace the `otelconftelemetry.NewFactory()` call in `components.go` with a custom factory that delegates everything to `otelconftelemetry` except `CreateTracerProvider`, which we implement as follows:

1. **Create one real `sdktrace.TracerProvider`** using the existing `jtracer` initialization logic: OTLP gRPC exporter configured via `OTEL_EXPORTER_OTLP_ENDPOINT` and related env vars, batch span processor, W3C TraceContext + Baggage propagators.

2. **Wrap it in a `FilteringTracerProvider`** that inspects the `otelcol.component.id` instrumentation attribute on each `Tracer()` call against a fixed allowlist of extensions that are known to produce user-meaningful internal spans:
   - `"jaeger_query"` → delegate to the real provider
   - `"jaeger_mcp"` → delegate to the real provider
   - anything else (all receivers, processors, exporters, connectors, other extensions, or absent) → return a `noopNoContextTracer`

   This default-off / explicit-allowlist policy has two virtues: the recursive loop is closed for all receiver components, and no currently-uninstrumented component accidentally starts emitting spans when a future version adds internal instrumentation. New Jaeger extensions that add meaningful internal tracing opt in by being added to the allowlist.

3. **Remove manual `jtracer.NewProvider` calls** from `jaegerquery/server.go` and `remotestorage/server.go`. Both extensions will use `s.telset.TracerProvider` directly, which the framework populates from the factory. Their `closeTracer` fields and associated shutdown logic also go away.

## Design

### Package Layout

The implementation lives in `cmd/jaeger/internal/telemetry.go`. The core types:

```go
var tracedComponents = map[string]struct{}{
    "jaeger_query": {},
    "jaeger_mcp":   {},
}

// FilteringTracerProvider wraps a real provider and returns a noop tracer
// for all components except those in the explicit allowlist, preventing
// recursive self-tracing and avoiding accidental span emission from
// uninstrumented components.
type FilteringTracerProvider struct {
    real trace.TracerProvider
    noop trace.TracerProvider
}

var componentIDKey = attribute.Key("otelcol.component.id")

func (f *FilteringTracerProvider) Tracer(name string, opts ...trace.TracerOption) trace.Tracer {
    cfg := trace.NewTracerConfig(opts...)
    id, _ := cfg.InstrumentationAttributes().Value(componentIDKey)
    if _, ok := tracedComponents[id.AsString()]; ok {
        return f.real.Tracer(name, opts...)
    }
    return f.noop.Tracer(name, opts...)
}

func (f *FilteringTracerProvider) Shutdown(ctx context.Context) error {
    return f.real.(interface{ Shutdown(context.Context) error }).Shutdown(ctx)
}
```

The noop used for receivers must be the same `noopNoContextTracerProvider` style — returning the original context unchanged from `Start()` — to avoid propagating incoming trace context into a non-recording span that could still be serialized and exported.

### Factory Wiring

`otelconftelemetry` only exports `NewFactory()` — all individual sub-functions are unexported. `WrapFactory` therefore embeds the complete `otelconftelemetry` factory as a delegate and overrides only `CreateTracerProvider`:

```go
// cmd/jaeger/internal/telemetry.go
func WrapFactory(delegate telemetry.Factory) telemetry.Factory {
    return telemetry.NewFactory(
        delegate.CreateDefaultConfig,
        telemetry.WithCreateResource(delegate.CreateResource),
        telemetry.WithCreateLogger(delegate.CreateLogger),
        telemetry.WithCreateMeterProvider(delegate.CreateMeterProvider),
        telemetry.WithCreateTracerProvider(createTracerProvider),
    )
}

// cmd/jaeger/internal/components.go
Telemetry: telemetry.WrapFactory(otelconftelemetry.NewFactory()),
```

### Validating the Attribute Injection

The `otelcol.component.id` injection by `tracerProviderWithAttributes` is in an `internal/` package of the Collector and is observed behavior rather than a contractual API. To catch any upstream change at the next dependency bump, the implementation must include a Go test that exercises the full injection path in-process, using the same pattern as the Collector's own `service_test.go`:

```go
func TestFilteringTracerProvider_FrameworkInjection(t *testing.T) {
    var receiverGotReal, extensionGotReal atomic.Bool

    // recordingTP returns a sentinel tracer; noop returns the standard noop tracer.
    // The test component inspects which it received.
    realTP  := sdktrace.NewTracerProvider()
    checkTP := func(tp trace.TracerProvider, out *atomic.Bool) {
        tr := tp.Tracer("test")
        _, span := tr.Start(context.Background(), "probe")
        out.Store(span.SpanContext().IsValid()) // real spans have valid context
        span.End()
    }

    // Mock receiver whose Start records whether it got the real provider.
    receiverFactory := receiver.NewFactory(component.MustNewType("test_receiver"),
        func() component.Config { return &struct{}{} },
        receiver.WithTraces(func(_ context.Context, set receiver.Settings, _ component.Config, _ consumer.Traces) (receiver.Receiver, error) {
            return &testReceiver{onCreate: func() { checkTP(set.TracerProvider, &receiverGotReal) }}, nil
        }, component.StabilityLevelDevelopment),
    )

    // Mock extension similarly.
    extensionFactory := extension.NewFactory(component.MustNewType("jaeger_query"), ...)

    set := service.Settings{
        TelemetryFactory: jtracer.WrapFactory(otelconftelemetry.NewFactory()),
        // ... nop receivers, exporters, processors
    }
    cfg := /* minimal config wiring the test receiver and jaeger_query extension */

    srv, err := service.New(context.Background(), set, cfg)
    require.NoError(t, err)
    require.NoError(t, srv.Start(context.Background()))
    t.Cleanup(func() { srv.Shutdown(context.Background()) })

    assert.False(t, receiverGotReal.Load(),  "receiver must not get real TracerProvider")
    assert.True(t,  extensionGotReal.Load(), "jaeger_query extension must get real TracerProvider")
}
```

If the Collector renames `otelcol.component.id` or changes how attributes are injected, this test fails at the next `go get` bump — no subprocess or E2E environment required.

### Propagator Initialization

In `otelconftelemetry`, `otel.SetTextMapPropagator` is called **inside** `createTracerProvider` and only when `cfg.Traces.Level != LevelNone`. Since the default config has no `service.telemetry.traces` block, `Level` defaults to `LevelNone`, the noop is returned immediately, and **no global propagator is ever set** by the standard factory.

The global propagator matters: instrumentation libraries like `otelhttp` call `otel.GetTextMapPropagator()` to extract W3C TraceContext from incoming HTTP request headers. Without it, query requests carrying trace context from callers produce no child spans — the trace is silently broken.

`jtracer.NewProvider` currently handles this via a `once.Do` guard:
```go
otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
    propagation.TraceContext{},
    propagation.Baggage{},
))
```

Our `CreateTracerProvider` override must do the same. The `once.Do` guard is still appropriate since `CreateTracerProvider` is called once at service startup. This setup is independent of whether `OTEL_EXPORTER_OTLP_ENDPOINT` is set — propagation should always be active so that incoming trace context is honored even if Jaeger is not exporting its own internal spans anywhere.

The `otelconftelemetry` propagator support (`tracecontext` and `b3` via config) is superseded by our explicit initialization. If users need B3 propagation alongside W3C, the list can be expanded; for now W3C TraceContext + Baggage matches current behavior.

### Sampler and Default Behavior

`sdktrace.NewTracerProvider()` calls `applyTracerProviderEnvConfigs()` at construction time, which reads `OTEL_TRACES_SAMPLER` and applies the appropriate sampler automatically — before any explicit `WithSampler()` option would override it. This is already in effect for `jtracer.NewProvider` today (it calls `sdktrace.NewTracerProvider` without `WithSampler`), and will continue to work with the new design at no extra cost.

The six standard values are handled natively by the SDK (`sdk/trace/sampler_env.go`):

| `OTEL_TRACES_SAMPLER` | Sampler used |
|---|---|
| _(unset)_ | `ParentBased(AlwaysSample)` — SDK default |
| `always_on` | `AlwaysSample` |
| `always_off` | `NeverSample` |
| `traceidratio` | `TraceIDRatioBased(OTEL_TRACES_SAMPLER_ARG)` |
| `parentbased_always_on` | `ParentBased(AlwaysSample)` |
| `parentbased_always_off` | `ParentBased(NeverSample)` |
| `parentbased_traceidratio` | `ParentBased(TraceIDRatioBased(OTEL_TRACES_SAMPLER_ARG))` |

Note: the SDK default (unset) is `ParentBased(AlwaysSample)`, not bare `AlwaysSample`. In practice this means root spans are always sampled, and child spans follow the parent's decision — the expected production-safe default.

**Per-operation sampling via `jaeger_remote`.**
`jaeger_remote` is not in the SDK's built-in switch — it returns `errUnsupportedSampler`. Our `CreateTracerProvider` must handle it explicitly: detect `OTEL_TRACES_SAMPLER=jaeger_remote` and pass `WithSampler(jaegerremote.New(...))` to `sdktrace.NewTracerProvider`. The `OTEL_TRACES_SAMPLER_ARG` parsing is already implemented inside the `jaegerremote` package's `getEnvOptions()`, handling comma-separated key=value pairs:

```
OTEL_TRACES_SAMPLER=jaeger_remote
OTEL_TRACES_SAMPLER_ARG=endpoint=http://localhost:5778/sampling,pollingIntervalMs=5000,initialSamplingRate=0.001
```

The remote sampling server returns per-operation strategies (e.g., "always sample `GET /api/traces`, sample `POST /api/traces` at 0%"), which is the per-endpoint control the standard samplers cannot provide.

Note: `jaeger_remote` is defined in the OTel Configuration Schema v1.0 spec as `jaeger_remote/development` but not yet implemented in Go's `otelconf` library. The env-var wiring described here is independent of that and usable today.

**Limitation.** The sampler applies uniformly to all spans from the allowed extensions (`jaeger_query`, `jaeger_mcp`). With `jaeger_remote`, the sampling server can differentiate by operation name within those extensions. Without a remote server, users are limited to a single process-wide probability.

### The `EnableTracing` Config Field

`jaeger_query` has an `enable_tracing` config field (default `true`). This field is **preserved** — removing it would be a breaking change, and it remains useful as a per-extension opt-out gate.

The factory's allowlist unconditionally enables the real `TracerProvider` for `jaeger_query` at the framework level. The extension itself then applies `EnableTracing` as a local override after receiving `telset.TracerProvider` from the framework:

```go
// jaegerquery/server.go — after receiving telset from the framework
if !s.config.EnableTracing {
    telset.TracerProvider = nooptrace.NewTracerProvider()
}
```

This preserves the existing user-visible behaviour: `enable_tracing: false` disables query tracing regardless of what `OTEL_EXPORTER_OTLP_ENDPOINT` is set to. The same pattern applies to `jaeger_mcp` if it ever gains an equivalent config field.

## User-Facing Changes

This section is intended as the basis for release notes.

**What is new:**

- Internal tracing is now enabled by default for the query service and MCP server. Set `OTEL_EXPORTER_OTLP_ENDPOINT` (and optionally `OTEL_EXPORTER_OTLP_INSECURE=true`) to start receiving Jaeger's own internal spans. No other configuration is required.

- Jaeger no longer creates a recursive self-tracing loop when its OTLP receiver is the export destination for internal telemetry. Receiver components are permanently excluded from tracing.

- Per-operation sampling rates are now configurable for internal spans via the Jaeger remote sampler:
  ```
  OTEL_TRACES_SAMPLER=jaeger_remote
  OTEL_TRACES_SAMPLER_ARG=endpoint=http://localhost:5778/sampling,pollingIntervalMs=5000,initialSamplingRate=0.001
  ```
  The remote sampling server can specify different rates for individual HTTP routes or gRPC methods within the query service or MCP server.

- The standard `OTEL_TRACES_SAMPLER` / `OTEL_TRACES_SAMPLER_ARG` env vars are honoured for all built-in sampler types (`always_on`, `always_off`, `traceidratio`, `parentbased_*`). The default sampler is `parentbased_always_on`.

**What is unchanged:**

- `enable_tracing: false` in the `jaeger_query` config continues to disable query tracing.
- All existing `OTEL_EXPORTER_OTLP_*` and `OTEL_TRACES_*` environment variables work as before.
- The `service.telemetry.traces` YAML block has no effect on tracing; OTEL env vars take precedence.

## Consequences

- Recursive self-tracing loop is closed by design, not by documentation.
- Extensions no longer manage tracer lifecycle; the Collector framework owns it.
- New extensions get internal tracing automatically once added to the allowlist — no per-extension boilerplate.
- The `TODO` comments referencing the now-closed upstream issue #7532 are removed.
- The `otelcol.component.id` attribute injection is observed behavior from an internal Collector package, not a contractual API. The in-process test described in **Validating the Attribute Injection** catches any upstream breakage at the next dependency bump.

## Alternatives Considered

**Use `otelconftelemetry` YAML tracing config as-is.** Requires users to add a `service.telemetry.traces` YAML block with an OTLP exporter config — a different and less familiar configuration paradigm compared to `OTEL_*` env vars. Does not solve the recursive loop by default (loop still occurs if the OTLP endpoint in the YAML points to Jaeger itself). Rejected.

**Keep per-extension `jtracer.NewProvider`.** Does not solve the loop for receivers (which never called `jtracer.NewProvider`, so they always got the framework's noop). Does not benefit new extensions automatically. Rejected as a dead end.

**Filter by instrumentation scope name prefix `go.opentelemetry.io/collector/receiver/`.** Fragile: depends on receiver authors following the naming convention, and would need updating as new receivers are added. Superseded by the component attribute approach.

**Allowlist by `otelcol.component.kind = "extension"`, denylist receivers.** Allows all extensions through rather than only the two that have meaningful internal tracing today. Any future extension that adds instrumentation would emit spans without an explicit decision. Rejected in favour of the more conservative component-id allowlist.

**Wait for upstream issue #10663.** No progress; no timeline. Not viable.
