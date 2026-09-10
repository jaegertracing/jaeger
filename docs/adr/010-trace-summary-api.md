# ADR-010: Trace Summary API for Lightweight Search Results

* **Status**: Implemented; graduated from [RFC 0011](../rfc/0011-trace-summary-api.md)
* **Decided**: 2026-05-21 — delivered across five milestones, [#8604](https://github.com/jaegertracing/jaeger/pull/8604) through [#8812](https://github.com/jaegertracing/jaeger/pull/8812); storage interface reshaped afterwards in [#9067](https://github.com/jaegertracing/jaeger/pull/9067)
* **Describes the implementation as of**: 2026-07-25

> **Graduation note:** The proposal behind this decision — the motivation, the four alternatives weighed, and the five-milestone plan with its delivery history — lives in [RFC 0011](../rfc/0011-trace-summary-api.md). This ADR describes the summary API as it stands today.

## Context

The search-results screen needs only a handful of fields per trace: root service and operation, start and end timestamps, span and error counts, and a per-service breakdown. Serving that from `FindTraces` means shipping every span of every matching trace and aggregating in the browser, which costs tens of kilobytes per trace where a few dozen bytes suffice. Elasticsearch and similar backends can compute the same aggregates far more cheaply server-side.

`FindTraceSummaries` is therefore a first-class operation from the wire protocol down to the storage interface, with a fallback so that backends unable to aggregate natively still serve the endpoint. [RFC 0011](../rfc/0011-trace-summary-api.md) covers why this is a separate endpoint rather than a flag on `FindTraces`.

## Decision

### The wire API

`api_v3.QueryService` exposes `FindTraceSummaries(FindTraceSummariesRequest) returns (stream FindTraceSummariesResponse)`, reusing the same `TraceQueryParameters` as `FindTraces` so no new query-parameter parsing exists. The HTTP gateway serves it at `GET /api/v3/trace-summaries` (`routeFindSummaries` in [`apiv3/http_gateway.go`](../../cmd/jaeger/internal/extension/jaegerquery/internal/apiv3/http_gateway.go)), collecting the iterator before writing the response; the gRPC handler streams response chunks as they arrive.

`TraceSummary` carries the trace ID, root service and operation names, `min_start_time_unix_nano` / `max_end_time_unix_nano`, span, error-span and orphan-span counts, and a `ServiceSummary` per distinct service sorted by name. Duration is deliberately absent — callers derive it from the two timestamps. The timestamps are `fixed64`, which proto3 JSON encodes as decimal strings, so JavaScript consumers avoid `float64` precision loss above 2^53; the HTTP gateway gets this for free by marshalling the generated response type with `gogoproto/jsonpb`.

### `FindTraceSummaries` is a method on `tracestore.Reader`

Every [`tracestore.Reader`](../../internal/storage/v2/api/tracestore/reader.go) implements `FindTraceSummaries(ctx, TraceQueryParams) iter.Seq2[[]TraceSummary, error]`. A backend that cannot compute summaries natively must yield `errors.ErrUnsupported` (wrapped with `%w`) as its first error, before any batch. Embedding [`tracestore.UnsupportedTraceSummaries`](../../internal/storage/v2/api/tracestore/summary.go) supplies exactly that, so opting out costs one line — ClickHouse, Cassandra, memory, the v1 adapter and the bare Elasticsearch reader all do.

This replaced an optional `tracestore.SummaryReader` interface discovered by type assertion, which [#9067](https://github.com/jaegertracing/jaeger/pull/9067) deleted. The optional shape imposed a composition tax: every decorator wrapping a `Reader` had to re-detect and re-expose the capability or silently drop it, which produced a shadow `ReadMetricsDecoratorWithSummary` type and a wrapper whose mere presence encoded the capability. A required method with an embeddable default removes that class of bug — a decorator that forwards the method cannot lose the capability. The original motivation for optionality, a remote storage server without the RPC, never needed it: gRPC returns `codes.Unimplemented`, which the client normalizes to `errors.ErrUnsupported`.

### The fallback lives in `QueryService`

[`QueryService.FindTraceSummaries`](../../cmd/jaeger/internal/extension/jaegerquery/querysvc/service.go) calls the reader directly — no capability check — and on a first yielded `errors.ErrUnsupported` switches to `computeSummaries` over `FindTraces`. That path runs `jptrace.AggregateTraces` so a trace split across consecutive chunks yields exactly one summary, and applies the clock-skew adjuster before summarizing. Any other error is passed through and stops the iterator.

Callers therefore see one behavior regardless of backend: the endpoint always works, and native aggregation is a performance property rather than a feature flag in the API.

### Native support, and who has it

Elasticsearch/OpenSearch computes summaries in a single storage-side aggregation, behind the `jaeger.es.nativeTraceSummaries` feature gate — Beta and enabled by default since v2.20.0, and requiring inline Painless scripts on the cluster. When the gate is off, or the cluster rejects scripting, the reader yields `errors.ErrUnsupported` and the query service falls back transparently.

The remote storage gRPC adapter forwards the RPC and translates `codes.Unimplemented` into `errors.ErrUnsupported`. For server-streaming RPCs the server's status arrives on the first `Recv()` rather than at stream open, which the client iterator handles.

The shared storage integration suite always runs a `FindTraceSummaries` sub-test; a backend without native support opts out through `capabilities.FindTraceSummariesTest` in its skip list, so the gap is declared per backend rather than silently skipped.

### Consumers

The endpoint backs the UI search screen, which no longer aggregates full traces client-side, and the `search_traces` MCP tool, which consumes `QueryService.FindTraceSummaries` directly rather than the HTTP route.

## Consequences

### Positive

- The search path transfers only what it renders, so response size no longer scales with spans per trace.
- A backend can make search dramatically cheaper by implementing one method, with no API or UI change.
- Because the method is on `Reader`, a decorator that forwards it cannot accidentally drop native support — the previous optional-interface shape lost it twice this way.
- The endpoint's behavior is backend-independent: unsupported backends fall back rather than failing.

### Negative

- Every `Reader` implementation must now provide the method, even if only by embedding the mixin — the cost of removing the optional interface.
- The fallback re-reads full traces, so a backend without native support gets the tidier API and none of the performance benefit.
- Summary types are maintained in parallel with the `TracesData`-based responses, in the proto, in Go, and in the UI.

### Neutral

- Native Elasticsearch aggregation depends on Painless scripting being enabled, so the same deployment can be fast or fall back depending on cluster policy.
- `min_start_time_unix_nano` and `max_end_time_unix_nano` are JSON strings; consumers must parse them (`BigInt` in the UI) rather than treating them as numbers.

## References

* [RFC 0011: Trace Summary API for Lightweight Search Results](../rfc/0011-trace-summary-api.md) — motivation, alternatives, and milestone history
* Storage interface: [`internal/storage/v2/api/tracestore/reader.go`](../../internal/storage/v2/api/tracestore/reader.go), [`summary.go`](../../internal/storage/v2/api/tracestore/summary.go)
* Query service and fallback: [`querysvc/service.go`](../../cmd/jaeger/internal/extension/jaegerquery/querysvc/service.go)
* gRPC and HTTP v3 handlers: [`internal/apiv3/`](../../cmd/jaeger/internal/extension/jaegerquery/internal/apiv3/)
* Native Elasticsearch aggregation: [`internal/storage/v2/elasticsearch/tracestore/summary.go`](../../internal/storage/v2/elasticsearch/tracestore/summary.go)
* Remote storage adapter: [`internal/storage/v2/grpc/`](../../internal/storage/v2/grpc/)
* Integration-test opt-out: [`internal/storage/integration/capabilities/capabilities.go`](../../internal/storage/integration/capabilities/capabilities.go)
