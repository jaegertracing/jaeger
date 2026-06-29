# ADR-010: Trace Summary API for Lightweight Search Results

* **Status**: In progress (✅ Milestones 1, 2, 3, and 4 complete; ⏳ Milestone 5 pending)
* **Date**: 2026-05-21
* **Last updated**: 2026-06-29

## Context

The Jaeger UI search screen calls `GET /api/traces` (or `/api/v3/traces`) to display a
list of matching traces. The response today contains **full trace data** — every span with
all its attributes. The UI then post-processes that data locally to render a compact result
row for each trace (see `ResultItem.tsx` and `transformTraceData()`):

* Root service name and operation name (derived from the root span)
* Trace duration (latest span end time − earliest span start time)
* Total span count
* Total error span count (spans with OTEL `StatusCode.ERROR`)
* Per-service breakdown: for each distinct service name, the number of spans belonging
  to that service and the count of those spans that carry `StatusCode.ERROR` — rendered
  as a coloured tag with an optional error icon when `error_span_count > 0`,
  e.g. `frontend (12) ⚠`. Only spans directly owned by the service are counted; there
  is no error propagation from child spans of other services.
* Trace start time (absolute + relative)

The scatter plot in the search header also uses span count (bubble size) and the
presence of any error (bubble colour).

Returning full traces just to display a handful of summary fields makes the protocol
unnecessarily heavy. A trace with hundreds of spans may be tens of kilobytes of JSON, yet
the search results screen only needs a few dozen bytes per trace. For users who work with
high-cardinality services or long traces this manifests as noticeable latency and memory
pressure in the browser.

Some storage backends (e.g. Elasticsearch, ClickHouse) can compute these aggregates
server-side far more cheaply than fetching all spans and shipping them to the UI.

This ADR proposes a new **`FindTraceSummaries`** endpoint in the Jaeger API v3, a
matching **`TraceSummary`** data model, the propagation of that method through the Go
`tracestore.Reader` interface and the Remote Storage gRPC API, and a fallback path for
storage backends that do not implement native summary computation.

### Affected Repositories

| Repository | Role |
|---|---|
| `jaegertracing/jaeger-idl` | Proto definitions (`api_v3`, `storage/v2`) |
| `jaegertracing/jaeger` | Backend: gRPC/HTTP handler, QueryService, storage interface, adapters |
| `jaegertracing/jaeger-ui` | UI: search screen, API client |

---

## Decision

### ✅ 1. Data Model — `TraceSummary`

A new message that carries everything the search results screen needs without any
individual span payloads.

**Proto (added to `jaeger-idl/proto/api_v3/query_service.proto`):**

```protobuf
// ServiceSummary contains per-service statistics for a trace, matching
// what the UI renders as a coloured tag in the search results row.
message ServiceSummary {
  // Name of the service.
  string name = 1;

  // Number of spans attributed to this service in the trace.
  int32 span_count = 2;

  // Number of spans from this service that carry OTEL StatusCode = ERROR.
  // The UI renders an error icon when this value is > 0.
  // Only spans explicitly owned by this service are counted; there is no
  // error propagation from child spans of other services.
  int32 error_span_count = 3;
}

// TraceSummary contains lightweight summary information about a trace,
// suitable for display in search result lists.
message TraceSummary {
  // Hex-encoded 128-bit trace ID.
  string trace_id = 1;

  // Name of the service that owns the root span.
  string root_service_name = 2;

  // Operation name of the root span.
  string root_operation_name = 3;

  // Start timestamp of the earliest span in the trace (Unix nanoseconds).
  // Named to match the OTLP convention (e.g. startTimeUnixNano in OTLP span JSON).
  // proto3 JSON encoding rule: fixed64/uint64/int64 fields are serialised as
  // decimal strings (not numbers) to avoid float64 precision loss in JavaScript
  // for values above 2^53.  The existing OTLP startTimeUnixNano field on Span
  // already follows this convention.
  fixed64 min_start_time_unix_nano = 4;

  // End timestamp of the latest span in the trace (Unix nanoseconds).
  // The UI may compute duration as BigInt(maxEndTimeUnixNano) - BigInt(minStartTimeUnixNano).
  fixed64 max_end_time_unix_nano = 5;

  // Total number of spans in the trace.
  int32 span_count = 6;

  // Number of spans that carry an error indicator
  // (OTEL StatusCode = ERROR).
  int32 error_span_count = 7;

  // Number of spans whose parent span ID is not present in this trace.
  // A non-zero value indicates an incomplete or partial trace.
  int32 orphan_span_count = 9;

  // Per-service breakdown, one entry per distinct service name observed
  // across all spans, sorted by name.  Matches the coloured service tags
  // shown in the search results row (name, span count, error indicator).
  repeated ServiceSummary services = 8;
}

// Request object for FindTraceSummaries.
message FindTraceSummariesRequest {
  TraceQueryParameters query = 1;
}

// Response chunk for FindTraceSummaries.  A single RPC call may yield multiple
// chunks, each carrying one or more summaries, mirroring the chunked streaming
// used by FindTraces / GetTrace.
message FindTraceSummariesResponse {
  repeated TraceSummary summaries = 1;
}
```

### ✅ 2. API v3 — New RPC

**Added to `QueryService` in `jaeger-idl/proto/api_v3/query_service.proto`:**

```protobuf
service QueryService {
  // ... existing RPCs ...

  // FindTraceSummaries searches for traces matching the given query and streams
  // back lightweight summary information for each matching trace.  Each response
  // chunk may contain one or more summaries.  Use this instead of FindTraces when
  // full span data is not required (e.g. search results page).
  rpc FindTraceSummaries(FindTraceSummariesRequest) returns (stream FindTraceSummariesResponse) {
    option (google.api.http) = {
      get: "/api/v3/trace-summaries"
      additional_bindings {
        post: "/api/v3/trace-summaries"
        body: "*"
      }
    };
  }
}
```

`FindTraceSummariesRequest` embeds the same `TraceQueryParameters` inner type as
`FindTracesRequest`, so no new query-parameter parsing is needed.

### ✅ 3. Storage v2 Remote API — Optional RPC

**Added to `TraceReader` in `jaeger-idl/proto/storage/v2/trace_storage.proto`:**

```protobuf
// ServiceSummary and TraceSummary mirror the definitions in api_v3 but live
// in the storage package to avoid a cross-package proto dependency.
message ServiceSummary {
  string name             = 1;
  int32  span_count       = 2;
  int32  error_span_count = 3;
}

message TraceSummary {
  bytes  trace_id            = 1;  // 16-byte binary trace ID
  string root_service_name   = 2;
  string root_operation_name = 3;
  fixed64 min_start_time_unix_nano = 4;  // Unix nanoseconds; 0 if unknown
  fixed64 max_end_time_unix_nano   = 5;  // Unix nanoseconds; 0 if unknown
  int32   span_count               = 6;
  int32   error_span_count         = 7;
  repeated ServiceSummary services = 8;
  int32   orphan_span_count        = 9;
}

message FindTraceSummariesRequest {
  TraceQueryParameters query = 1;
}

// Response chunk for FindTraceSummaries.  Mirrors the chunked streaming
// contract of GetTraces / FindTraces: each chunk carries one or more summaries.
message FindTraceSummariesResponse {
  repeated TraceSummary summaries = 1;
}

service TraceReader {
  // ... existing RPCs ...

  // FindTraceSummaries is an optional streaming RPC. If a remote storage backend
  // does not implement it, it MUST return gRPC status UNIMPLEMENTED so that the
  // caller can fall back to FindTraces + client-side aggregation.
  rpc FindTraceSummaries(FindTraceSummariesRequest) returns (stream FindTraceSummariesResponse) {}
}
```

Marking the method "optional by convention" (return `UNIMPLEMENTED`) keeps backward
compatibility with existing remote storage plugins: they continue to compile because the
auto-generated Go server interface provides a default `UnimplementedTraceReaderServer`
embedding that already returns `UNIMPLEMENTED` for any un-overridden method.

### ✅ 4. Go `tracestore.Reader` Interface — Optional Extension Interface

Rather than adding a method directly to `tracestore.Reader` (which would break all
existing storage implementations), a new **optional** interface is introduced:

```go
// SummaryReader is an optional extension to tracestore.Reader that allows
// storage backends to compute trace summaries natively.  Backends that do not
// implement this interface will fall back to FindTraces + client-side aggregation.
//
// The iterator contract mirrors FindTraces: each yielded batch may contain one
// or more summaries, and implementations may yield results incrementally as the
// underlying query executes rather than buffering all results first.
// Implementations that do not support the operation should yield
// errors.ErrUnsupported (wrapped with %w) as the first error.
type SummaryReader interface {
    FindTraceSummaries(ctx context.Context, query TraceQueryParams) iter.Seq2[[]TraceSummary, error]
}

// ServiceSummary holds per-service statistics for a single trace.
type ServiceSummary struct {
    Name           string
    SpanCount      int
    ErrorSpanCount int
}

// TraceSummary mirrors TraceSummary in jaeger.api_v3 but uses Go types.
type TraceSummary struct {
    TraceID pcommon.TraceID
    // RootServiceName is the service name of the root span — the span with no
    // parent span ID. If multiple such spans exist, the one with the earliest
    // start timestamp is chosen.
    RootServiceName   string
    RootOperationName string
    // MinStartTime is the start timestamp of the earliest span in the trace.
    MinStartTime time.Time
    // MaxEndTime is the maximum end timestamp across all spans in the trace.
    // Duration can be derived as MaxEndTime - MinStartTime.
    MaxEndTime     time.Time
    SpanCount      int
    ErrorSpanCount int
    // OrphanSpanCount is the number of spans whose parent span ID is not
    // present in this trace (indicates an incomplete / partial trace).
    OrphanSpanCount int
    // Services contains one entry per distinct service name observed across all
    // spans, including the root service. Entries are sorted by name.
    Services []ServiceSummary
}
```

This follows the existing pattern in Jaeger where optional storage capabilities are
expressed as separate interfaces (e.g. `spanstore.Writer` vs `spanstore.WriteFlags`).

### ✅ 5. QueryService — Fallback Logic

`querysvc.QueryService` gains a new method:

```go
func (qs *QueryService) FindTraceSummaries(
    ctx context.Context,
    query tracestore.TraceQueryParams,
) iter.Seq2[[]tracestore.TraceSummary, error]
```

The return type is an iterator, consistent with `FindTraces` and `FindTraceIDs`, allowing
summaries to be streamed incrementally to the caller rather than buffered in memory.

`SummaryReader.FindTraceSummaries` signals "not supported" by yielding
`errors.ErrUnsupported` (Go 1.21 standard sentinel, wrapped with `%w`) as the **first
iterator error**. The signature mirrors `FindTraces` — a plain iterator, no top-level
error. `QueryService.FindTraceSummaries` wraps the iterator and, on first `ErrUnsupported`,
falls back transparently to `FindTraces` + `computeSummaries`.

Using `errors.ErrUnsupported` rather than a Jaeger-specific sentinel keeps the interface
clean: any `SummaryReader` implementation can signal "not available" without importing
internal packages.

**Implementation logic:**

```go
// In QueryService.FindTraceSummaries (simplified):
if sr, ok := qs.traceReader.(tracestore.SummaryReader); ok {
    return func(yield func([]tracestore.TraceSummary, error) bool) {
        for batch, err := range sr.FindTraceSummaries(ctx, query) {
            if errors.Is(err, errors.ErrUnsupported) {
                // fall through to computeSummaries
                for b, e := range computeSummaries(qs.traceReader.FindTraces(ctx, query), qs.adjuster) {
                    if !yield(b, e) { return }
                }
                return
            }
            if err != nil { yield(nil, err); return }
            if !yield(batch, nil) { return }
        }
    }
}
// fallback: aggregate full traces into summaries via jptrace.AggregateTraces,
// applying the clock-skew adjuster before summarizing each assembled trace
return computeSummaries(qs.traceReader.FindTraces(ctx, query), qs.adjuster)
```

`computeSummaries` uses `jptrace.AggregateTraces` to reassemble multi-chunk traces
before computing each summary, ensuring a trace split across consecutive `ptrace.Traces`
chunks always produces exactly one `TraceSummary`. The summary records `MinStartTime`
and `MaxEndTime` as raw `time.Time` values; duration is intentionally omitted and left
for callers to derive.

### ✅ 6. Remote Storage Adapter — Fallback on UNIMPLEMENTED

The gRPC-based remote storage adapter wraps the remote `TraceReader` gRPC client. Its
`FindTraceSummaries` implementation is a plain iterator (no top-level error) that calls
the remote RPC and yields `errors.ErrUnsupported` when the server returns
`codes.Unimplemented`. Note: for server-streaming RPCs in gRPC-Go, the server's
RPC-level error (including `codes.Unimplemented`) is delivered via the first `Recv()`
call, not the initial stream-open call; the iterator handles this transparently.
`QueryService` detects `ErrUnsupported` from the first iterator yield and falls back to
`FindTraces` + `computeSummaries`. The feature therefore works transparently with
existing remote storage plugins that have not yet implemented the new RPC.

### ✅ 7. gRPC and HTTP Handlers

**gRPC handler** (`apiv3/grpc_handler.go`) streams response chunks back to the client:

```go
func (h *Handler) FindTraceSummaries(
    req *api_v3.FindTraceSummariesRequest,
    stream api_v3.QueryService_FindTraceSummariesServer,
) error {
    params := convert(req.Query)
    h.queryService.FindTraceSummaries(stream.Context(), params)(
        func(batch []tracestore.TraceSummary, err error) bool {
            if err != nil { /* handle */ return false }
            return stream.Send(&api_v3.FindTraceSummariesResponse{
                Summaries: convert(batch),
            }) == nil
        },
    )
    return nil
}
```

**HTTP gateway** (`apiv3/http_gateway.go`): registers `GET /api/v3/trace-summaries`
using the same query-parameter parsing as `FindTraces`. The HTTP handler collects the
full iterator via `jiter.FlattenWithErrors` before writing the JSON response (HTTP/1.1
does not support true streaming for this use case; HTTP/2 streaming can be added later
if needed).

### ✅ 8. Jaeger UI Changes

**API client** (`jaeger-ui/packages/jaeger-ui/src/api/jaeger.ts`):

```typescript
findTraceSummaries(query: Record<string, any>): Promise<FindTraceSummariesResponse> {
  return getJSON(`${this.apiRoot}v3/trace-summaries`, { query });
}
```

The search screen (`src/components/SearchTracePage/`) is updated to call
`findTraceSummaries` instead of `searchTraces` and bind the result directly to the
`TraceSummary` shape, eliminating the client-side aggregation step.

New TypeScript types are introduced:

```typescript
export type ServiceSummary = {
  name: string;
  spanCount: number;
  errorSpanCount: number;
};

export type TraceSummary = {
  traceId: string;
  // rootServiceName is the service of the span with no parent (earliest start
  // time wins when multiple root candidates exist).
  rootServiceName: string;
  rootOperationName: string;
  // Unix nanoseconds encoded as decimal strings (per proto3 JSON convention);
  // use BigInt() to do arithmetic. Consistent with OTLP startTimeUnixNano /
  // endTimeUnixNano on Span which are also string-encoded in the JSON wire format.
  // Empty string when unknown.
  minStartTimeUnixNano: string;
  maxEndTimeUnixNano: string;
  spanCount: number;
  errorSpanCount: number;
  // Number of spans whose parent span ID is not present in this trace.
  orphanSpanCount: number;
  // One entry per distinct service, sorted by name, matching the coloured
  // tags in the search results row (name, span count, error indicator).
  services: ServiceSummary[];
};
```

#### Proto → OpenAPI → Zod pipeline for timestamp fields (Milestone 3)

When the proto is formalized in Milestone 3, the toolchain propagates the string
encoding automatically with no special handling:

1. **Proto (`fixed64`)** — gnostic `protoc-gen-openapi` maps `fixed64` (and `uint64`/`int64`)
   to `type: string` in the generated OpenAPI YAML, following the proto3 JSON mapping spec.
   This is the same mapping that already exists for `startTimeUnixNano`/`endTimeUnixNano`
   on the OTLP `Span` type in the current `query_service.openapi.yaml`.
2. **OpenAPI (`type: string`)** → `openapi-zod-client` generates `z.string()`.
   The existing `generated-client.ts` already contains `startTimeUnixNano: z.string()` and
   `endTimeUnixNano: z.string()` for OTLP spans, confirming the pipeline is correct.
3. **UI code** — schema validation and type inference automatically treat the fields as
   strings; arithmetic uses `BigInt(minStartTimeUnixNano)`.

As of Milestone 3, the HTTP handler uses `gogoproto/jsonpb` marshalling of `api_v3.FindTraceSummariesResponse`, which encodes `fixed64` fields as decimal strings automatically — exactly replicating what the manual `strconv.FormatInt(t.UnixNano(), 10)` encoding did before.

#### Validation gap: `z.string()` does not enforce numeric content

`z.string()` accepts any string, including non-numeric values like `"abc"` or `""`.
The application code that parses the fields (e.g. `BigInt(minStartTimeUnixNano)` for
duration, or truncation to microseconds) will throw at runtime on bad input, but that
is after schema validation has already passed.

**Fix for Milestone 3:** add a `pattern` constraint to the proto field via the gnostic
OpenAPI annotation:

```protobuf
import "openapiv3/annotations.proto";

fixed64 min_start_time_unix_nano = 4 [
  (openapi.v3.property) = {pattern: "^[0-9]+$"},
  ...
];
```

This flows through the pipeline as:

```yaml
# generated OpenAPI
min_start_time_unix_nano:
  type: string
  pattern: "^[0-9]+$"
```

```typescript
// generated Zod
minStartTimeUnixNano: z.string().regex(/^[0-9]+$/)
```

The same annotation should be applied to `max_end_time_unix_nano`.

During Milestone 2 (hand-written TypeScript types), the validation gap exists but is
benign in practice: the server always emits well-formed numeric strings via
`strconv.FormatInt`, so a conforming backend never produces invalid values. The gap
matters only if a non-conforming or mocked backend is used in tests.

---

### ✅ 9. Integration Tests

The existing `TestJaegerQueryService` integration test (`cmd/jaeger/internal/integration/query_test.go`)
runs two Jaeger instances connected over gRPC remote storage. It exercises the full stack but
did not previously cover the `FindTraceSummaries` endpoint.

**End-to-end test coverage added:**

- `traceReader` in `cmd/jaeger/internal/integration/trace_reader.go` implements
  `tracestore.SummaryReader` by calling the `api_v3.QueryService.FindTraceSummaries` gRPC RPC
  and converting `api_v3.TraceSummary` proto messages to `tracestore.TraceSummary`.
- `StorageIntegration` in `internal/storage/integration/integration.go` always runs a
  `FindTraceSummaries` sub-test via `RunSpanStoreTests`. The test casts `TraceReader` to
  `tracestore.SummaryReader` and fails loudly if the cast does not succeed. Storage backends
  that do not yet implement `SummaryReader` opt out by adding `"FindTraceSummaries"` to their
  `Capabilities.SkipList`. The sub-test:
  1. Writes the `example_trace` fixture via the trace writer.
  2. Queries summaries with a time window covering the trace.
  3. Asserts the returned summary matches the expected trace ID, span count, and non-zero timestamps.
- `traceReader` already implements both `tracestore.Reader` and `tracestore.SummaryReader`,
  so the e2e integration test gains `FindTraceSummaries` coverage automatically — no extra
  field wiring or separate binary needed.

This exercises the complete path:
`HTTP/gRPC handler → QueryService (fallback aggregation) → gRPC remote storage reader → memory backend`

---

## Alternatives Considered

### A. Add query parameter `summary=true` to `FindTraces`

Return a stripped-down representation when `summary=true` is passed.

**Pros:** No new endpoint; minimal proto change.

**Cons:** The response type is `stream TracesData`, which is OTEL spans — not a natural
home for summary-only fields. Callers cannot differentiate by type system alone; would
require a runtime switch inside response parsing. Harder to version independently.

### B. Compute summaries in the UI from the existing full-trace response

Status quo. No backend changes.

**Pros:** Zero implementation cost.

**Cons:** The fundamental performance problem is not addressed. The network and memory
pressure grow linearly with span count per matching trace.

### C. Extend `FindTraceIDs` to return metadata alongside IDs

Return a richer `FoundTraceID` from `FindTraceIDs` that includes summary fields.

**Pros:** Reuses an existing method.

**Cons:** `FindTraceIDs` is semantically meant for ID-only lookups; bundling display
metadata into it is conceptually awkward and would confuse consumers of that API that
genuinely want only IDs. Adding optional fields to `FoundTraceID` creates ambiguity
about which calls populate them.

### D. Add `FindTraceSummaries` directly to `tracestore.Reader`

Require all storage implementations to implement the method (with a default implementation
in a base struct).

**Pros:** Uniform interface.

**Cons:** Breaks all existing storage implementations and any third-party plugins. The
optional-interface approach (chosen) is the established Jaeger pattern and is less
disruptive.

---

## Implementation Milestones

Each milestone is independently shippable and provides a concrete improvement over the
previous state. Changes to `jaeger-idl` are deferred until the design has been validated
end-to-end in `jaeger/` and `jaeger-ui/`.

---

### Milestone 1 — Working backend endpoint with fallback aggregation (`jaeger/` only)

> **Status: ✅ Complete**
>
> - [jaegertracing/jaeger#8604](https://github.com/jaegertracing/jaeger/pull/8604) — main implementation
> - [jaegertracing/jaeger#8618](https://github.com/jaegertracing/jaeger/pull/8618) — rename `query.num_traces` → `query.search_depth`
> - [jaegertracing/jaeger#8633](https://github.com/jaegertracing/jaeger/pull/8633) — fix `traceId` JSON field name casing

**Goal:** Ship a functional `GET /api/v3/trace-summaries` HTTP endpoint backed entirely
by the fallback path (load full traces, compute summaries server-side). No changes to
`jaeger-idl` or `jaeger-ui`. This validates the data model, the aggregation logic, and
the HTTP contract before touching other repositories.

**Delivered:**
1. `tracestore.ServiceSummary`, `tracestore.TraceSummary`, and the optional `tracestore.SummaryReader` interface (`internal/storage/v2/api/tracestore/summary.go`).
2. `computeSummaries` fallback aggregation in `querysvc/summary.go`, using `jptrace.AggregateTraces` to reassemble multi-chunk traces before summarizing.
3. `querysvc.QueryService.FindTraceSummaries` with both the `SummaryReader` native path and the fallback path. `SummaryReader` discovery is a direct `ok`-guarded type assertion on `qs.traceReader`; `ReadMetricsDecorator` surfaces `SummaryReader` directly when the wrapped reader implements it (see §5 below). If the `SummaryReader` yields `errors.ErrUnsupported`, `QueryService` falls back transparently to `computeSummaries` (see §6).
4. `GET /api/v3/trace-summaries` in the HTTP gateway, reusing `parseFindTracesQuery`. Response is plain JSON; timestamps encoded as decimal strings per proto3 JSON convention.
5. `query.search_depth` is the canonical query parameter (matching the proto field); `query.num_traces` is accepted as a deprecated alias (jaegertracing/jaeger#8617). Defaults to 100 when omitted.
6. Unit tests for `computeSummaries` (empty, error, multi-service, multi-chunk, orphan spans), `FindTraceSummaries` (fallback path, native `SummaryReader`, `ErrUnsupported` fallback), HTTP handler (success, storage error, deprecated alias).
7. Integration test: `FindTraceSummaries` added to `RunSpanStoreTests`, exercised end-to-end via `TestJaegerQueryService` (see §9).

---

### Milestone 2 — UI migration to the new endpoint (`jaeger-ui/` only)

> **Status: ✅ Complete**
>
> - [jaegertracing/jaeger-ui#3941](https://github.com/jaegertracing/jaeger-ui/pull/3941) — introduce `TraceSummary` type
> - [jaegertracing/jaeger-ui#3943](https://github.com/jaegertracing/jaeger-ui/pull/3943) — migrate search to `/api/v3/trace-summaries` (phase 2b)
> - [jaegertracing/jaeger-ui#3947](https://github.com/jaegertracing/jaeger-ui/pull/3947) — v3 trace-summaries API client and sort model
> - [jaegertracing/jaeger-ui#3964](https://github.com/jaegertracing/jaeger-ui/pull/3964) — use `/api/v3/trace-summaries` for search results
> - [jaegertracing/jaeger-ui#3966](https://github.com/jaegertracing/jaeger-ui/pull/3966) — complete phase 2c discovery query keys

**Goal:** The search screen calls `GET /api/v3/trace-summaries` instead of
`GET /api/traces`, delivering the network-size reduction to users and validating that
the `TraceSummary` shape is complete and correct for all search-results rendering.

**Delivered:**
1. `ServiceSummary` and `TraceSummary` types in `src/types/trace-summary.ts`; the internal `TraceSummary` uses `traceID` (uppercase D) and `startTime`/`duration` in microseconds to match the legacy `ITrace`-based rendering code.
2. `fetchTraceSummaries` in `src/api/v3/client.ts` calls `GET /api/v3/trace-summaries` with camelCase query parameters (`query.search_depth`, etc.) and maps the wire response (nanosecond strings, `traceId`) to the internal type. Zod schemas in `src/api/v3/schemas.ts` add format constraints (hex regex for `traceId`, decimal-string pattern for timestamp fields).
3. `useSearchTraces` React Query hook in `src/hooks/useTraceDiscovery.ts` replaces the Redux `searchTraces` action for the search results path. The search page (`SearchTracePage`) uses this hook directly.
4. `transformTraceData` aggregation is no longer called on the search path; it is still used on the trace detail page.

**Deviation from plan:** No `searchTraces` v1 fallback was implemented. The UI unconditionally calls the v3 endpoint. Deployments using a Jaeger backend older than Milestone 1 will see search fail rather than fall back gracefully. This was accepted as a trade-off given the controlled rollout.

---

### Milestone 3 — Formalise the API in `jaeger-idl`

> **Status: ✅ Complete**
>
> IDL commits on `jaeger-idl` main:
> - [jaeger-idl#203](https://github.com/jaegertracing/jaeger-idl/pull/203) (`8c84d89`) — Add `FindTraceSummaries` RPC to `api_v3` and `storage/v2`
> - [jaeger-idl#200](https://github.com/jaegertracing/jaeger-idl/pull/200) (`c4f36ba`) — Give `FindTraceIDs` its own request type in `storage/v2`
> - [jaeger-idl#202](https://github.com/jaegertracing/jaeger-idl/pull/202) (`2543795`) — Fix JSON naming in OpenAPI spec
> - [jaeger-idl#204](https://github.com/jaegertracing/jaeger-idl/pull/204) (`0daa719`) — Mark `trace_id` and `ServiceSummary.name` as REQUIRED

**Goal:** Promote the endpoint from an internal HTTP-only contract to a first-class
gRPC RPC defined in the IDL, now that the data model has been validated by real UI
usage. This also makes the endpoint accessible to gRPC clients and code-generated SDKs.

**Changes:**
1. ~~**`jaeger-idl`**: Add `ServiceSummary`, `TraceSummary`, `FindTraceSummariesRequest`,
   `FindTraceSummariesResponse`, and the `FindTraceSummaries` RPC to `api_v3/query_service.proto`.
   Also introduce a dedicated `FindTraceIDsRequest` type in `storage/v2/trace_storage.proto`.~~ ✅ Already done in `jaeger-idl` main — see commits above.
2. ✅ **`jaeger`**: Bump the `idl/` submodule to latest `jaeger-idl` main (`0daa719`). Regenerate Go bindings. Implement the gRPC handler method (`apiv3/grpc_handler.go`). ([#8634](https://github.com/jaegertracing/jaeger/pull/8634))
3. ✅ **`jaeger`**: Replace hand-written JSON scaffold types in the HTTP gateway with `api_v3.FindTraceSummariesResponse` + `gogoproto/jsonpb` marshalling ([#8645](https://github.com/jaegertracing/jaeger/pull/8645)). The gRPC-gateway approach was ruled out: it only supports OpenAPI v2, is a heavyweight dependency, and does not work with the `gogoproto` custom marshallers used throughout the project. Instead, the existing `marshalResponse`/`jsonpb` path is used — `jsonpb` encodes `fixed64` fields as decimal strings, matching the proto3 JSON spec and the OTLP convention, so no behaviour change occurs at the wire level.

**Success criteria:**
- Proto files pass `buf lint` and `buf breaking` against the previous IDL version.
- gRPC call via `grpcurl` returns the same payload as the HTTP endpoint.
- `make test` and `make lint` pass.
- OpenAPI spec regenerated; documentation updated.

---

### Milestone 4 — Remote Storage gRPC adapter with fallback (`jaeger-idl` + `jaeger/`)

> **Status: ✅ Complete**

**Goal:** Remote storage backends can optionally implement native summary computation.
The adapter falls back transparently when they do not, so existing plugins require no
changes.

**Delivered:**
1. ~~**`jaeger-idl`**: Add `ServiceSummary`, `TraceSummary`, `FindTraceSummariesRequest`,
   `FindTraceSummariesResponse`, and the optional `FindTraceSummaries` RPC to `storage/v2/trace_storage.proto`.~~ ✅ Already done in `jaeger-idl` main (same PR #203).
2. ✅ **`jaeger`**: `Handler.FindTraceSummaries` in the gRPC storage server (`internal/storage/v2/grpc/handler.go`) forwards to the underlying `tracestore.SummaryReader` if available, otherwise returns `codes.Unimplemented`.
3. ✅ **`jaeger`**: `TraceReader.FindTraceSummaries` in the gRPC storage client (`internal/storage/v2/grpc/tracereader.go`) implements `tracestore.SummaryReader` as a plain iterator (matching the `FindTraces` signature). `codes.Unimplemented` from the server (delivered via the first `Recv()`) is yielded as `errors.ErrUnsupported`; `QueryService` detects it and falls back to `computeSummaries` automatically.
4. ✅ Storage backends that don't implement `SummaryReader` opt out via `Capabilities.SkipList` — the `FindTraceSummaries` integration test is only run for backends that implement it (currently the e2e `traceReader` in `cmd/jaeger/internal/integration/`).

---

### Milestone 5 — Native summary support in one storage backend

> **Status: ⏳ Pending** (depends on Milestone 4)

**Goal:** Demonstrate the full performance benefit of the `SummaryReader` interface with
a native implementation in one backend, serving as a reference for other backends.

**Candidate:** Elasticsearch or ClickHouse (whichever can express the aggregation most
naturally as a single query).

**Changes (`jaeger/` only):**
1. Implement `SummaryReader.FindTraceSummaries` for the chosen backend using a native
   aggregation query (e.g. an ES `terms` + `top_hits` aggregation).
2. Benchmark: native vs. fallback path on a dataset of 1 000 traces with ≥ 100 spans.

**Success criteria:**
- Native implementation passes the same golden tests used for the fallback.
- Benchmark shows ≥ 50% reduction in backend CPU time and/or bytes read from storage
  compared to the fallback path.

---

## Remaining Work — Suggested PR Sequence

A concise breakdown for contributors picking up Milestones 3–5. Each PR is
independently reviewable and leaves `main` in a working state.

| # | Repo | Description | Notes |
|---|------|-------------|-------|
| ✅ A | `jaeger/` | Bump `idl/` submodule to `jaeger-idl` main (`0daa719`); regenerate Go bindings; fix any compilation errors from the renamed `FindTraceIDsRequest` | [#8634](https://github.com/jaegertracing/jaeger/pull/8634) |
| ✅ B | `jaeger/` | Implement the gRPC handler for `FindTraceSummaries` (`apiv3/grpc_handler.go`) | [#8634](https://github.com/jaegertracing/jaeger/pull/8634) |
| ✅ C | `jaeger/` | Replace hand-written JSON scaffold types in the HTTP gateway with `api_v3.FindTraceSummariesResponse` + `gogoproto/jsonpb`; delete `summaries.go` | [#8645](https://github.com/jaegertracing/jaeger/pull/8645) |
| ✅ D | `jaeger/` | Implement `SummaryReader` in the gRPC remote storage adapter (`internal/storage/v2/grpc/`) — server forwards to underlying `SummaryReader`; client is a plain iterator that yields `errors.ErrUnsupported` when the server returns `UNIMPLEMENTED` | Milestone 4 |
| G | `jaeger/` | Native `SummaryReader` in one storage backend (Elasticsearch or ClickHouse) | Milestone 5, optional |

---

## Consequences

### Positive

* The search results page transfers only the data it actually needs, reducing latency
  and browser memory usage proportionally to trace size.
* Storage backends can optionally provide fast, native aggregation (e.g. a single
  Elasticsearch `terms` aggregation instead of fetching all span documents).
* The existing `FindTraces` endpoint is unchanged; this is a purely additive change
  with no breaking impact.
* The optional-interface pattern keeps all current storage implementations compiling
  without modification.

### Negative

* New proto types must be maintained alongside the existing `TracesData`-based
  responses.
* The fallback path re-introduces the same per-trace span loading that this ADR is
  designed to avoid; backends that do not implement `SummaryReader` see no performance
  improvement until they do.
* The UI will need a compatibility shim or feature-detection path if it is deployed
  against an older Jaeger backend that does not serve `/api/v3/trace-summaries` (return
  404 → fall back to `/api/v3/traces`).

---

## References

* `idl/proto/api_v3/query_service.proto` – existing v3 query service proto
* `idl/proto/storage/v2/trace_storage.proto` – remote storage gRPC proto
* `internal/storage/v2/api/tracestore/reader.go` – Go `Reader` interface
* `cmd/jaeger/internal/extension/jaegerquery/querysvc/service.go` – QueryService
* `cmd/jaeger/internal/extension/jaegerquery/internal/apiv3/` – gRPC and HTTP v3 handlers
* `jaeger-ui/packages/jaeger-ui/src/api/jaeger.ts` – UI API client
