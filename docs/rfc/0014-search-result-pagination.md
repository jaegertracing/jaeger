# RFC 0014: Search Result Pagination

- **Status:** Draft
- **Author:** Yuri Shkuro
- **Created:** 2026-08-12
- **Last Updated:** 2026-08-19
- **Related:** [RFC 0005 (structured query filters)](0005-structured-query-filters.md), [RFC 0011 (trace summary API)](0011-trace-summary-api.md), [ADR-013 (storage capability declaration)](../adr/013-storage-capability-declaration.md)

---

## Abstract

Jaeger's trace-search API returns a single truncated page and offers no way to ask for the next one. A caller sets `search_depth` (documented as a soft `LIMIT`), the backend returns up to that many traces ordered most-recent-first, and everything past the cap is unreachable. There is no offset, no cursor, and no continuation token anywhere in the public api_v3 API, the internal `tracestore.Reader` interface, or the Remote Storage gRPC protocol. This RFC adds end-to-end **keyset pagination** across all three layers: an opaque, self-describing `page_token` that a caller passes back to fetch the next page, a matching `next_page_token` on the response, and a backend capability declaration so that a backend which cannot paginate degrades to a single capped page rather than returning wrong or silently-partial results. The concrete backend design targets Elasticsearch/OpenSearch, replacing the current cross-shard-approximate terms aggregation over trace IDs with a shard-exact, time-ordered scheme paged by a keyset cursor.

The change is additive and backward compatible at every layer: `search_depth` keeps working, and a client that sends no pagination request sees exactly today's behavior.

---

## 1. Motivation

### 1.1 The current contract is a cap, not pagination

A trace search carries a result bound named `search_depth`. In api_v3 it is [`TraceQueryParameters.search_depth`](https://github.com/jaegertracing/jaeger-idl/blob/main/proto/api_v3/query_service.proto) (field 8), documented as: "this may behave like an SQL `LIMIT` clause. However, some implementations might not support precise limits, and a larger value generally results in more traces being returned." The internal interface carries it as `TraceQueryParams.SearchDepth` (`internal/storage/v2/api/tracestore/reader.go`), and the v1 model still calls it `NumTraces` (`internal/storage/v1/api/spanstore/interface.go`). The default is 100 (`defaultSearchDepth`, `internal/storage/v2/elasticsearch/tracestore/core/reader.go`).

There is no way to retrieve results beyond the cap. Raising `search_depth` fetches a larger first page and nothing else; there is no offset, cursor, or continuation token. The v1 HTTP query struct has an `Offset int` field (`cmd/jaeger/internal/extension/jaegerquery/internal/http_handler.go`, `json:"offset"`) that is parsed off the request and then never read by any query path. It is dead code, a vestige of an offset-paging idea that was never wired through, and this RFC does not resurrect it (§10 explains why offset paging is the wrong model and recommends deleting the field).

### 1.2 The Elasticsearch/OpenSearch limit is also approximate

`FindTraceIDs` on Elasticsearch/OpenSearch does not run an ordinary search. It runs a **terms aggregation** on `traceID` with `Size(searchDepth)`, ordered by a `max(startTime)` sub-aggregation descending, to return the top-N most-recent distinct trace IDs (`buildTraceIDAggregation`, `internal/storage/v2/elasticsearch/tracestore/core/reader.go`). Two properties of that aggregation matter here.

It has no notion of a next page. The aggregation returns the top `searchDepth` buckets and stops; there is no `after` key threaded back in, so the traces past the cap cannot be reached at all.

Its ordering is approximate across shards. Ordering a `terms` aggregation by a sub-aggregation metric (here `max(startTime)`) is a documented Elasticsearch limitation: each shard computes its own top-N buckets locally and the coordinating node merges them, so a trace that is globally in the top-N but not locally top-N on the shard that holds it can be dropped from the result. The code does not set `shard_size` to widen the per-shard candidate set, so the error is larger than it needs to be. The current single-page result is therefore already not a reliable "the N most recent traces." Pagination cannot be built on top of an ordering that is not stable and total, so fixing the ordering is a prerequisite, not a side quest.

### 1.3 There is a keyset precedent, at the wrong granularity

The Elasticsearch reader already paginates — but *within a single trace*, not across the result set. When a trace is too large to return in one search, `multiRead` pages its spans with `search_after` keyset paging over `(startTime, spanID)`, where `spanID` is the unique tie-breaker that `startTime` alone cannot provide (`buildTraceReadRequest` and `traceReadCursor`, `internal/storage/v2/elasticsearch/tracestore/core/reader.go`). This is exactly the mechanism result-set pagination needs, one level up: a stable total ordering with a unique tie-breaker, resumed after the last item of the previous page. This RFC reuses the same shape for the result set and is explicit about how the two cursors — the intra-trace span cursor and the inter-trace result cursor — coexist (§7.3).

### 1.4 Why a cap is not enough

A cap forces the user to choose between missing results and paying for a huge first page. Set `search_depth` to 20 and the twenty-first matching trace is invisible; set it to 5000 and every search assembles five thousand traces up front, most of which the user never scrolls to. Neither is what an interactive search wants: fetch a screenful, then fetch the next screenful on demand. Every general-purpose search API — including the trace backends Jaeger is measured against, Grafana Tempo and Elastic's own APM — offers this, and Jaeger's own UI cannot implement "load more" without it. The absence is felt most on high-volume queries, which are precisely the ones where the cap bites.

---

## 2. Goals and non-goals

### Goals

- **G1 — Continuation.** A caller can retrieve results past the first page by passing an opaque `page_token` returned from the previous response, until the backend signals the last page with an empty `next_page_token`.
- **G2 — Completeness and no duplication.** Over a stable dataset, paging start-to-finish visits every matching trace exactly once — no gaps, no repeats — under a stable total ordering. §3.4 states what survives of this when the dataset is not stable: no repeats unconditionally, and no gaps for any trace whose sort key does not move.
- **G3 — Bounded cost per page.** Fetching page *k* costs the same as page 1; there is no term whose cost grows with *k* (no deep-paging wall, in contrast to offset paging; §3).
- **G4 — End to end.** The token and page size travel through all three layers: public api_v3, the internal `tracestore.Reader`, and the Remote Storage gRPC protocol, so a remote backend paginates too.
- **G5 — Backward compatibility.** `search_depth` keeps its current meaning, the new fields default to empty, and a client that never asks for pagination gets byte-for-byte today's behavior. No previously valid request changes meaning.
- **G6 — Honest degradation.** A backend that cannot paginate *declares* so through the capability mechanism of ADR-013; the query service then serves a single capped page and reports the truncation, rather than pretending a short result is complete. This mirrors the posture RFC 0005 §7 established for unserviceable filters.

### Non-goals

- **Random access / jump-to-page-N.** Keyset pagination is sequential: you can go to the next page, not directly to page 42. Total result counts and page numbers are not provided (§3 explains why this is the right trade for a time-series store). "Previous page" is a client concern (remember prior tokens), not a server feature.
- **Snapshot isolation across a paging session.** The dataset may change between page requests. A forward traversal never repeats a trace, but it may skip one that arrives into or migrates into a region it already passed; §3.4 states the guarantee precisely and §3.5 gives the client contract for revisiting a page. A point-in-time snapshot is considered and rejected in §10 as stateful and costly.
- **Result shaping beyond ordering and paging.** Projection, field selection, and aggregation are RFC 0005's L4 and out of scope. This RFC delivers the *paging* half of RFC 0005's deferred L3 tier (§8).
- **Changing the matching semantics.** Which traces match, and the same-span-vs-any-span question, are unchanged. Pagination orders and slices the matched set; it does not redefine it.
- **A new sort API.** The user-visible order stays most-recent-first. A configurable sort key is a future extension the cursor design leaves room for (§12), not part of this RFC.
- **Paging the spans of a single trace.** `GetTraces` gains no page token. Paging the *set* of requested IDs is not a question — api_v3's `GetTraceRequest` names one trace, and the storage-layer `GetTraces(ctx, traceIDs...)` takes a list the caller already chunks itself. Paging the *spans within* one trace is a real need for traces too large to return or render, and the machinery is tantalizingly close: `multiRead` already keyset-pages a large trace's spans and merely exhausts the cursor internally (§1.3, §7.3). It is still the wrong answer to that need. A page of search results is a usable answer on its own — twenty traces a user can read — whereas a page of spans is an arbitrary prefix by start time, with parent references pointing at spans the client does not have, no complete critical path, and no meaningful duration or service breakdown, so a UI receives a page it must discard. What a caller actually wants from an oversized trace is a *semantic* reduction — a subtree, one service, spans over a duration threshold, or a summary — which is result shaping, RFC 0005's L4 tier, and is excluded above for that reason. A span cursor would also have to encode both which trace it stopped in and where inside it, since the storage method is variadic over trace IDs, giving the cursor two jobs where the result cursor has one. Should the case be made later (bulk export of a giant trace is the plausible one), nothing here needs redesigning: `Pagination` is a standalone message, so `GetTraceRequest` can adopt it as-is, and the token is opaque, so `multiRead`'s existing `(startTime, spanID)` cursor drops straight in.

---

## 3. Pagination model — the core decision

Three models can deliver continuation. The columns below are those three; the rows are the properties G1–G3 demand plus the operational concerns a trace store imposes.

**Offset (`from` + `size`).** Page *k* asks the backend to compute the first `k × size` results and discard all but the last `size`. This is the SQL `OFFSET`/`LIMIT` model and what the dead `Offset` field gestured at.

**Keyset cursor (`search_after`).** Results carry a stable total ordering with a unique tie-breaker. Page *k+1* asks for "the next `size` results ordered after *this* sort key," where the sort key is the last row of page *k*. No offset is computed; the backend seeks to the cursor and scans forward.

**Opaque page-token wrapping a keyset cursor.** Identical execution to the keyset cursor, but the sort-key values are wrapped in an opaque, backend-produced token (base64 of a small proto) rather than exposed as raw fields on the wire. The client treats the token as a blob and echoes it back.

Legend: 🟢 good · 🟡 partial / caveated · 🔴 poor

| Criterion | Offset (`from`+`size`) | Keyset cursor (raw) | Opaque token over keyset |
|-----------|---|---|---|
| Deep paging (page *k* cost) | 🔴 O(*k*·size); hits ES `max_result_window` | 🟢 O(size) | 🟢 O(size) |
| Completeness under concurrent writes | 🔴 inserts shift rows, causing skips *and* dupes¹ | 🟡 no dupes ever; late data can be missed (§3.4) | 🟡 no dupes ever; late data can be missed (§3.4) |
| Works over the ES/OS aggregation path | 🔴 aggregations have no offset skip² | 🟡 needs the shard-exact scheme of §7 | 🟡 needs the shard-exact scheme of §7 |
| Random access (jump to page N) | 🟢 native | 🔴 sequential only | 🔴 sequential only |
| Wire contract evolvability | 🟢 two ints | 🔴 leaks sort-key shape³ | 🟢 opaque, backend-defined |
| Cross-backend uniformity | 🟡 offset is universal but the wall differs | 🟡 each backend's cursor differs | 🟢 each backend hides its cursor behind one token type |
| Guards against cross-query token misuse | n/a | 🔴 nothing binds the cursor to its query | 🟢 token embeds a query fingerprint (§3.2) |

¹ With most-recent-first ordering, a trace written between page requests shifts every subsequent absolute offset by one, so offset paging either repeats a trace at the page boundary or skips one. ² The Elasticsearch/OpenSearch `FindTraceIDs` path is an aggregation, not a hit search (§1.2); aggregations expose no `from` skip, so offset paging is not even implementable there without abandoning the aggregation. ³ A raw cursor puts `startTime`/`traceID` on the wire as first-class fields, which pins the ordering into the public contract and makes changing the sort key later a breaking change.

**Decision — opaque page-token wrapping a keyset cursor, stateless and self-describing.**

Offset paging fails the two properties that matter most for a time-series store — bounded deep-paging cost and completeness under writes — and is not implementable over the ES/OS aggregation path at all. It buys only random access, which an interactive "load more" flow does not use and which is expensive to offer honestly over sharded storage. It is rejected.

Between the raw keyset cursor and the opaque token, the execution is identical; the only question is what crosses the wire. The opaque token wins on three counts. It keeps the sort-key structure out of the public contract, so the ordering can change (a configurable sort key, §12) without a wire break. It lets each backend define its own cursor — a `search_after` tuple for ES/OS, a `LIMIT`/`WHERE` boundary for ClickHouse, a paging state for a remote plugin — behind one uniform `string page_token`, which is what makes G4 (one contract across backends) achievable. And it has somewhere to carry a **query fingerprint** so a token minted for one query cannot be replayed against a different one (§3.2). The cost is losing a generated strongly-typed cursor for gRPC clients, which is immaterial for a value the client only ever echoes back.

### 3.1 The token is stateless, not a server session

The token is **self-describing**: it encodes the cursor, so the server keeps no per-session state. This is decisive operationally. Jaeger-query runs behind a load balancer and restarts freely; a stateful cursor (a server-held scroll or point-in-time handle keyed by a session ID) would pin a paging session to one process and one moment, break across a restart or a rebalanced request, and require eviction policy and memory for abandoned sessions. A stateless token has none of that: any replica can serve any page, and an abandoned paging session costs nothing because nothing was allocated. The proposed encoding is `base64(proto{ cursor bytes, query fingerprint, backend tag, version })`.

### 3.2 Binding the token to its query

A keyset token is only meaningful against the query that produced it — its cursor is a position in *that* query's ordering. The token therefore embeds a **fingerprint** (a hash) of the request's matching parameters (service, operation, time range, duration bounds, filter). On continuation the query service recomputes the fingerprint from the incoming request and compares; a mismatch is rejected as `InvalidArgument` rather than silently returning nonsense from a cursor that means something else. The `version` byte lets the token format evolve, and the `backend tag` lets the query service reject a token replayed against a different storage backend.

### 3.3 Stable total ordering and the tie-breaker

Keyset paging requires a **total** order with a **unique** tie-breaker; otherwise two traces with equal sort values straddle a page boundary and one is dropped or repeated. The order is most-recent-first on a per-trace time, tie-broken by trace ID: **`(startTime desc, traceID asc)`**. Trace ID is globally unique, so the composite key is total, and the cursor is exactly that pair. This is the same discipline the intra-trace span cursor already applies with `(startTime, spanID)` (§1.3); here `traceID` plays `spanID`'s role one level up. "Per-trace `startTime`" needs a precise definition because a trace has many spans at many times. §7.1 pins it to the trace's maximum span `startTime`, which is what the current ordering already uses and what the Elasticsearch mechanism of §7.2 can express — and also, as §7.1 explains, not the value Jaeger's own `TraceSummary.MinStartTime` calls a trace's start.

### 3.4 Behavior under concurrent writes

Keyset paging defines each page by *position in the ordering* rather than by absolute offset, and that is what makes it well-behaved under writes: a page boundary is a key, so adding or removing a trace elsewhere in the order renumbers nothing. What it does not do is freeze the result set, and for a trace store the reason is sharper than for an ordinary table, because the sort key comes from the data rather than from the clock.

The per-trace sort value is the trace's maximum span `startTime` (§7.1), which records when the work happened, not when the document was indexed. Spans arrive late as a matter of course — SDK batching, collector retries and backpressure, mobile or offline clients flushing much later — so a write that lands at wall-clock time *T* can carry a `startTime` far older than *T*, and it lands **anywhere** in the ordering, including in a region a traversal already passed. New data is not confined to the front of the order.

The sort key can also **move**. Because a trace's key is the *maximum* over its spans, appending a span with a later `startTime` raises that maximum, and since the order is `startTime desc` a raised maximum moves the trace *earlier*, toward page 1. That direction is one-way: the maximum only ever increases, so a trace can migrate toward pages already visited but never toward pages still ahead. The result is an asymmetry that is worth stating plainly, because it is the actual guarantee this RFC offers:

- **A forward traversal never returns the same trace twice.** Each page asks for keys strictly after the previous page's last key, and a trace whose key moves can only move into the already-passed region, so it cannot come back around.
- **A forward traversal may skip a trace.** A trace sitting ahead of the cursor whose key moves behind it — because a late span raised its maximum — is not visited, and neither is a trace written late directly into an already-passed region. Such a trace is not lost: a fresh search, or a re-fetch of the page that now covers it, shows it.

So the precise promise is this: *within one forward walk, using at each step the token returned by the immediately preceding response, every trace whose sort key does not change during the walk is visited exactly once.* Traces that arrive into, or migrate into, an already-passed region are missed by that walk. This is the snapshot-isolation non-goal of §2, stated in terms of what actually causes it, and returning slightly fewer or slightly more traces than a hypothetical frozen view is the accepted cost — bounded by how much data lands in the traversed region during the session, and with no effect on per-page cost.

Which of the two artifacts a deployment gets is decided by the choice of sort key, and the direction reverses if that choice is ever revisited (§7.1, §12). Keying on the maximum `startTime` over matching spans, as this RFC does, means the key only rises — a late span moves it only if that span matches the query — so a trace can only migrate toward the front, and skips are possible while duplicates are not. Keying on the minimum would invert it: a minimum only falls, moving the trace toward the back, so duplicates would be possible and skips would not — arguably the safer failure, since a client can dedupe by trace ID whereas a skip is silent. Either way the exposure is concentrated at the front of the ordering, because a trace's key stops moving once its spans have all been ingested; deep pages are read over settled data, and the churn is on page 1, where "load more" has not been pressed yet.

One mechanical detail matters here. The cursor is a *value*, not a handle on a document, so a `search_after` on a key whose trace has since expired or been deleted remains perfectly valid and returns everything ordered after that key. Nothing breaks when the trace a token names disappears, which is not true of a stateful scroll or point-in-time handle (§3.1, §10).

### 3.5 Revisiting a page with a remembered token

A client that keeps the token which opened each page — as a UI offering "previous page" naturally would — needs to know what those tokens mean once the data has moved. Suppose the user paged through five pages, the UI remembered the five tokens, and late data then arrived.

Re-fetching one page with its remembered token is well defined and cheap. The token is stateless and carries no timestamp (§3.1), so it never expires and cannot become unusable. What it returns is "the next *N* traces after this key **as the index stands now**," which may differ from what that page showed before: a trace inserted inside the page appears, and pushes that page's former last entry out past the boundary.

What is **not** safe is combining a freshly-fetched page with a remembered token from further down. That breaks both properties, in both directions. Take pages of two over the ordering `a, b, c, d, e, f` — page 1 is `[a,b]`, page 2 is `[c,d]`, page 3 is `[e,f]`, and the remembered tokens are the keys `b` and `d`:

- **A gap.** A trace `x` is written with a key between `c` and `d`. Re-fetching page 2 after `b` now yields `[c,x]`, pushing `d` past the boundary. Continuing with the remembered token `d` yields `[e,f]`, so `d` is never shown, even though nothing about `d` changed.
- **A repeat.** `c` expires. Re-fetching page 2 after `b` now yields `[d,e]`. Continuing with the remembered token `d` yields `[e,f]`, so `e` is shown twice.

Both artifacts disappear if the client always steps forward using the token from the response it most recently received. That is the client contract, and it is simple enough to state as a rule: remembered tokens are a stack for going *backward*, and whenever a page is fetched or re-fetched, the client discards every remembered token below it and rebuilds them from the responses as the user moves forward again. A UI that pushes the token it just received and pops on "previous page" gets this for free.

The server cannot enforce that contract and deliberately does not try. Detecting that a remembered token's region has shifted would mean binding the token to a point in time, which is the stateful design §3.1 and §10 reject, and it would trade a rare self-correcting display artifact for tokens that expire.

---

## 4. Wire/API shape (api_v3)

Pagination adds one request field and one response field, both additive. The two paging inputs — the page bound and the continuation token — go into a nested `Pagination` message rather than onto `TraceQueryParameters` directly:

```protobuf
message TraceQueryParameters {
  // ... existing fields 1..9 (service_name, operation_name, attributes,
  // start_time_min/max, duration_min/max, search_depth = 8, raw_traces = 9) ...

  // pagination requests a paginated search. When the message is absent the search
  // behaves exactly as today: one page bounded by search_depth, no continuation.
  Pagination pagination = 11;
}

// Pagination asks for one page of results and, on continuation, says where the
// previous page stopped.
message Pagination {
  // page_size bounds the number of results in one page. When zero it falls back to
  // search_depth (and search_depth's own default).
  int32 page_size = 1;

  // page_token continues a previous search. Empty starts a new one. The value is
  // opaque: clients MUST treat it as a blob and echo back exactly what the server
  // returned. A token is only valid for the same query that produced it.
  string page_token = 2;
}
```

Field numbers are illustrative and must be coordinated with RFC 0005, which reserves field 10 (`filter`) on the same message; 11 is shown here to avoid that collision. Grouping the two fields costs one field number instead of two on a message that already carries nine, plus RFC 0005's filter.

Nesting buys more than tidiness. The two fields are one concept — a page bound is meaningless without the cursor it bounds, and both are always supplied by the same caller for the same purpose — so a reader of the message sees one thing to understand rather than two fields it must infer are related. It also gives the request **presence semantics** that flat fields cannot express: because a proto3 message field distinguishes absent from default-valued, "this client does not know about pagination" (message absent) is a different request from "this client wants a page of the default size" (message present, `page_size` zero). With two flat fields, zero and empty would have to serve for both, and the server could not tell an old client from a new one asking for defaults. Finally, one message type defines the paging contract once and can attach to any request that becomes paginated later, instead of each one growing its own pair of fields. The trade-off is a deliberate divergence from the flat `page_size`/`page_token` convention of Google's AIP-158, and one extra level of access for callers; §5 keeps the Go side ergonomic by mirroring the message as a value struct.

`Pagination.page_size` is the page bound; `search_depth` is retained and its role is clarified rather than changed. When `page_size` is zero it defaults to `search_depth`, so `search_depth` becomes the default page size for callers that have not adopted pagination — and such a caller, sending no `Pagination` at all, receives one page and no continuation, which is precisely today's behavior (G5). `search_depth` additionally remains meaningful as an optional **overall traversal ceiling**: a backend or the query service may cap the total number of pages a single cursor lineage will yield, so an unbounded "load more" cannot walk an entire index; when the ceiling is reached the response returns an empty `next_page_token` even though matches remain. Whether to enforce such a ceiling by default is an open question (§12).

The response carries the continuation token as a flat field rather than a mirrored wrapper message, because there is only one thing for a response to say about paging and nothing to group it with; the request nests because it has two related inputs, not for symmetry's own sake. `FindTraceIDs` returns a single `FindTraceIDsResponse`, which gains the field directly; `FindTraceSummaries` streams `FindTraceSummariesResponse` chunks, and the **last** chunk carries the token:

```protobuf
message FindTraceIDsResponse {
  repeated FoundTraceID trace_ids = 1;
  string next_page_token = 2;   // empty when there are no more pages
}

message FindTraceSummariesResponse {   // streamed; the final chunk sets the token
  repeated TraceSummary summaries = 1;
  string next_page_token = 2;
}
```

`FindTraces` is the awkward one. Its api_v3 response is a stream of OpenTelemetry `TracesData`, an OTLP type with no room for a Jaeger continuation field. Rather than wrap or fork the OTLP stream, pagination attaches to the **find** half of the search, not the **fetch** half. The query service already implements `FindTraces` as find-then-fetch — resolve matching trace IDs, then load those traces — and the ES reader does exactly this internally (`FindTraces` calls `FindTraceIDs`, then `multiRead`; §1.2, §7.3). So the token-bearing primitives are `FindTraceIDs` and `FindTraceSummaries`, and `FindTraces` pagination is driven by the query service: it paginates the ID resolution (which yields a token), fetches that page of traces, and surfaces `next_page_token` on the api_v3 HTTP response envelope alongside the streamed traces. No pagination field is added to the OTLP stream, and the storage-layer `FindTraces` RPC (which streams whole traces) is left un-tokenized on purpose (§5, §6).

For the UI this is a non-issue: the search-results list is populated from `FindTraceSummaries` (RFC 0011), which carries the token cleanly, so "load more" in the results list is a `FindTraceSummaries` continuation.

---

## 5. Internal storage interface

`TraceQueryParams` (`internal/storage/v2/api/tracestore/reader.go`) gains the inbound fields mirroring the proto:

```go
type TraceQueryParams struct {
    // ... existing fields ...
    SearchDepth int
    Pagination  Pagination // zero value: not a paginated request
}

// Pagination mirrors the proto message of the same name.
type Pagination struct {
    PageSize  int    // page bound; zero means this is not a paginated request
    PageToken string // opaque continuation cursor; empty starts a new search
}
```

The Go mirror is a value struct, not a pointer, so no reader has to nil-check before reading a page size. The proto's absent-versus-present distinction (§4) is resolved at the API boundary: the query service turns an absent `Pagination` into the zero struct and a present one into a concrete `PageSize`, having already applied the `search_depth` fallback. A reader therefore reads `PageSize > 0` as "this is a paginated request, return a cursor" and never has to reason about which fields the client set.

The outbound token needs a home on the return path. `FindTraceIDs` returns `iter.Seq2[[]FoundTraceID, error]` — batches of IDs — and the cursor belongs to the *page*, not to any single ID. The recommended shape carries the token on the page rather than widening `FoundTraceID`:

```go
// FindTraceIDs yields the page's IDs together with the cursor for the next page.
FindTraceIDs(ctx context.Context, query TraceQueryParams) iter.Seq2[TraceIDPage, error]

type TraceIDPage struct {
    TraceIDs      []FoundTraceID
    NextPageToken string // set on the terminal batch; empty when no more pages
}
```

This changes the element type of an internal iterator, which is acceptable because the internal `tracestore.Reader` is versioned with the binary — ADR-013 and RFC 0005 both rely on evolving it in step with its callers, unlike the published gRPC contract. The alternative of a raw `[]FoundTraceID` element with the token smuggled onto the last element's fields was rejected as a worse fit: the token is not a property of a trace ID. `FindTraceSummaries` gains the token the same way, on its yielded summary page.

`FindTraces` at this interface keeps returning `iter.Seq2[[]ptrace.Traces, error]` with no token, for the same reason as §4: whole-trace streaming is the fetch half, and the query service drives its pagination through `FindTraceIDs`. A backend therefore implements the cursor logic once, in its ID/summary search, and inherits `FindTraces` pagination for free.

The query service is the single place that mints and validates tokens end to end: it builds the reader's `Pagination` from the api_v3 request, checks the fingerprint (§3.2), calls the reader, and relays `next_page_token` back — the same central-enforcement posture ADR-013 uses for capabilities.

---

## 6. Remote Storage gRPC (storage/v2) and capability declaration

The same shape goes on `jaeger.storage.v2` so a remote backend paginates over the wire. `jaeger.storage.v2` does not import `jaeger.api_v3` — it already carries its own parallel `TraceQueryParameters` — so it declares its own `Pagination` message with the same fields rather than sharing one type across the two packages:

```protobuf
message TraceQueryParameters {   // storage/v2
  // ... existing fields 1..8 (search_depth = 8) ...
  Pagination pagination = 9;
}

message Pagination {   // storage/v2; same shape as the api_v3 message
  int32  page_size  = 1;
  string page_token = 2;
}

message FindTraceIDsResponse {
  repeated FoundTraceID trace_ids = 1;
  string next_page_token = 2;
}

message FindTraceSummariesResponse {   // streamed; final chunk sets the token
  repeated TraceSummary summaries = 1;
  string next_page_token = 2;
}
```

Field numbers are illustrative and must be coordinated with RFC 0005's additions to the same messages. The storage-layer `FindTraces` RPC — a stream of OTLP `TracesData` — gains no token, matching §4/§5: a remote backend exposes its cursor through `FindTraceIDs`/`FindTraceSummaries`, and the query service composes `FindTraces` from the paginated ID search.

### 6.1 Declaring the capability

A plain additive `page_token` would be a silent gap at the remote boundary. A plugin that predates the field ignores the unknown `page_token`, always answers page one, and returns an empty `next_page_token` — indistinguishable, to the client, from a genuine last page. The client would conclude "no more results" when in truth the backend never understood the request. This is the exact failure ADR-013 was built to prevent, so pagination plugs into the same mechanism rather than inventing its own.

`SearchCapabilities` (`internal/storage/v2/api/tracestore/reader.go`, and `jaeger.storage.v2` `SearchCapabilities` carried by the `Capabilities` service) gains a `Paginated` field alongside the existing `WithoutServiceName`. Its zero value must mean "cannot paginate," so that every existing implementation — and every plugin predating the field, which the `Capabilities` service already maps to the least-capable reading (ADR-013) — declares no pagination and is never sent a `page_token`:

```protobuf
message SearchCapabilities {
  bool without_service_name = 1;   // ADR-013
  bool paginated            = 2;   // RFC 0014; absent/false = single capped page only
}
```

```go
type SearchCapabilities struct {
    WithoutServiceName bool
    Paginated          bool // false (zero value) = cannot paginate; serve one capped page
}
```

The capability is `Paginated`, an adjective describing the reader's behavior, and not `Pagination`, so that it does not collide with the `Pagination` request type of §4/§5 — both live in package `tracestore`, and `caps.Pagination` sitting next to `tracestore.Pagination` would make every mention of either ambiguous.

A boolean suffices for the first cut because the honest keyset scheme is the only kind of pagination on offer; if a backend ever gains a weaker mode (e.g. best-effort paging that cannot promise completeness), this becomes an enum, exactly as ADR-013 anticipates capability fields graduating from bool to richer types.

### 6.2 How the query service degrades

The query service reads the declaration before it dispatches and behaves like this:

- **Backend declares `Paginated = true`.** Pass the `Pagination` message through; relay `next_page_token`. Full pagination.
- **Backend declares `Paginated = false`, request has no `page_token`.** Serve a single page capped at `page_size`/`search_depth` and return an **empty** `next_page_token`. This is a *reported degradation*, not an error: the query service still surfaces the truncation honestly through the capability the UI already reads (ADR-013's reporting path), so the client can show "results may be incomplete" rather than mistaking a short list for the whole answer.
- **Backend declares `Paginated = false`, request carries a `page_token`.** Reject with `InvalidArgument`. A backend that cannot paginate cannot have minted a valid cursor, so any token presented to it is either forged or stale, and answering it would be a lie.

This is the RFC 0005 §7 posture applied to pagination: a backend-wide limit is *declared* and enforced centrally before dispatch, so a backend never silently returns a wrong or partial result. Which backends declare `true` at rollout is set by §7 (ES/OS) and §11 (the roadmap for the others); ClickHouse can paginate natively (`WHERE (startTime, traceID) < cursor ORDER BY … LIMIT size`), while the flat backends (Cassandra, Badger) declare `Paginated = false` until their index is made cursor-able, and degrade honestly in the meantime.

---

## 7. Elasticsearch/OpenSearch implementation

This is the focus, because the current ES/OS path (§1.2) is both un-pageable and approximate, and fixing it is the substance of the work. The design has to produce, for a matching query, the distinct trace IDs ordered most-recent-first, sliced into pages that are exact, shard-safe, and resumable by a cursor.

### 7.1 The core tension: distinct traces, ordered by time, paged exactly

The difficulty is specific and worth stating plainly, because it drives the choice. A trace is many span documents at many `startTime`s. Three requirements pull against each other on Elasticsearch: (a) reduce spans to **one row per trace**; (b) order those rows by a **per-trace time** (most-recent-first); (c) **page** them exactly and shard-safely with a resumable cursor. Any two are easy; all three at once is where ES fights back, and each candidate mechanism gives up on a different one.

Two of the three mechanisms weighed in §7.2 are aggregations, which need no introduction. The third is **field collapsing**, and because the argument below turns on how it behaves, here is what it actually does. Collapsing is a feature of an ordinary hit search rather than an aggregation: adding `"collapse": {"field": "traceID"}` to a search groups the matching documents by that field's value and returns **one hit per distinct value** instead of every matching document. Which document represents each group is not something the caller picks — it is whichever one the search's own `sort` ranks first inside that group — and the groups then come back in the order of those representatives' sort values. So a search sorted `startTime desc` and collapsed on `traceID` yields, for each matching trace, its latest-starting span, with traces ordered by that span's time. Two consequences run through the rest of this section: the sort chooses the representative *and* the group order together, they cannot be set independently; and the reported hit total counts matching *documents*, not groups, which is why the number of distinct traces is not cheaply available (§7.5).

The per-trace time is worth stating precisely, because the loose version hides the problem. Today's ranking value is the **maximum `startTime` among the spans that matched the query** — not the trace's maximum, because a terms aggregation is computed over the query's result set, so spans excluded by the service, operation, tag or time filters never enter the `max` at all. Whether that value should be the ranking key is left open in §12. Two properties of it are worth knowing first.

Max is what the current ordering uses, and it has been there since July 2017. `buildTraceIDAggregation` gained `Order(startTimeField, Descending)` over a `max(startTime)` sub-aggregation in "Fix ES to retrieve most recent traces" (#297), which fixed a real bug: before it the terms aggregation carried no order at all, so Elasticsearch applied its default of doc count descending and search results were ranked by which trace had the most spans. Nothing records why the recency metric is the maximum rather than the minimum — the commit message is its one-line subject, the pull request body is empty, and nine years later `buildTraceIDSubAggregation` still carries no comment. The likely reading is that max was simply a natural way to say "most recent" when the alternative on the table was no ordering at all, rather than a decision taken between max and min.

Jaeger already defines a trace's own time interval, and this value is neither end of it. `TraceSummary` documents `MinStartTime` as "the start timestamp of the earliest span in the trace" and `MaxEndTime` as the maximum across span end timestamps, adding that "Duration can be derived as MaxEndTime - MinStartTime" (`internal/storage/v2/api/tracestore/summary.go`). The interval Jaeger attributes to a trace is therefore `[MinStartTime, MaxEndTime]`, and the search ranks by a point in its interior. The results list shows the split: nothing re-sorts between storage and the client, so the list is **ordered by that interior point and labeled with `MinStartTime`**. For a request-scoped trace the two are milliseconds apart and nobody notices; for a long-running trace — a batch job, a streaming consumer — they diverge by the trace's whole duration, and a trace that started earlier can outrank one that started later.

The value also shifts with the query. Because the `max` is taken over *matching* spans, the same trace ranks differently depending on what was searched for: filter on a service that appears only in the trace's first second and the trace ranks early, filter on a service that appears near its end and the same trace ranks late. A ranking key that moves with the query is hard to defend under any definition of a trace's start time.

No specification settles which value is right. OTLP has no trace-level entity — `TracesData` is resource spans containing scope spans containing spans — so no normative "trace start time" exists to appeal to. So the case against it rests on Jaeger's own definitions, not on what a trace's start time ought to mean. Either way, the inconsistency predates pagination.

The choice is free today, and the mechanism this RFC adopts takes the freedom away. A terms aggregation can be ordered by any single-value metric sub-aggregation, so ranking by the minimum is a one-word change — and `FindTraceSummaries` already computes the value: `buildTraceSummariesAggregation` requests both `min_start` and `max_start`, feeds `min_start` into the summary the results list displays, and orders by `max_start` two lines below. Field collapsing cannot be made to do the same at any price, and the reason is the coupling noted above: one sort picks both the representative and the group order. Making the representative each trace's *earliest* span requires sorting `startTime asc`, which simultaneously flips the traces into oldest-first order. Ordering traces by their minimum, descending, is not expressible.

This RFC accepts that cost and keeps max, on two grounds. The order users see does not change, which keeps the RFC to pagination instead of bundling in a silent change to what "most recent" means. And no other candidate mechanism delivers exact, shard-safe, resumable paging over distinct traces at all (§7.2), so the alternative is not "collapse keyed on min" but "no pagination" — which is why the row appears in the §7.2 matrix as a cost of the winning option rather than as a tiebreaker. Fixing the semantics properly means giving the search a trace-level start time to sort on, which needs one document per trace (§11). §3.4 records what the choice means for paging, §12 carries the open question, and the opaque token (§3) keeps a later switch to a min-keyed order off the wire contract.

### 7.2 Candidate mechanisms

**Terms aggregation (today).** Bucket by `traceID`, order by a `max(startTime)` sub-aggregation. Gives (a) and (b) but not (c): no `after` key, and the sub-metric ordering is cross-shard approximate (§1.2).

**Composite aggregation with `after_key`.** The composite aggregation is ES's built-in *paged* aggregation: it emits buckets ordered by a composite of source values and threads an `after_key` back to resume, exactly and shard-safely. But it orders strictly by its **source keys**, and it cannot order buckets by a sub-aggregation metric. Sourcing on `traceID` alone gives exact, resumable, distinct traces — but ordered lexicographically by trace ID, losing (b), the most-recent-first order users expect. Sourcing on `(startTime, traceID)` restores a time order but breaks (a): a trace with spans at *N* distinct `startTime`s produces *N* separate buckets, so "one row per trace" is lost and cross-page de-duplication becomes necessary — which a stateless cursor cannot do, because a trace seen on page 1 could resurface on page 5. So the composite aggregation is exact and shard-safe (its headline strength) but cannot simultaneously give distinct-per-trace **and** recency order.

**Field collapse + `search_after`.** Collapsing on `traceID` (§7.1) gives one hit per distinct trace, and because it is a plain hit search it composes with `search_after`, which aggregations cannot. Sorting by `[{startTime: desc}, {traceID: asc}]` makes each trace's representative its latest-starting matching span, orders traces by that span's time with `traceID` as the unique tie-breaker, and resumes after the `(startTime, traceID)` cursor — preserving today's ranking value (§7.1) rather than changing it. This gives all three: distinct per trace (a, via collapse), most-recent-first (b, via the sort), and exact resumable paging (c, via `search_after`, which is shard-safe by construction — the coordinating node merges per-shard results against a total order). It is also the natural generalization of the intra-trace `search_after` the reader already runs (§1.3).

Legend: 🟢 good · 🟡 partial / caveated · 🔴 poor

| Criterion | Terms agg (today) | Composite agg (`after_key`) | Collapse + `search_after` |
|-----------|---|---|---|
| One row per trace (distinct) | 🟢 | 🟡 only if sourced on `traceID` alone | 🟢 via `collapse` |
| Most-recent-first ordering | 🟢 | 🔴 lexicographic by source key¹ | 🟢 via sort |
| Exact / shard-safe | 🔴 cross-shard approximate² | 🟢 headline strength | 🟢 `search_after` is shard-safe |
| Resumable cursor (next page) | 🔴 none | 🟢 `after_key` | 🟢 `search_after` |
| Can rank by per-trace *minimum* `startTime` (§7.1) | 🟢 order by a `min` sub-aggregation | 🔴 no metric ordering at all | 🔴 representative is the sort's top hit |
| Reuses existing reader machinery | 🟡 aggregation path | 🔴 new aggregation path | 🟢 same as intra-trace paging (§1.3) |
| Cost per page | 🟡 recomputes top-N each call | 🟢 bounded | 🟢 bounded³ |

¹ Ordering distinct traces by recency needs ordering buckets by a per-trace time metric, which the composite aggregation cannot do; sourcing on `(startTime, traceID)` to get a time order reintroduces duplicate trace buckets and defeats distinctness. ² Ordering a terms aggregation by a sub-metric drops globally-top traces that are not locally top on their shard, worsened by an unset `shard_size` (§1.2). ³ Collapse examines more than `page_size` documents per shard to find distinct groups, but the cost is bounded per page and does not accumulate with page depth the way an offset would; `search_after` carries no `from` offset, so the `index.max_result_window` (~10k) deep-paging wall never applies.

**Decision — field collapse on `traceID` plus `search_after`.** It is the only candidate that satisfies distinctness, recency ordering, and exact resumable paging together; it fixes the cross-shard approximation of §1.2 as a side effect (a correctness win independent of pagination); and it reuses the keyset discipline the reader already applies one level down, so `esquery` needs a new `collapse` clause but not a new execution path. The composite aggregation, despite being ES's purpose-built paging aggregation, is disqualified by requirement (b): it cannot order distinct traces by time. The terms aggregation is replaced outright.

Collapse does lose one row: the terms aggregation can rank traces by their own start time and collapse cannot, so this decision freezes the maximum-span-`startTime` ordering that §7.1 argues is the wrong value. That is the price of the only mechanism that can page at all, and it is a price the terms aggregation only appears to avoid — its ordering is approximate across shards either way, so what it offers is the right metric over an unreliable ranking. Fixing the metric properly belongs with the per-trace document of §11.

### 7.3 Two cursors, cleanly separated

There are now two `search_after` cursors at different granularities, and they must not be confused. The **result cursor** paginates *distinct traces* in `FindTraceIDs`/`FindTraceSummaries`, keyed by `(max span startTime, traceID)`, and it is what the opaque `page_token` wraps. The **intra-trace span cursor** paginates *spans within one trace* in `multiRead` during the fetch phase, keyed by `(startTime, spanID)` (§1.3), and it is an internal detail of assembling one trace, never surfaced to the client. Find-then-fetch keeps them apart by phase: the result cursor selects which page of trace IDs to fetch; the span cursor then loads each of those traces fully. The `page_token` encodes only the result cursor.

The span cursor stays internal by choice, not because exposing it would be hard — the loop that exhausts it could yield instead. §2 gives the reasoning: a page of spans is not an independently usable answer the way a page of traces is, and the real need behind large traces is semantic reduction rather than a temporal prefix.

### 7.4 Cursor encoding on ES/OS

The ES/OS reader mints the opaque token from the `search_after` values of the last collapsed hit on the page — the `(startTime, traceID)` pair — together with the query fingerprint and backend tag of §3.2. On continuation it decodes the pair back into the `search_after` clause. Because `traceID` is a sortable keyword in the ES mapping (it already backs the term queries the reader builds) and `startTime` is a numeric field, both are valid `search_after` values with no mapping change. The collapse field `traceID` is likewise already a keyword, so collapse needs no new mapping either — only a new `collapse` clause in the query builder.

### 7.5 Cost and correctness notes

Collapse is evaluated per shard and merged, so distinctness and ordering are global, not per-shard — this is the property the terms aggregation lacked. The per-page cost is bounded and independent of page depth (§7.2, footnote 3). One honest caveat: with collapse the exact **total** number of distinct matching traces is not cheaply known, which is fine because §2 makes total counts a non-goal; if an approximate total is ever wanted, `cardinality(traceID)` can supply an estimate without affecting paging.

### 7.6 OpenSearch

OpenSearch forked from Elasticsearch 7.10, where field collapsing, `search_after`, and the composite aggregation all already existed, so the recommended scheme runs unchanged on both. The reader targets the common subset and needs no engine-specific branch. The one thing to keep an eye on is per-version defaults for `index.max_result_window`, which the `search_after` approach deliberately does not depend on, so a lowered window on either engine does not regress paging.

---

## 8. Relationship to prior work

**RFC 0005 (structured query filters)** mapped a query-complexity ladder and deferred result shaping — projection, ordering, and paging — to its **L3 tier**, explicitly noting that L3's ordering and paging are *not* inert the way projection is, and are a natural extension of the filter model rather than a conflict (RFC 0005 §4). This RFC delivers the paging piece of that deferred L3 tier. It composes with RFC 0005 cleanly: pagination orders and slices whatever set the filter matched, and the `page_token`'s query fingerprint (§3.2) covers the `filter` expression along with the other matching parameters, so a token is bound to its full query including its filter.

**ADR-013 (storage capability declaration)** is the mechanism the degradation path rides on (§6). Pagination becomes the third capability the `SearchCapabilities` shape carries — after `WithoutServiceName` and RFC 0005's filter capabilities — and it exercises exactly the properties ADR-013 designed for: a zero value that means least-capable, a declaration that crosses the remote boundary via the `Capabilities` service, and central enforcement in the query service. ADR-013's own text names "whether `SearchDepth` is an exact limit or a hint" as a next candidate capability; the `Paginated` flag is the concrete answer, since a backend that declares it is exactly one whose result bound is now exact and resumable.

**RFC 0011 (trace summary API)** provides the response type the UI's results list is built from. `FindTraceSummaries` is the primary paginated surface for the UI (§4), so "load more" in search results is a summary continuation, and `next_page_token` lands on `FindTraceSummariesResponse` alongside the summaries RFC 0011 defined.

---

## 9. Backward compatibility and degradation

The new fields default to empty and are purely additive at all three layers, so a client that never sends a `Pagination` message is byte-for-byte unaffected (G5): with no message it gets one page, and `page_size` falling back to `search_depth` preserves the current page size and default. No previously valid request changes meaning.

Degradation is honest, never silent (G6, §6.2): a backend that cannot paginate declares so, the query service serves a single capped page and reports the truncation through the capability channel the UI already reads, and a stale or forged token presented to such a backend is rejected rather than answered. A remote plugin predating the capability field reads as least-capable and is never sent a token, so the pre-`page_token` under-answering failure of §6.1 cannot occur.

The dead `Offset` field (`http_handler.go`, §1.1) is removed as part of this work. It was never read, it encodes the offset model this RFC rejects (§3, §10), and leaving it beside a real `page_token` would invite the wrong mental model. Its removal is safe precisely because nothing consumes it.

---

## 10. Considered alternatives

The pagination-model comparison (offset vs. keyset vs. opaque token) is the §3 matrix and resolves to the opaque keyset token; the ES/OS mechanism comparison (terms agg vs. composite agg vs. collapse) is the §7.2 matrix and resolves to collapse + `search_after`. Three further alternatives were weighed and set aside:

- **Elasticsearch Point-in-Time (PIT) for snapshot isolation.** A PIT would freeze the index view across a paging session, giving a consistent traversal even under writes. Rejected: a PIT is *stateful* — it holds a server-side handle with a keep-alive and resource cost, must be explicitly closed, and pins the session to one coordinating context — which contradicts the stateless-token operational win of §3.1 and the snapshot-isolation non-goal of §2. The keyset guarantee of §3.4 is weaker than a PIT — it does not stop a late-arriving trace from being missed by one traversal — but §3.4 and §3.5 argue that gap is rare and self-correcting, which does not justify a stateful handle per paging session.
- **Resurrecting the `Offset` field for offset paging.** The dead field could have been wired through as an offset. Rejected for every reason in the §3 matrix — deep-paging wall, instability under writes, and outright non-implementability over the ES/OS aggregation path — and the field is instead deleted (§9).
- **Ordering by an index-time field instead of span time.** Sorting on when a document was indexed, rather than on its span `startTime`, would make the ordering append-only: new data would always land at the front of the order, no trace's key would ever move, and a forward traversal could not skip anything (§3.4). Rejected on two counts. It changes the user-visible order from "the most recent traces" to "the most recently ingested traces," which is not what a user scanning search results means and diverges from the ordering the current `max(startTime)` aggregation already provides (§7.1). And there is no field to sort on: span documents carry only span-derived timestamps (`startTime` and `startTimeMillis` in `dbmodel.Span`), so this would need a new mapped field written on every ingest, paid on the write path forever, to remove a display artifact §3.4 shows is rare and self-correcting.

---

## 11. Implementation roadmap

PR-sized milestones with explicit exit bars, grouped by layer, bottom-up so each rests on the one before. The proto and interface stages are additive and change no behavior; the ES/OS stage is where the correctness fix and the user-visible capability land.

**M1 — Proto foundation (jaeger-idl).** Add the `Pagination` message and a `pagination` field on `TraceQueryParameters` in both api_v3 and storage/v2; add `next_page_token` to `FindTraceIDsResponse` and `FindTraceSummariesResponse`; add the `paginated` field to `storage.v2.SearchCapabilities`. Legacy fields untouched; field numbers coordinated with RFC 0005. *Exit:* generated types compile and vendor cleanly; existing api_v3/storage callers byte-for-byte unaffected.

**M2 — Internal interface and query-service plumbing.** Extend `TraceQueryParams` with the nested `Pagination` struct (§5); change `FindTraceIDs`/`FindTraceSummaries` to yield the token on their result page (§5); add `Paginated` to `SearchCapabilities`; implement token minting, fingerprint binding (§3.2), and the degradation rules (§6.2) centrally in the query service. No backend paginates yet — every reader declares `Paginated = false`, so the query service serves one capped page and reports it. *Exit:* a request with no token returns today's results; a token against a non-paginating backend is rejected; the capability is reported to the UI.

**M3 — Elasticsearch/OpenSearch.** Add a `collapse` clause to `esquery`; replace the terms aggregation in `FindTraceIDs` with the collapse + `search_after` scheme (§7); mint/decode the result cursor (§7.4); keep the intra-trace span cursor separate (§7.3); declare `Paginated = true`. This also fixes the cross-shard ordering approximation of §1.2. *Exit:* start-to-finish paging over a fixed dataset visits every matching trace exactly once in most-recent-first order; a shard-count-varying integration test asserts completeness; a test writes a late span that raises an already-returned trace's maximum `startTime` mid-traversal and asserts the trace is not returned a second time (§3.4); unqualified single-page results match today's within the ordering fix.

**M4 — Remote Storage gRPC.** Carry the fields and the capability across the `jaeger.storage.v2` boundary; the remote-storage reader forwards `Paginated` from the backend it fronts and relays the token; a plugin predating the field reads as least-capable (§6.1). *Exit:* a remote ES/OS backend paginates over gRPC identically to in-process; a legacy plugin serves one page and is never sent a token.

**M5 — UI "load more".** The search-results list consumes `next_page_token` from `FindTraceSummaries` and offers "load more," graying out or disabling continuation when the backend declares no pagination (reusing the capability the UI already reads). Token handling follows the client contract of §3.5: push the token just received, pop on backward navigation, and discard remembered tokens below any page that is re-fetched. *Exit:* results paginate in the UI on paginating backends; non-paginating backends show a single page with an honest truncation indicator.

**Out of scope (future, this design enables):**
- ClickHouse declaring `Paginated = true` — a natural fit (`WHERE (startTime, traceID) < cursor ORDER BY … LIMIT size`).
- Cassandra declaring `Paginated = true`, which is harder than it looks. The CQL driver hands out a `PageState` blob that resumes a query where it stopped, so the obvious move is to wrap that in the token. It does not work here, for two reasons established in [#8961](https://github.com/jaegertracing/jaeger/pull/8961). A `PageState` is bound to one CQL statement against one schema version, so it is not a value the query service can safely mint, hand out, and honour later the way §3.1 requires of a stateless token. More fundamentally, a Cassandra search is not one statement: `duration_index` partitions on `(service_name, operation_name, bucket)` where `bucket` is the span's start time rounded to the hour, so a query spanning a time range fans out into one CQL query per hour bucket and merges the results (`queryByDuration`, `internal/storage/v1/cassandra/spanstore/reader.go`). `PageState` is per statement, not per logical result set, so there is no single blob that describes where the merged traversal stopped. The tractable alternative is to ignore `PageState` and give Cassandra the same keyset cursor everything else uses — carry `(startTime, traceID)` in the token and resume by setting `StartTimeMax` to the last key — which is implementable today but delivers the weaker per-bucket ordering the clustering order gives (`duration DESC, start_time DESC`), not the global recency order §3.3 requires. Either path is its own project; Cassandra declares `Paginated = false` until one is done.
- Badger declaring `Paginated = true` — its iterator supports `Seek` to a known key, so a keyset cursor is implementable, but the key encodes start time rather than the full set of query predicates, so the same ordering problem applies.
- A configurable sort key (order by duration, by error count) — the opaque token (§3) already hides the sort-key shape, so this extends without a wire break.
- An approximate total-count estimate via `cardinality(traceID)` (§7.5), if the UI ever wants "~N results."
- One document per trace, upserted as spans arrive, carrying the trace's minimum start time, maximum end time, span counts and services. It would let the search sort and page on the same trace-level values the results list displays, retiring the max-versus-min discrepancy of §7.1, and it would serve RFC 0011's summaries directly instead of reconstructing them by aggregation. The cost is a new write path with an upsert per span batch, which is why it is a separate project rather than a step here.

---

## 12. Open questions

1. **Default page size.** Should `Pagination.page_size`'s default track `search_depth`'s current default of 100, or should interactive search default to a smaller screenful (e.g. 20) with `search_depth` as the traversal ceiling? The compatibility argument (§4) favors 100; the interactive-UX argument favors smaller.
2. **A default traversal ceiling.** Should the query service cap the number of pages a single cursor lineage yields by default (so unbounded "load more" cannot walk an entire index), and if so, at what multiple of `search_depth`? Or is an unbounded traversal acceptable and rate-limiting left to deployment?
3. **Capability granularity.** Is a single `Paginated bool` enough, or should it be an enum from the start to leave room for a future best-effort mode that cannot promise completeness (§6.1)? A bool now, graduating to an enum on demand, matches ADR-013's stated evolution path, but naming it right the first time avoids a later rename.
4. **Sort-key semantics: max or min span `startTime`?** Search has ranked traces by their maximum span `startTime` since 2017 with no recorded rationale, while the results list labels each trace with its `MinStartTime`, so a long-running trace can outrank one that started later (§7.1). Adopting collapse makes that ordering permanent, since collapse cannot rank by a per-group minimum (§7.2). Is the discrepancy worth fixing — and if so, is the answer to keep the terms aggregation for ranking, to build the per-trace document of §11, or to accept max and document it as the definition? The choice also decides whether concurrent writes cause skips or duplicates (§3.4).
5. **Fingerprint scope.** Exactly which request fields enter the token's query fingerprint (§3.2)? Clearly the matching parameters; `raw_traces` and `page_size` are arguably presentation, not matching, so a client could legitimately change `page_size` mid-session — should that invalidate the token or not?
