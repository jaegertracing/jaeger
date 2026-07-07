# RFC 0008: Search Result Pagination

- **Status:** Draft
- **Author:** om7057
- **Created:** 2026-07-07
- **Last Updated:** 2026-07-07
- **Issue:** [#2027](https://github.com/jaegertracing/jaeger/issues/2027)

---

## Abstract

The Jaeger query API (`GET /api/traces`) returns at most `SearchDepth` traces per
call, with no mechanism to retrieve additional results. The response envelope already
carries `total`, `limit`, and `offset` fields, but they are never populated. This RFC
proposes a **cursor-based "load more" pagination** for the trace search API — a
stateless opaque token that the caller passes back to retrieve the next page — and
analyses how each supported storage backend can implement or opt out of it.

---

## 1. Motivation

### 1.1 The current state

`GET /api/traces?service=foo&limit=20` returns up to 20 traces. There is no way
to ask "give me the next 20." The response is:

```json
{ "data": [...], "total": 0, "limit": 0, "offset": 0, "errors": [] }
```

`total`, `limit`, and `offset` exist on the struct but `tracesToResponse()` never
fills them. This is not merely a cosmetic gap — users with high-cardinality
services (many operations, long time windows) cannot page through results at all.

### 1.2 Why offset-based pagination does not work here

Offset pagination (`LIMIT n OFFSET k`) requires a stable, deterministic result
ordering. Jaeger's storage backends do not provide one:

- Elasticsearch uses a terms aggregation over `traceID` to find candidate traces,
  then fetches spans for those traces. The aggregation is unordered across shards.
- Cassandra's primary key for the duration index is
  `(service_name, operation_name, bucket, duration, start_time, trace_id)`. A
  trace written between two pages (with a duration less than the current page's
  minimum) will silently appear in the wrong page or not at all.
- Badger iterates in key order; the key encodes start time but not all query
  predicates, so the result set is not stable under concurrent writes.

yurishkuro summarized this in the issue thread (Sep 2023): *"The best
approximation of a token would be the timestamp, such that the next page (as in
'more' not 'page #X') would be a query with `ts > Tprev`."*

### 1.3 The right model: opaque cursor tokens

An opaque cursor is a base64-encoded blob that encodes whatever state the backend
needs to resume its query. The API contract is:

1. If the response includes a non-empty `NextPageToken`, there may be more results.
2. Pass `pageToken=<token>` to the same endpoint (with the same other parameters)
   to retrieve the next page.
3. A missing or empty `NextPageToken` means no further pages.

Crucially, the token format is **backend-specific and opaque to the caller**. The
API surface is uniform; the encoding is not.

---

## 2. Current architecture

### 2.1 HTTP layer

`APIHandler.search()` in
`cmd/jaeger/internal/extension/jaegerquery/internal/http_handler.go`:

```go
func (aH *APIHandler) search(w http.ResponseWriter, r *http.Request) {
    tQuery, _ := aH.queryParser.parseTraceQueryParams(r)
    findTracesIter := aH.queryService.FindTraces(r.Context(), queryParams)
    tracesFromStorage, _ = v1adapter.V1TracesFromSeq2(findTracesIter)
    structuredRes := aH.tracesToResponse(tracesFromStorage, uiErrors)
    aH.writeJSON(w, r, structuredRes)
}

func (*APIHandler) tracesToResponse(traces []*model.Trace, ...) *structuredResponse {
    return &structuredResponse{Data: uiTraces, Errors: uiErrors}
    // Total, Limit, Offset, NextPageToken: never set
}
```

`structuredResponse`:

```go
type structuredResponse struct {
    Data   any               `json:"data"`
    Total  int               `json:"total"`
    Limit  int               `json:"limit"`
    Offset int               `json:"offset"`
    Errors []structuredError `json:"errors"`
}
```

### 2.2 Storage interface

`internal/storage/v2/api/tracestore/reader.go`:

```go
type Reader interface {
    FindTraces(ctx context.Context, query TraceQueryParams) iter.Seq2[[]ptrace.Traces, error]
    FindTraceIDs(ctx context.Context, query TraceQueryParams) iter.Seq2[[]FoundTraceID, error]
    // ...
}

type TraceQueryParams struct {
    ServiceName   string
    OperationName string
    Attributes    pcommon.Map
    StartTimeMin  time.Time
    StartTimeMax  time.Time
    DurationMin   time.Duration
    DurationMax   time.Duration
    SearchDepth   int
    // No PageToken field today
}
```

### 2.3 gRPC remote storage

`FindTraceIDs` on the gRPC path is a **unary** call that returns all IDs at once.
`FindTraceSummaries` is **server-streaming**. Both `TraceQueryParameters` in
`internal/proto-gen/storage/v2/trace_storage.pb.go` and in
`internal/proto/api_v3/query_service.pb.go` have no `page_token` field.

### 2.4 Per-backend query mechanisms

| Backend | How FindTraceIDs works today | Native pagination primitive |
|---|---|---|
| **Elasticsearch/OpenSearch** | Terms aggregation on `traceID`, `Size(searchDepth)`, `Order(startTimeField, false)` | Composite aggregation `after_key` |
| **Cassandra** | Time-bucket range queries + CQL `LIMIT`; result assembled from buckets | CQL driver `PageState` bytes |
| **Badger** | Iterator seek by time prefix, span-level limit | `Seek(lastKey)` — key-based resume |
| **Memory** | In-memory sort + slice | N/A — trivial, no token needed |
| **ClickHouse** | SQL `ORDER BY ... LIMIT n` | SQL `OFFSET` or keyset pagination |
| **gRPC remote** | Delegates to underlying backend via gRPC | Pass-through of opaque token |

---

## 3. Design constraints

| Layer | Constraint |
|---|---|
| **HTTP API** | New `pageToken` query param and `nextPageToken` response field must be additive; existing callers unaffected. |
| **Storage interface (internal)** | Can change freely; it is internal to the binary and versioned with it. |
| **Remote Storage gRPC API** | Third-party plugins must degrade gracefully if they do not support pagination. New fields added to the proto; old plugins that ignore them continue to return full results without a token. |
| **IDL submodule** | The `jaeger-idl` repo owns the public `api_v3` proto. Adding `page_token` to `TraceQueryParameters` requires a separate PR there. This is a Phase 2 concern (§6). |
| **Result ordering** | Backends with non-deterministic ordering MUST document that cursor tokens provide "more results", not "page N of a stable set". |

---

## 4. The pagination capability question

The central design question — raised explicitly by yurishkuro — is whether pagination
should be **a required extension of `tracestore.Reader`** or **an optional capability
that backends advertise**.

### 4.1 Option A: Required interface extension

Add `PageToken` to `TraceQueryParams` and `NextPageToken` to a new
`FindTracesResult` return type. Every backend must accept a token (possibly
ignoring it for the first page) and return a next-token (possibly empty, meaning
"no pagination supported").

**Pros:** Uniform API — callers always use the same path.

**Cons:** Forces all backends to be touched even if they cannot meaningfully
paginate. Memory and in-memory-ish backends (Badger for dev use) gain nothing.
A backend that returns an empty token for every response silently provides no
pagination, which is confusing to users and impossible to distinguish from "last
page reached."

### 4.2 Option B: Optional capability interface

Define a separate interface `PaginatingReader` that extends `tracestore.Reader`.
The query service and HTTP handler detect at runtime whether the loaded backend
implements it:

```go
// PaginatingReader is an optional extension of Reader that backends may
// implement to support opaque cursor-based pagination.
// Backends that do not implement this interface return all results within
// SearchDepth and never emit a NextPageToken.
type PaginatingReader interface {
    Reader
    // FindTracesPage streams trace chunks for one page of results.
    // Each yielded TracesPage carries a ptrace.Traces chunk; the final chunk
    // of the page carries a non-empty NextPageToken if more results exist.
    FindTracesPage(ctx context.Context, query PagedTraceQuery) iter.Seq2[[]TracesPage, error]
}

type PagedTraceQuery struct {
    TraceQueryParams
    PageToken string // opaque, backend-specific; empty means first page
}

// TracesPage is a single chunk from a FindTracesPage call.
// NextPageToken is non-empty only on the final chunk and only when more
// results exist beyond this page.
type TracesPage struct {
    Traces        ptrace.Traces
    NextPageToken string
}
```

The handler checks:

```go
if pr, ok := aH.queryService.TraceReader().(tracestore.PaginatingReader); ok {
    // use pr.FindTracesPage(...)
} else {
    // fall back to existing FindTraces path; return no NextPageToken
}
```

**Pros:**
- Backends that cannot paginate cleanly (Memory, Badger) are not forced to change.
- The capability is discoverable: the UI or a client can probe for it and hide the
  "load more" button if the backend does not support it.
- Follows the same pattern used elsewhere in the codebase (e.g., `SummaryReader`
  in `internal/storage/v2/api/tracestore/summary.go`).

**Cons:** Two code paths in the handler. Backends that partially support pagination
(say, Cassandra with known caveats) must explicitly opt in.

### 4.3 Recommendation

**Option B** — the optional capability interface. The `SummaryReader` precedent
in this codebase is the right model: an optional interface checked at runtime,
graceful fallback, no forced contract on every backend. The capability-based
approach also cleanly answers the Cassandra question (§5.3): Cassandra can opt in
with documented caveats about ordering rather than being forced to either implement
a half-correct token or break the interface contract.

The capability can be **advertised** through the existing `Capabilities()` mechanism
or simply via a runtime type assertion — the latter is simpler and consistent with
`SummaryReader`.

---

## 5. Backend analysis

### 5.1 Elasticsearch / OpenSearch

**Current query path:**
`FindTraceIDs` builds a `terms` aggregation with `Order(startTimeField, false)` and
`Size(searchDepth)`. This aggregation has no "resume" mechanism.

**Proposed cursor mechanism: ES Composite Aggregation**

Replace the `terms` aggregation in `findTraceIDsFromQuery` with an [ES composite
aggregation](https://www.elastic.co/guide/en/elasticsearch/reference/current/search-aggregations-bucket-composite-aggregation.html),
which natively supports an `after` cursor:

```json
{
  "aggs": {
    "traceIDs": {
      "composite": {
        "size": 20,
        "sources": [
          { "startTime": { "terms": { "field": "startTime", "order": "desc" } } },
          { "traceID":   { "terms": { "field": "traceID" } } }
        ],
        "after": { "startTime": 1720000000000, "traceID": "abc123" }
      }
    }
  }
}
```

The `after_key` from the response is serialized as the `NextPageToken`:

```json
{ "startTime": 1720000000000, "traceID": "abc123" }
```

Base64-encoded and returned as the opaque token.

**Ordering semantics:** Results are ordered by `(startTime DESC, traceID ASC)`.
This provides a deterministic, stable ordering that survives concurrent writes
because a new trace written with `startTime > Tprev` will appear in an earlier
page (which the user has already seen) and a new trace with `startTime < Tprev`
will appear in a later page, not the current one.

**Limitations:**
- Composite aggregation requires ES 6.5+ / OS 1.0+. This matches Jaeger's current
  minimum supported versions.
- The `after` cursor is not stable if the index is re-sharded or an alias points
  to multiple differently-mapped indices — document this limitation.
- The existing `FoundTraceID.Start`/`End` time-range hints can be populated
  from the composite aggregation's `min_startTime`/`max_endTime` sub-aggregations,
  making the ES implementation a net improvement on both fronts.

### 5.2 ClickHouse

ClickHouse's SQL engine supports standard keyset pagination:

```sql
SELECT DISTINCT traceID, min(Timestamp) AS minTS
FROM otel_traces
WHERE ServiceName = ? AND ...
  AND (minTS, traceID) < (?, ?)   -- cursor from previous page
ORDER BY minTS DESC, traceID ASC
LIMIT ?
```

The cursor token encodes `(minTS, traceID)`. This is deterministic and efficient
because ClickHouse can use a skip-index on `Timestamp`.

### 5.3 Cassandra

**This is the hard case**, as yurishkuro called out.

Cassandra's CQL driver exposes a `PageState` bytes blob that can be used to resume
a query from where it left off. However:

1. `PageState` is only valid for **the exact same CQL query** on **the same schema
   version**. It is not portable across schema changes or coordinator restarts in
   all cases.
2. Cassandra's duration index primary key is
   `(service_name, operation_name, bucket, duration, start_time, trace_id)`. A
   query for `service=foo, startTime in [T1, T2]` fans out across multiple time
   buckets. `PageState` is per-query, not per-logical-result-set, so it cannot
   trivially resume a multi-bucket fan-out.
3. Cassandra does not guarantee that results are stable between pages — a write
   that arrives between two pages can cause a trace to appear in both or neither.

**Proposed approach for Cassandra:**

Cassandra implements `PaginatingReader` with the following documented semantics:
- The cursor encodes `(lastSeenStartTime, lastSeenTraceID)` — a **time-keyset
  cursor**, not a CQL `PageState`.
- "Next page" is implemented as a new query with `StartTimeMax = lastSeenStartTime`
  (exclusive) and the same other parameters.
- This approach has the same limitations yurishkuro described: traces written with
  `startTime > lastSeenStartTime` after the first query will not appear in later
  pages. This is explicitly documented as the "more results" model, not a snapshot.

Alternatively, Cassandra can **opt out** of `PaginatingReader` entirely.
The capability model (§4.3) makes this a clean choice rather than a half-broken
contract. Given the acknowledged limitations, opting out may be the honest default
until a robust per-bucket `PageState` fan-out is designed.

**This RFC recommends Cassandra be the last backend to implement pagination, with
its support gated on a follow-up investigation of the multi-bucket PageState
approach.** For the initial implementation, Cassandra does not implement
`PaginatingReader`.

### 5.4 Badger

Badger is a development/local backend. Its iterator supports `Seek(key)` for
resuming from a known key. A time-keyset cursor similar to Cassandra's
`(lastSeenStartTime, lastSeenTraceID)` is implementable but of limited value in
practice. Like Cassandra, Badger is recommended to **opt out** of
`PaginatingReader` in the initial implementation.

### 5.5 Memory

The in-memory backend is used in tests and all-in-one dev mode. No pagination
needed; it does not implement `PaginatingReader`.

### 5.6 gRPC remote storage

The gRPC remote storage adapter passes queries through to an external backend
over gRPC. For pagination:

- `TraceQueryParameters` in `trace_storage.proto` gains a `string page_token = 9`
  field (Phase 2, §6).
- The `Handler.FindTraceSummaries` server-side method passes the token to the
  underlying backend if it implements `PaginatingReader`.
- Old backends that do not understand the field ignore it (proto3 default: empty
  string = no token = first page).
- The gRPC `TraceReader` client propagates whatever token the remote server
  returns. If the remote does not support pagination, it returns an empty token and
  the client treats the result as the only page.

---

## 6. Implementation plan

### Phase 1 — HTTP/storage plumbing and ES implementation

**No proto changes. No IDL submodule PR.**

1. **`tracestore` package:** Add `PaginatingReader` interface and `PagedTraceQuery`
   type to `internal/storage/v2/api/tracestore/`.

2. **`TraceQueryParams` extension:** No change to `TraceQueryParams` itself.
   `PagedTraceQuery` wraps it with an additional `PageToken string` field.

3. **`structuredResponse`:** Add `NextPageToken string` field with
   `json:"nextPageToken,omitempty"`.

4. **`query_parser.go`:** Parse `pageToken` URL query parameter.

5. **`http_handler.go`:** In `search()`, check if the backend implements
   `PaginatingReader`. If yes, call `FindTracesPage`; populate `NextPageToken` on
   the response. If no, fall through to the existing `FindTraces` path.

6. **`querysvc.QueryService`:** Add `FindTracesPage` that delegates to
   `PaginatingReader` if the reader implements it.

7. **ES backend:** Implement `PaginatingReader` using the composite aggregation
   approach (§5.1). Emit `NextPageToken` only when the aggregation returns a
   non-empty `after_key`.

8. **ClickHouse backend:** Implement `PaginatingReader` using keyset pagination
   (§5.2).

9. **Tests:** Unit tests for the ES composite aggregation path; integration tests
   asserting that consecutive pages return non-overlapping, non-empty result sets.

### Phase 2 — gRPC / proto propagation

1. **`jaeger-idl` submodule:** Add `string page_token = 10` to
   `TraceQueryParameters` in `query_service.proto` (field 9 is already `raw_traces`),
   and `string page_token = 9` to `TraceQueryParameters` in `trace_storage.proto`
   (currently ends at field 8). Add `string next_page_token = 2` to
   `FindTraceSummariesResponse` in both protos (field 1 is already `summaries`).
   This requires a PR to the `jaeger-idl` repository.

2. **Regenerate protos:** Run `make generate-proto` after the IDL PR merges.

3. **gRPC handler/client:** Propagate `PageToken` from `FindTraceSummariesRequest`
   to the underlying reader; return `NextPageToken` in the response.

4. **Remote storage plugin compat:** Old plugins ignore the new field (proto3
   semantics). Document the behavior.

### Phase 3 — Cassandra / Badger (future, if warranted)

Investigate multi-bucket `PageState` fan-out for Cassandra. Open separate issues
for each backend. Neither is required to make Phase 1 or 2 shippable.

---

## 7. API changes

### 7.1 HTTP

**Request:** `GET /api/traces?service=foo&limit=20&pageToken=<opaque>`

**Response (with more pages):**

```json
{
  "data": [...],
  "total": 0,
  "limit": 20,
  "offset": 0,
  "nextPageToken": "eyJzdGFydFRpbWUiOjE3MjAwMDAwMDAwMDAsInRyYWNlSUQiOiJhYmMxMjMifQ",
  "errors": []
}
```

**Response (last page or non-paginating backend):**

```json
{
  "data": [...],
  "total": 0,
  "limit": 20,
  "offset": 0,
  "errors": []
}
```

The `total`, `limit`, and `offset` fields remain at their current semantics (total
traces in storage is not computable at query time; `limit` reflects the requested
`SearchDepth`; `offset` stays 0 — it is not part of cursor-based pagination).

`nextPageToken` is **omitempty**: backends that do not support pagination simply
omit it, which is indistinguishable to callers from reaching the last page.

### 7.2 gRPC / api_v3 (Phase 2)

```protobuf
message TraceQueryParameters {
  // ... existing fields ...
  // field 9 is raw_traces (api_v3); page_token is 10 in query_service.proto
  // field 9 is available in trace_storage.proto (ends at 8 today)
  string page_token = 10; // api_v3: opaque; empty means first page
}

message FindTracesResponse {
  // ... existing fields (field 1 = summaries) ...
  string next_page_token = 2; // empty means no further pages
}
```

---

## 8. Token encoding

Each backend encodes its cursor as a **JSON object**, base64url-encoded (no
padding), transmitted as a plain string. The schema is backend-internal and not
part of the public API contract.

Example (Elasticsearch):

```go
type esPageToken struct {
    StartTime int64  `json:"st"`
    TraceID   string `json:"tid"`
}
```

Tokens MUST NOT contain PII or credentials. Tokens are **not signed or
encrypted** in Phase 1 — they are opaque to the caller but readable if decoded.
If token tampering becomes a concern, HMAC signing can be added in a follow-up
without changing the API surface.

---

## 9. Capability advertisement

When a client needs to know upfront whether pagination is supported (e.g., the UI
deciding whether to show a "load more" button), two mechanisms are available:

1. **Implicit:** attempt the first query; if `nextPageToken` is absent on a full
   page (`len(data) == limit`), pagination is not supported.
2. **Explicit:** a future `/api/capabilities` endpoint (out of scope for this RFC)
   can advertise `pagination: true/false`.

For the initial implementation, the implicit mechanism is sufficient.

---

## 10. Open questions

1. **Should `FindTracesPage` be a new method on `PaginatingReader`, or should
   `PageToken` be added directly to `TraceQueryParams`?** A new method keeps the
   existing `Reader` interface clean and makes the capability explicitly opt-in.
   Adding to `TraceQueryParams` is simpler but requires every `FindTraces`
   implementation to silently ignore an unknown token, which is error-prone.
   This RFC proposes the new-method approach but would value maintainer input.

2. **Cassandra opt-in timing.** Should Cassandra implement a best-effort
   time-keyset cursor in Phase 1, with documented caveats, or wait until a
   multi-bucket `PageState` approach is designed? The RFC recommends waiting, but
   acknowledges that a time-keyset cursor would already be better than nothing.

3. **Token lifetime / invalidation.** Should tokens have an expiry encoded in
   them? ES composite aggregation cursors are stateless (they encode the last seen
   value, not a server-side scroll context), so they do not expire. Cassandra
   `PageState` blobs do expire (configurable TTL on the coordinator). The RFC
   recommends documenting that tokens are best-effort and may return an error if
   expired or used with different query parameters, rather than encoding TTLs.

4. **`total` field.** It is currently always 0. Should Phase 1 populate it with
   `len(results)` (the count on this page), or keep it 0 to avoid breaking callers
   who may be testing `total == 0` as a sentinel? The RFC recommends keeping it 0
   for now and addressing the semantics in a separate issue.

---

## 11. References

- [Issue #2027: Search API pagination](https://github.com/jaegertracing/jaeger/issues/2027)
- [ES composite aggregation docs](https://www.elastic.co/guide/en/elasticsearch/reference/current/search-aggregations-bucket-composite-aggregation.html)
- [RFC 0005: Qualified Attribute Queries](./0005-qualified-attribute-queries.md) — similar cross-backend capability model
- [SummaryReader interface](../../internal/storage/v2/api/tracestore/summary.go) — precedent for optional reader capability
- [tusharsharma.dev: API pagination the right way](https://tusharsharma.dev/posts/api-pagination-the-right-way#offset-pagination) — linked by yurishkuro in the issue thread