# RFC 0011: Trace Summary API for Lightweight Search Results

- **Status:** Implemented — graduated into [ADR-010](../adr/010-trace-summary-api.md)
- **Author:** Yuri Shkuro
- **Created:** 2026-05-21
- **Last Updated:** 2026-07-25
- **Delivered by:** [#8604](https://github.com/jaegertracing/jaeger/pull/8604), [#8618](https://github.com/jaegertracing/jaeger/pull/8618), [#8633](https://github.com/jaegertracing/jaeger/pull/8633), [#8634](https://github.com/jaegertracing/jaeger/pull/8634), [#8645](https://github.com/jaegertracing/jaeger/pull/8645), [#8812](https://github.com/jaegertracing/jaeger/pull/8812), and the `jaeger-idl` / `jaeger-ui` PRs listed per milestone below
- **Provenance:** Extracted from [ADR-010](../adr/010-trace-summary-api.md) on 2026-07-25. It was originally written as that ADR in [#8602](https://github.com/jaegertracing/jaeger/pull/8602), before `docs/rfc/` existed, and is a proposal with a milestone tracker rather than a decision record. The analysis below is preserved as written in May 2026 — including its present tense, which describes the code as it stood then. ADR-010 records the API as it stands today.

---

## Abstract

The Jaeger UI search screen fetches full trace data — every span with every attribute — only to render a handful of summary fields per result row, so a search over traces with hundreds of spans transfers tens of kilobytes per trace where a few dozen bytes would do. This RFC proposes a `FindTraceSummaries` endpoint in API v3 with a matching `TraceSummary` data model, carried through the Go storage interface and the remote storage gRPC API, so backends that can aggregate server-side do, and those that cannot fall back to computing summaries from full traces.

## 1. Motivation
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

This RFC proposes a new **`FindTraceSummaries`** endpoint in the Jaeger API v3, a
matching **`TraceSummary`** data model, the propagation of that method through the Go
`tracestore.Reader` interface and the Remote Storage gRPC API, and a fallback path for
storage backends that do not implement native summary computation.

### Affected Repositories

| Repository | Role |
|---|---|
| `jaegertracing/jaeger-idl` | Proto definitions (`api_v3`, `storage/v2`) |
| `jaegertracing/jaeger` | Backend: gRPC/HTTP handler, QueryService, storage interface, adapters |
| `jaegertracing/jaeger-ui` | UI: search screen, API client |

## 2. Alternatives Considered

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

> **Delivered shape:** the optional-interface approach recommended above was later reversed in favour of Alternative D. [#9067](https://github.com/jaegertracing/jaeger/pull/9067) deleted `tracestore.SummaryReader` and moved `FindTraceSummaries` onto `tracestore.Reader`, with an embeddable `UnsupportedTraceSummaries` mixin supplying the fallback signal — the "default implementation in a base struct" this section dismissed. The reason was a composition tax this analysis did not anticipate: every decorator wrapping a `Reader` had to re-detect and re-expose the optional interface or silently drop the capability. See [ADR-010](../adr/010-trace-summary-api.md).

## 3. Implementation Milestones
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

> **Status: ✅ Complete** ([#8812](https://github.com/jaegertracing/jaeger/pull/8812), Elasticsearch/OpenSearch)

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

### Remaining Work — Suggested PR Sequence

A concise breakdown for contributors picking up Milestones 3–5. Each PR is
independently reviewable and leaves `main` in a working state.

| # | Repo | Description | Notes |
|---|------|-------------|-------|
| ✅ A | `jaeger/` | Bump `idl/` submodule to `jaeger-idl` main (`0daa719`); regenerate Go bindings; fix any compilation errors from the renamed `FindTraceIDsRequest` | [#8634](https://github.com/jaegertracing/jaeger/pull/8634) |
| ✅ B | `jaeger/` | Implement the gRPC handler for `FindTraceSummaries` (`apiv3/grpc_handler.go`) | [#8634](https://github.com/jaegertracing/jaeger/pull/8634) |
| ✅ C | `jaeger/` | Replace hand-written JSON scaffold types in the HTTP gateway with `api_v3.FindTraceSummariesResponse` + `gogoproto/jsonpb`; delete `summaries.go` | [#8645](https://github.com/jaegertracing/jaeger/pull/8645) |
| ✅ D | `jaeger/` | Implement `SummaryReader` in the gRPC remote storage adapter (`internal/storage/v2/grpc/`) — server forwards to underlying `SummaryReader`; client is a plain iterator that yields `errors.ErrUnsupported` when the server returns `UNIMPLEMENTED` | Milestone 4 |
| ✅ G | `jaeger/` | Native `SummaryReader` in one storage backend (Elasticsearch or ClickHouse) | [#8812](https://github.com/jaegertracing/jaeger/pull/8812) |

---

## 4. References

* `idl/proto/api_v3/query_service.proto` – existing v3 query service proto
* `idl/proto/storage/v2/trace_storage.proto` – remote storage gRPC proto
* `internal/storage/v2/api/tracestore/reader.go` – Go `Reader` interface
* `cmd/jaeger/internal/extension/jaegerquery/querysvc/service.go` – QueryService
* `cmd/jaeger/internal/extension/jaegerquery/internal/apiv3/` – gRPC and HTTP v3 handlers
* [ADR-010](../adr/010-trace-summary-api.md) – the API as it stands today, including where the delivered design departed from this proposal
