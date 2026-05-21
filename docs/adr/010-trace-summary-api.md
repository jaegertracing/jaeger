# ADR-010: Trace Summary API for Lightweight Search Results

* **Status**: Proposed
* **Date**: 2026-05-21

## Context

The Jaeger UI search screen calls `GET /api/traces` (or `/api/v3/traces`) to display a
list of matching traces. The response today contains **full trace data** — every span with
all its attributes. The UI then post-processes that data locally to render a compact result
row for each trace:

* Root service name and operation name
* Trace duration (max end time − min start time)
* Span count
* Error count / error indicator
* Distinct service names

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

### 1. Data Model — `TraceSummary`

A new message that carries everything the search results screen needs without any
individual span payloads.

**Proto (added to `jaeger-idl/proto/api_v3/query_service.proto`):**

```protobuf
// TraceSummary contains lightweight summary information about a trace,
// suitable for display in search result lists.
message TraceSummary {
  // Hex-encoded 128-bit trace ID.
  string trace_id = 1;

  // Name of the service that owns the root span.
  string root_service_name = 2;

  // Operation name of the root span.
  string root_operation_name = 3;

  // Start time of the earliest span in the trace.
  google.protobuf.Timestamp start_time = 4;

  // End-to-end duration: latest span end time minus earliest span start time.
  google.protobuf.Duration duration = 5;

  // Total number of spans in the trace.
  int32 span_count = 6;

  // Number of spans that carry an error indicator
  // (OTEL status code = ERROR, or a "error"=true attribute).
  int32 error_count = 7;

  // Distinct service names observed across all spans.
  repeated string service_names = 8;
}

// Response for FindTraceSummaries.
message FindTraceSummariesResponse {
  repeated TraceSummary summaries = 1;
}
```

### 2. API v3 — New RPC

**Added to `QueryService` in `jaeger-idl/proto/api_v3/query_service.proto`:**

```protobuf
service QueryService {
  // ... existing RPCs ...

  // FindTraceSummaries searches for traces matching the given query and returns
  // lightweight summary information for each matching trace.  Use this instead
  // of FindTraces when full span data is not required (e.g. search results page).
  rpc FindTraceSummaries(FindTracesRequest) returns (FindTraceSummariesResponse) {
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

The request reuses the existing `FindTracesRequest` / `TraceQueryParameters` types so
no new query-parameter parsing is needed.

### 3. Storage v2 Remote API — Optional RPC

**Added to `TraceReader` in `jaeger-idl/proto/storage/v2/trace_storage.proto`:**

```protobuf
// TraceSummary mirrors the definition in api_v3 but is in the storage package
// to avoid a cross-package dependency in the proto graph.
message TraceSummary {
  bytes  trace_id            = 1;  // 16-byte binary trace ID
  string root_service_name   = 2;
  string root_operation_name = 3;
  google.protobuf.Timestamp start_time = 4;
  google.protobuf.Duration  duration   = 5;
  int32  span_count          = 6;
  int32  error_count         = 7;
  repeated string service_names = 8;
}

message FindTraceSummariesResponse {
  repeated TraceSummary summaries = 1;
}

service TraceReader {
  // ... existing RPCs ...

  // FindTraceSummaries is an optional RPC. If a remote storage backend does
  // not implement it, it MUST return gRPC status UNIMPLEMENTED so that the
  // caller can fall back to FindTraces + client-side aggregation.
  rpc FindTraceSummaries(FindTracesRequest) returns (FindTraceSummariesResponse) {}
}
```

Marking the method "optional by convention" (return `UNIMPLEMENTED`) keeps backward
compatibility with existing remote storage plugins: they continue to compile because the
auto-generated Go server interface provides a default `UnimplementedTraceReaderServer`
embedding that already returns `UNIMPLEMENTED` for any un-overridden method.

### 4. Go `tracestore.Reader` Interface — Optional Extension Interface

Rather than adding a method directly to `tracestore.Reader` (which would break all
existing storage implementations), a new **optional** interface is introduced:

```go
// SummaryReader is an optional extension to tracestore.Reader that allows
// storage backends to compute trace summaries natively.  Backends that do not
// implement this interface will fall back to FindTraces + client-side aggregation.
type SummaryReader interface {
    FindTraceSummaries(ctx context.Context, query TraceQueryParams) ([]TraceSummary, error)
}

// TraceSummary mirrors TraceSummary in jaeger.api_v3 but uses Go types.
type TraceSummary struct {
    TraceID           pcommon.TraceID
    RootServiceName   string
    RootOperationName string
    StartTime         time.Time
    Duration          time.Duration
    SpanCount         int
    ErrorCount        int
    ServiceNames      []string
}
```

This follows the existing pattern in Jaeger where optional storage capabilities are
expressed as separate interfaces (e.g. `spanstore.Writer` vs `spanstore.WriteFlags`).

### 5. QueryService — Fallback Logic

`querysvc.QueryService` gains a new method:

```go
func (qs *QueryService) FindTraceSummaries(
    ctx context.Context,
    query tracestore.TraceQueryParams,
) ([]tracestore.TraceSummary, error)
```

**Implementation logic:**

```
if reader implements tracestore.SummaryReader:
    return reader.FindTraceSummaries(ctx, query)
else:
    traces = collect all from reader.FindTraces(ctx, query)
    return computeSummaries(traces)   // client-side aggregation
```

`computeSummaries` iterates the `ptrace.Traces` iterator and accumulates the required
statistics without retaining the full span data.

### 6. Remote Storage Adapter — Fallback on UNIMPLEMENTED

The gRPC-based remote storage adapter (`plugin/storage/grpc/`) wraps the remote
`TraceReader` gRPC client. Its `FindTraceSummaries` implementation calls the remote RPC
and, if the server returns `codes.Unimplemented`, falls back to calling
`FindTraces` and computing summaries client-side:

```go
func (r *grpcReader) FindTraceSummaries(ctx, query) ([]TraceSummary, error) {
    resp, err := r.client.FindTraceSummaries(ctx, req)
    if status.Code(err) == codes.Unimplemented {
        return r.findTraceSummariesFallback(ctx, query)
    }
    return convert(resp.Summaries), err
}
```

### 7. gRPC and HTTP Handlers

**gRPC handler** (`apiv3/grpc_handler.go`):

```go
func (h *Handler) FindTraceSummaries(
    ctx context.Context,
    req *api_v3.FindTracesRequest,
) (*api_v3.FindTraceSummariesResponse, error) {
    params := convert(req.Query)
    summaries, err := h.queryService.FindTraceSummaries(ctx, params)
    // convert []tracestore.TraceSummary → []api_v3.TraceSummary
    ...
}
```

**HTTP gateway** (`apiv3/http_gateway.go`): registers `GET /api/v3/trace-summaries`
via the existing grpc-gateway mechanism, using the same query-parameter parsing as
`FindTraces`.

### 8. Jaeger UI Changes

**API client** (`jaeger-ui/packages/jaeger-ui/src/api/jaeger.ts`):

```typescript
findTraceSummaries(query: Record<string, any>): Promise<FindTraceSummariesResponse> {
  return getJSON(`${this.apiRoot}v3/trace-summaries`, { query });
}
```

The search screen (`src/components/SearchTracePage/`) is updated to call
`findTraceSummaries` instead of `searchTraces` and bind the result directly to the
`TraceSummary` shape, eliminating the client-side aggregation step.

A `TraceSummary` TypeScript type is introduced:

```typescript
export type TraceSummary = {
  traceID: string;
  rootServiceName: string;
  rootOperationName: string;
  startTime: number;       // Unix microseconds
  duration: number;        // microseconds
  spanCount: number;
  errorCount: number;
  serviceNames: string[];
};
```

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

### Milestone 1 — Data model and proto definitions

**Goal:** All three repositories have the new proto types and generated code; nothing is
wired up yet.

**Changes:**
1. **`jaeger-idl`**: Add `TraceSummary`, `FindTraceSummariesResponse`, and the
   `FindTraceSummaries` RPC to both `api_v3/query_service.proto` and
   `storage/v2/trace_storage.proto`. Bump the IDL version.
2. **`jaeger`**: Regenerate protobuf Go bindings. Add `tracestore.SummaryReader` and
   `tracestore.TraceSummary` to `internal/storage/v2/api/tracestore/`.
3. **`jaeger-ui`**: Add the `TraceSummary` TypeScript type to `src/types/`.

**Success criteria:**
- Proto files pass `buf lint` and `buf breaking` checks against the previous version.
- Generated Go and TypeScript types compile cleanly.
- No existing tests are broken.
- PR review approved in all three repos before proceeding.

---

### Milestone 2 — QueryService + fallback implementation

**Goal:** The backend can serve `FindTraceSummaries` for any storage backend, using
fallback aggregation when the backend does not provide native support.

**Changes:**
1. **`jaeger`**: Implement `querysvc.QueryService.FindTraceSummaries` with the
   fallback logic (type-assert to `SummaryReader`; if absent, call `FindTraces` and
   compute summaries via `computeSummaries`).
2. **`jaeger`**: Add unit tests for `computeSummaries` with representative trace
   fixtures (single-span trace, multi-service trace, traces with errors, empty result).
3. **`jaeger`**: Add unit tests for `QueryService.FindTraceSummaries` covering both the
   native-summary path (mock `SummaryReader`) and the fallback path (mock
   `tracestore.Reader` that does not implement `SummaryReader`).

**Success criteria:**
- `make test` passes.
- `FindTraceSummaries` with fallback returns results identical to what the UI currently
  computes from `FindTraces` output, verified by a table-driven golden test that
  compares summaries computed from full traces vs. summaries returned by the fallback.

---

### Milestone 3 — gRPC and HTTP API wiring

**Goal:** The `FindTraceSummaries` endpoint is reachable via both gRPC and HTTP and
returns correct results end to end.

**Changes:**
1. **`jaeger`**: Implement `FindTraceSummaries` in the gRPC handler
   (`apiv3/grpc_handler.go`).
2. **`jaeger`**: Register `GET /api/v3/trace-summaries` and
   `POST /api/v3/trace-summaries` in the HTTP gateway
   (`apiv3/http_gateway.go`), reusing the existing query-parameter parser.
3. **`jaeger`**: Add handler-level tests verifying query-parameter parsing and
   response serialisation.

**Success criteria:**
- `make test` and `make lint` pass.
- Manual smoke test with `curl` against a running Jaeger-all-in-one confirms the
  endpoint returns the expected JSON structure.
- OpenAPI spec regenerated and updated documentation reflects the new endpoint.

---

### Milestone 4 — Remote Storage gRPC adapter

**Goal:** The remote storage (gRPC plugin) adapter calls `FindTraceSummaries` on the
remote backend and gracefully falls back to the client-side aggregation when the remote
server returns `UNIMPLEMENTED`.

**Changes:**
1. **`jaeger`**: Implement `FindTraceSummaries` in the gRPC storage reader
   (`plugin/storage/grpc/`), including the `codes.Unimplemented` fallback.
2. **`jaeger`**: Add integration tests using a test gRPC server that alternately
   implements and does not implement `FindTraceSummaries`.

**Success criteria:**
- `make test` passes.
- The adapter correctly falls back when the remote server returns `Unimplemented`,
  verified by a test that injects the error.
- An existing remote storage plugin (e.g. the in-tree `memstore` used in tests)
  compiles and passes all tests without implementing `SummaryReader`.

---

### Milestone 5 — Jaeger UI migration

**Goal:** The search screen uses `FindTraceSummaries` instead of `FindTraces`,
delivering the performance improvement to end users.

**Changes:**
1. **`jaeger-ui`**: Add `findTraceSummaries` to the API client.
2. **`jaeger-ui`**: Update the search Redux action/selector and the `SearchResults`
   component to use `TraceSummary` data directly, removing the client-side
   aggregation step.
3. **`jaeger-ui`**: Remove dead code for summary computation from full traces.

**Success criteria:**
- Existing search UI tests pass against mock `FindTraceSummaries` responses.
- Manual QA on the search page: result rows display correct service name, operation,
  duration, span count, error indicator, and service list.
- Network tab in browser DevTools shows response size reduced by ≥ 80% for traces with
  ≥ 50 spans (benchmark with a synthetic test dataset).
- No regression in the trace detail page (still uses `GetTrace`).

---

### Milestone 6 — Native summary support in at least one storage backend

**Goal:** Demonstrate that the optional `SummaryReader` interface provides a real
performance benefit by implementing it in one storage backend without the fallback.

**Candidate:** The in-memory store or Elasticsearch backend (whichever is more
straightforward as a reference implementation).

**Changes:**
1. **`jaeger`**: Implement `SummaryReader.FindTraceSummaries` for the chosen backend.
2. **`jaeger`**: Benchmark comparing native vs. fallback path on a dataset of 1 000
   traces with ≥ 100 spans each.

**Success criteria:**
- Native implementation passes the same golden tests used for the fallback in
  Milestone 2, confirming API contract equivalence.
- Benchmark shows ≥ 50% reduction in backend CPU time and/or bytes read from storage
  compared to the fallback path.

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
