# Lazy Storage Factory Initialization

* **Status**: Implemented; graduated from [RFC 0009](../rfc/0009-lazy-storage-factory-initialization.md)
* **Decided**: 2026-01-20 — delivered in [#7887](https://github.com/jaegertracing/jaeger/pull/7887), error propagation later refined in [#8593](https://github.com/jaegertracing/jaeger/pull/8593)
* **Describes the implementation as of**: 2026-07-25

> **Graduation note:** The proposal behind this decision — the problem analysis, the two options weighed against each other, and the recommendation — lives in [RFC 0009](../rfc/0009-lazy-storage-factory-initialization.md). This ADR describes the implementation as it stands today.

## Context

The `jaeger_storage` extension lets a deployment declare any number of storage backends by name, and consumers request the ones they need by name — so a configuration routinely declares backends that a given deployment never uses. Constructing all of them at startup, as the extension originally did, meant an unused backend still opened connections, allocated memory and started background goroutines, and an unused backend that happened to be unavailable failed the whole process even when the storage actually serving traffic was healthy.

Initialization is therefore deferred to first use, accepting that a connection failure surfaces when a component first requests the storage rather than at startup. [RFC 0009](../rfc/0009-lazy-storage-factory-initialization.md) covers that trade-off and the rejected alternative — a two-phase `Configure`/`Initialize` framework imposed on every backend factory.

## Decision

Storage factories are created on first request and cached; configuration is validated at startup, separately from initialization.

### Startup validates configuration and creates nothing

[`storageExt.Start`](../../cmd/jaeger/internal/extension/jaegerstorage/extension.go) records the Collector `component.Host` on its telemetry settings, installs the metrics factory, and then validates every declared backend, failing with `invalid configuration for trace storage '<name>'` (or `metric storage`). It creates no factories and opens no connections.

Validation is per-backend: [`TraceBackend.Validate`](../../cmd/internal/storageconfig/config.go) and `MetricBackend.Validate` reject an entry with no backend type (`empty configuration`) and one naming several (`multiple backend types found for trace storage`). `storageconfig.Config` additionally requires at least one trace backend. Because that `Config` is registered as an `xconfmap.Validator`, the Collector already validates it while loading configuration; `Start` re-checks the same backends, so a malformed entry cannot reach factory creation.

### Factories are built on demand, under a mutex, and cached

`TraceStorageFactory(name)` and `MetricStorageFactory(name)` each take `factoryMu`, return the cached factory if one exists, and otherwise look the name up in the extension's configuration. An unknown name yields `storage '<name>' not declared in '<extension>' extension configuration` (`metric storage '<name>' not declared …` on the metric path); a known one is passed to `storageconfig.CreateTraceStorageFactory` — which is where connections are actually established — and the result is cached in the corresponding map for subsequent callers.

A backend that is declared but never requested is therefore never constructed at all.

### The lookup methods return an error, not a bool

The `Extension` interface exposes `TraceStorageFactory(name string) (tracestore.Factory, error)` and `MetricStorageFactory(name string) (storage.MetricStoreFactory, error)`. Returning an error rather than a `bool` lets callers distinguish "not declared in configuration" from "initialization failed" and surface the underlying cause.

The package-level helpers `GetTraceStoreFactory`, `GetMetricStorageFactory`, `GetSamplingStoreFactory` and `GetPurger` return that error unchanged. They originally re-wrapped it as `cannot find definition of storage '<name>' …`, which claimed the name was missing from configuration even when the name resolved and initialization was what failed; [#8593](https://github.com/jaegertracing/jaeger/pull/8593) dropped the wrapper, since the two lookup methods already describe both cases accurately.

### Shutdown closes only what was built

`Shutdown` iterates the populated factory maps and closes each entry implementing `io.Closer`, joining any errors. Entries for unrequested backends are absent rather than empty, so nothing has to be skipped.

### The factory interfaces are untouched

Lazy initialization lives entirely in the extension. `tracestore.Factory` implementations know nothing about it: there is no `Configure`, `Initialize` or `IsInitialized` anywhere in the tree, and a backend factory still connects in its constructor. What changed is only *when* the extension calls that constructor.

## Consequences

### Positive

- A declared but unused backend consumes no connections, memory, or goroutines.
- Startup succeeds when an unused backend is unavailable, so one broken archive backend cannot keep Jaeger from serving.
- Backend factory implementations carry no lazy-initialization logic, so the behavior stays confined to the extension and its callers.
- Error returns name the failing storage and the reason.

### Negative

- A connection failure for a declared backend surfaces when a component first requests it, not at startup.
- Validation covers the shape of a backend's configuration, not its reachability, so a wrong host or credential is still a first-use failure.

### Neutral

- Storage initialization log output appears at first access rather than during startup.
- The factory maps are populated incrementally, so anything iterating them observes only the backends built so far.

## References

- [RFC 0009: Lazy Storage Factory Initialization](../rfc/0009-lazy-storage-factory-initialization.md) — the proposal, alternatives, and recommendation
- Extension implementation: [`cmd/jaeger/internal/extension/jaegerstorage/extension.go`](../../cmd/jaeger/internal/extension/jaegerstorage/extension.go)
- Configuration and validation: [`cmd/internal/storageconfig/config.go`](../../cmd/internal/storageconfig/config.go)
- Factory creation: [`cmd/internal/storageconfig/factory.go`](../../cmd/internal/storageconfig/factory.go)
- Factory interface: [`internal/storage/v2/api/tracestore/factory.go`](../../internal/storage/v2/api/tracestore/factory.go)
