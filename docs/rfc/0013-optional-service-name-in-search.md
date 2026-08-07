# RFC 0013: Optional Service Name in Trace Search

- **Status:** Partially Implemented
- **Author:** Yuri Shkuro
- **Created:** 2026-08-07
- **Last Updated:** 2026-08-07
- **Issues:** [jaeger#423](https://github.com/jaegertracing/jaeger/issues/423) (backend), [jaeger-ui#180](https://github.com/jaegertracing/jaeger-ui/issues/180) (UI), [jaeger#8492](https://github.com/jaegertracing/jaeger/issues/8492) (MCP `search_traces`)

---

## Abstract

Jaeger's search screen cannot run a query until a service is selected, so a user who knows a tag but not which service emits it — `http.status_code=500`, a transaction id, `error=true` — has no way to ask the question without searching every service in turn. The requirement comes from Cassandra, whose manually maintained indices are all keyed by service name; it is not inherent to the search contract, and Elasticsearch/OpenSearch, ClickHouse, and the in-memory store already answer service-less queries at the storage layer today. This RFC proposes that a storage backend declare whether it can search without a service name, that the query service carry that declaration to the UI through the existing `backendCapabilities` blob, and that the UI offer an "All Services" option in the Service dropdown only when the backend says yes. Elasticsearch/OpenSearch is the primary target; Cassandra declares the capability unsupported and its behavior is unchanged.

## 1. Motivation

The ask is nine years old and duplicated across both repositories ([jaeger#423](https://github.com/jaegertracing/jaeger/issues/423), [jaeger-ui#180](https://github.com/jaegertracing/jaeger-ui/issues/180)), with users repeatedly citing the same two workflows: find traces carrying a tag whose emitting service is unknown, and find error traces across a whole deployment. Zipkin supports tag search without a service, and at least one reported migration from Zipkin to Jaeger stalled on the gap. The MCP `search_traces` tool has the same problem in a sharper form: an agent asked "show me HTTP 500s in the last 10 minutes" must call `get_services` and then fan out one search per service, spending context and latency on work the backend could do in one query.

Two things make this the right time. First, the search path the UI uses has already moved to API v3 (`GET /api/v3/trace-summaries`), whose query parser requires only a time range and sends `query.serviceName` only when the caller sets it, so the API layer no longer stands in the way. Second, the capability channel the design needs already exists: the query service publishes a `backendCapabilities` object into the UI bundle, and the UI reads it through `getConfig()`.

What blocks the feature is therefore narrow: no backend declares what it can do, the UI hard-requires a service before it will submit, and two non-v3 callers still reject an empty service name.

## 2. Current State

Support for an empty `ServiceName` in `tracestore.TraceQueryParams` is already uneven, and the unevenness is undocumented — nothing in the `Reader.FindTraces` contract says whether an empty service name is a valid query, a cross-service query, or an error.

| Backend | Behavior today with an empty `ServiceName` | Where |
| --- | --- | --- |
| Elasticsearch / OpenSearch | 🟢 Works. The query builder adds the `process.serviceName` clause only when the name is non-empty, and tag clauses do not reference the service. One vestigial guard rejects the combination of an empty service name *with* attributes, which the tag query itself does not need. | `internal/storage/v2/elasticsearch/tracestore/core/reader.go:392` (`validateQuery`), `:519` (conditional service clause), `:530` (tag clauses) |
| In-memory | 🟢 Works. The span matcher treats an empty service name as "match any". | `internal/storage/v2/memory/tenant.go:237` |
| ClickHouse | 🟢 Works. The `WHERE` clause for service is appended only when the name is non-empty, and no validation requires it. | `internal/storage/v2/clickhouse/tracestore/query_builder.go:139` |
| Badger | 🟡 Partial. With no service name it falls back to a full time-range scan, but rejects an empty service name combined with tags or an operation name, because those filters are only applied through service-keyed index seeks. | `internal/storage/v1/badger/spanstore/reader.go:502` (`validateQuery`), `:498` (`scanTimeRange`) |
| Cassandra | 🔴 Silently wrong. Every index is keyed by service name, so a query with no service name falls through to `queryByService` with an empty partition key and returns zero results rather than an error. | `internal/storage/v1/cassandra/spanstore/reader.go:217` (`validateQuery`), `:313` (`queryByService`) |
| gRPC remote storage | ⚪ Unknowable. The reader forwards `ServiceName` to the remote `TraceReader` service; whether the query succeeds depends entirely on the backend behind it. | `internal/storage/v2/grpc/tracereader.go:84` |

The callers are similarly split. API v3, both HTTP and gRPC, requires only `startTimeMin`/`startTimeMax` (`internal/.../apiv3/query_parser.go:87`, `apiv3/grpc_handler.go:91`). The legacy `/api/traces` parser rejects a query with neither trace IDs nor a service name (`internal/.../query_parser.go:362`). The MCP `search_traces` handler rejects an empty `service_name` outright (`internal/.../mcptools/internal/handlers/search_traces.go:137`).

On the UI side, `SearchForm.tsx` seeds the Service field with the sentinel `'-'`, derives `noSelectedService` from it, and uses that to disable the Find Traces button; the operation dropdown is driven by `useSpanNames(service)`, which is skipped while no service is selected. The client sends `query.serviceName` only when the form supplied one (`api/v3/client.ts:63`), so an "all services" search needs no change to request construction.

## 3. Design

### 3.1 Declaring the capability

The UI must know whether service-less search is possible *before* the user submits anything, because the answer determines whether the "All Services" option is offered at all. That rules out learning it from the outcome of a query.

| Criterion | **A. New method on `tracestore.Reader`** (recommended) | B. Optional interface, type-asserted by the caller | C. Runtime negotiation via `ErrUnsupported` | D. Operator config flag on jaeger-query |
| --- | --- | --- | --- | --- |
| Answer available before a query runs | 🟢 | 🟢 | 🔴 | 🟢 |
| Backend is the authority on its own abilities | 🟢 | 🟢 | 🟢 | 🔴 operator must know |
| Survives wrapping | 🟢 compiler rejects a decorator that does not forward | 🔴 a wrapper silently downgrades the capability | 🟢 | 🟢 |
| No storage-specific knowledge in the query layer | 🟢 | 🟢 | 🟢 | 🟢 |
| Works for gRPC remote storage | 🟡 config-backed answer¹ | 🟡 config-backed answer¹ | 🟢 | 🟢 |
| Follows an existing pattern in the codebase | 🟢 `FindTraceSummaries` | 🟢 `SyncBulkWriteConfig` | 🟢 `UnsupportedTraceSummaries` | 🟡 |
| Cost of adding it | 🟡 every Reader and decorator must answer² | 🟢 only the backends that opt in | 🟢 | 🟢 |

🟢 good · 🟡 partial · 🔴 poor
¹ The remote backend's abilities cannot be introspected over the current `jaeger.storage.v2` API, so the gRPC reader answers from its own configuration until the IDL grows a capability RPC (§3.6).
² Around eight implementations plus the generated mocks, each stating in place why that backend requires the field — the same blast radius `FindTraceSummaries` had, and the compiler enumerates it.

**Recommendation: A, with C retained as the enforcement backstop.** The declaration is what the UI needs; `ErrUnsupported` from the reader is what protects a direct API caller that ignores the declaration, and the two do not conflict.

B is the cheaper change and the wrong trade. `tracestore.Reader` implementations are wrapped routinely — the metrics decorator wraps every reader a factory builds, and `queryinterceptor.NewReaderDecorator` wraps that again — and a type assertion against a wrapper silently answers "no capability" instead of failing. Declaring the capability on the factory rather than the reader only moves the hazard: lazy factory initialization (ADR-003) would wrap factories the same way. A method on the interface makes a decorator that forgets to forward a compile error, which is the only version of this that stays correct as wrappers accumulate.

The shape follows `FindTraceSummaries`: a method on `Reader`, returning a struct.

```go
// in internal/storage/v2/api/tracestore, beside the Reader it belongs to

// SearchCapabilities describes how a Reader's search methods behave where backends
// differ: which TraceQueryParams fields may be omitted, which are honored exactly
// rather than approximated, and which combinations a backend cannot serve. Its zero
// value is the least capable reader.
type SearchCapabilities struct {
    WithoutServiceName bool
}

// on Reader, in the shape its other methods take
SearchCapabilities(ctx context.Context) (SearchCapabilities, error)
```

The method is fallible and takes a context, like every other method on `Reader`, because "declares nothing" and "cannot say" are different answers and a reader that conflates them forces its callers to trust a fabricated value. A reader that knows returns the zero value with a nil error; one whose backend sits behind an API that cannot be asked returns `errors.ErrUnsupported`, and the query service treats that as no capabilities while logging why. The context is there because the answer may arrive over the wire — both §3.6 and §3.7 propose exactly that.

A struct return rather than one boolean method per capability, because the divergences worth reporting go well beyond this one and are not all of the same shape. Two that exist today: `SearchDepth` is an exact limit on some backends and a hint on others — the API contract already warns that "some implementations might not support precise limits" — and Cassandra cannot intersect a duration filter with tags, since it serves durations from a separate `duration_index` whose partition key does not compose ([ADR-001](../adr/001-cassandra-find-traces-duration.md), [#1047](https://github.com/jaegertracing/jaeger/issues/1047)). Each of those becomes a field that every existing implementation inherits as `false`, rather than an interface method all of them must grow.

Each reader implements the method itself, including the ones that declare nothing — no embeddable default. An embeddable that returns the zero value would have to be named either for one field, which misdescribes what embedding it declares, or for the whole set, which reads as an assertion about capabilities the backend was never assessed for. Three lines per reader, stating in place why that backend requires the field, is worth more than the line the embed would save.

From there the value travels the path `archiveStorage` already travels:

```
Reader.SearchCapabilities().WithoutServiceName                       // asked after wrapping
  → querysvc.StorageCapabilities{SearchWithoutServiceName bool}      // service.go:38
  → internal.BackendCapabilities{searchWithoutServiceName: bool}     // static_handler.go:44
  → JAEGER_BACKEND_CAPABILITIES in index.html
  → getConfig().backendCapabilities.searchWithoutServiceName         // UI
```

`jaegerquery`'s `server.Start` asks the reader it is about to hand to the query service — after the metrics and interceptor decorators are in place — so what reaches the UI is whatever the backend declared through them.

`querysvc.StorageCapabilities` already carries a comment anticipating exactly this kind of extension, and the blob is additive in both directions of version skew, which is what frees the backend and UI milestones to land in either order.

Nothing validates the shape of the blob. The backend marshals `BackendCapabilities` and search-replaces it into `index.html` as `JAEGER_BACKEND_CAPABILITIES = {…};` (`static_handler.go:174`), and the UI folds it in with a plain object spread — `{ ...defaultConfig.backendCapabilities, ...getBackendCapabilities() }` (`get-config.ts`). A key the UI does not know lands in the config object and is never read; there is no schema, and the unknown-property warning path (`processDeprecation`) runs over the user's UI config file, not over the capability blob. The `BackendCapabilities` TypeScript type is a compile-time declaration in `jaeger-ui`, so it constrains UI code rather than the payload. A new backend serving an old UI bundle is therefore inert, not invalid.

The dependency runs the other way, and only within the UI: before the UI *reads* the flag it must carry the key in `defaultConfig.backendCapabilities` with the value `false`, so that an older backend which injects nothing leaves the option hidden rather than driven by `undefined`. That default and its first reader belong in the same UI change (Milestone 4).

### 3.2 Naming

The capability is named for what it permits: `SearchWithoutServiceName` in Go, `searchWithoutServiceName` in the JSON blob. [jaeger#423](https://github.com/jaegertracing/jaeger/issues/423)'s plan calls it "multi-service search", which reads as a query for traces *spanning* several services — a different feature. "Cross-service search" has the same ambiguity. The chosen name says which field becomes optional and nothing more.

### 3.3 Where the requirement is enforced

Today the requirement is asserted in three places with three behaviors: an error from the legacy parser, an error from the MCP handler, and silence from Cassandra. The proposal collapses this to one enforcement point plus one backstop.

The **query service** (`querysvc.QueryService`) gains the check: when a query arrives with an empty `ServiceName` and the storage factory did not declare `SearchWithoutServiceName`, it fails with a typed error that the API layers map to `InvalidArgument` / HTTP 400 and whose message names the backend's limitation rather than the field. This puts the rule where the capability is already known, and gives every caller — API v3, the legacy handler, MCP, and any future one — the same behavior for free.

The **storage backends** keep their own `validateQuery` guards as the last line of defense, with two changes. Elasticsearch/OpenSearch drops the vestigial `ServiceName == "" && attributes > 0` rejection, since its tag clauses never referenced the service. Cassandra replaces its silent empty result with `errors.ErrUnsupported`, so a caller that reaches it directly gets a diagnosis instead of a wrong answer — this is the `UnsupportedTraceSummaries` precedent applied to a second dimension of the same interface.

The two callers that carry their own requirement lose it: `errServiceParameterRequired` comes out of the legacy `/api/traces` parser, and the MCP `search_traces` handler stops rejecting an empty `service_name`, with its tool schema describing the field as optional and noting that omitting it searches every service. That is the substance of [jaeger#8492](https://github.com/jaegertracing/jaeger/issues/8492), which was closed because it asserted a storage-wide capability that does not exist; it becomes true for the backends that declare it.

### 3.4 Per-backend support

| Backend | Declares `SearchWithoutServiceName` | Work required |
| --- | --- | --- |
| Elasticsearch / OpenSearch | `true` | Remove the vestigial attribute guard; integration test in the existing ES/OS matrix |
| In-memory | `true` | None beyond the declaration and a test |
| ClickHouse | `true` | None beyond the declaration and a test; the search SQL appends every predicate conditionally |
| Badger | `false` initially | A truthful `true` requires applying tag and operation filters during the time-range scan; until then the honest answer is `false` |
| Cassandra | `false` | Replace the silent empty result with `ErrUnsupported` |
| gRPC remote storage | Cannot answer (`ErrUnsupported`) | New config field so an operator can answer for it (§3.6) |

Badger is the case that shows why the capability must stay a single honest boolean rather than a partial one. Badger can scan a time range without a service name, but it cannot apply tags or an operation name that way, so declaring `true` would advertise a search the UI offers with filters Badger would silently drop. Reporting `false` until the scan path filters properly is the smaller lie — none.

### 3.5 UI behavior

The Service dropdown gains a reserved "All Services" entry, rendered only when `backendCapabilities.searchWithoutServiceName` is true, carrying a reserved sentinel value distinct from the existing `'-'` placeholder. Selecting it makes `noSelectedService` false, so Find Traces enables; the search-query builder omits `query.serviceName`, which `api/v3/client.ts` already handles by not setting the parameter.

| Criterion | **Reserved sentinel in the existing Select** (recommended) | Separate "search all services" checkbox | Clearable/empty Service field |
| --- | --- | --- | --- |
| Discoverable where users already look | 🟢 | 🟡 | 🔴 empty reads as "not yet chosen" |
| Round-trips through the search URL unchanged | 🟢 `?service=<sentinel>` | 🟡 new param | 🔴 ambiguous with absent param |
| Distinguishable from "nothing selected yet" | 🟢 | 🟢 | 🔴 |
| No collision with a real service name | 🟡 reserved value² | 🟢 | 🟢 |

² A service whose name equals the sentinel would be shadowed. The sentinel should be chosen to make that implausible and the choice documented next to the existing `'-'` placeholder, which already reserves a plausible service name.

Two consequences follow from operations being per-service. The operation dropdown is disabled in All Services mode, because `GetOperations` takes a service name and there is no API to enumerate operations across services; the field is cleared rather than silently ignored. Everything else on the form — tags, duration bounds, lookback, limit — stays available, since those filters are service-independent in every backend that declares the capability.

The stored `lastSearch` and the URL both carry the sentinel like any other service value, so a shared "all services" search link reproduces the search. When a UI holding a sentinel-valued URL loads against a backend that does not declare the capability, the form falls back to the `'-'` placeholder rather than submitting a query the backend will reject.

### 3.6 gRPC remote storage

A remote backend's abilities are invisible to Jaeger: `jaeger.storage.v2`'s `TraceReader` service has no capability RPC, and `FindTracesRequest` carries no hint about which fields are optional. Two ways forward, and they compose:

Near term, the reader reports `ErrUnsupported` — it genuinely cannot tell — and the gRPC storage backend gains a field such as `search_without_service_name: true` for an operator who can, defaulting to unset. The operator running a remote backend knows whether it can serve the query; the default keeps every existing deployment behaving exactly as it does today.

Longer term, a `Capabilities` RPC on `TraceReader` in `jaeger-idl` would let the remote backend answer for itself, at which point the config field becomes an override for backends that predate the RPC. That is a separate proposal against `jaeger-idl` and is deliberately not a prerequisite here.

### 3.7 Reporting capabilities to API clients

The UI learns the capability, but no other client can: the query service publishes it only by search-replacing a blob into `index.html`, so an API consumer — an SDK, a script, or Jaeger's own e2e test client, which drives the suite through `api_v3.QueryServiceClient` — has no way to ask what the deployment supports. That gap has teeth beyond convenience. A capability-gated integration test that consulted the e2e client's own `SearchCapabilities` would skip on every backend, including the ones that support the query, and a silently-skipped test reads exactly like a passing one.

Two things follow, on different timescales. Until the API exists, an integration test gates on the suite's own per-backend `integration/capabilities.Capabilities`, which CI populates from the `STORAGE` under test — the test harness knows what it started, even though the client cannot ask. And api_v3 should gain real capability discovery, which is worth designing rather than bolting onto the trace API:

| Criterion | **A. Separate `Capabilities` gRPC service** (recommended) | B. New RPC on `QueryService` | C. HTTP endpoint only (`/api/v3/capabilities`) | D. Keep the `index.html` blob as the only source |
| --- | --- | --- | --- | --- |
| Serves non-UI clients (SDKs, scripts, e2e suite) | 🟢 | 🟢 | 🟢 | 🔴 |
| Separates deployment metadata from the trace-reading contract | 🟢 | 🔴 mixes concerns into the query API | 🟡 | 🟢 |
| One authoritative source the UI can also consume | 🟢 | 🟢 | 🟢 | 🟡 build-time blob, not queried |
| Version negotiation for older backends | 🟢 `Unimplemented` on the whole service | 🟢 `Unimplemented` on the method | 🟡 404 | ⚪ |
| Can grow beyond search (archive, metrics, AI) | 🟢 the natural home | 🟡 | 🟡 | 🟡 |
| Cost | 🟡 new `jaeger-idl` service plus gateway wiring | 🟢 smaller IDL change | 🟢 | 🟢 |

🟢 good · 🟡 partial · 🔴 poor · ⚪ not applicable

**Recommendation: A.** Until it exists, Jaeger's own e2e client returns `errors.ErrUnsupported` from `SearchCapabilities` — the honest answer for a client that cannot ask — and the storage interface's fallible signature is what lets it say so. What a deployment can do is not a property of trace reading, and the same endpoint should be able to report the archive, metrics and AI capabilities the UI blob already carries — which makes a service of its own the right home rather than a method bolted onto `QueryService`. The gRPC gateway gives the HTTP form of option C for free. A client that gets `Unimplemented` treats the deployment as declaring nothing, which is the same conservative default the storage layer uses.

This is a `jaeger-idl` change and its own milestone; §3.6's storage-side `Capabilities` RPC is the mirror of it one layer down, and the two are independent.

### 3.8 Cost and safety

A service-less query on Elasticsearch/OpenSearch is a `bool` query with the same time-range clause and one fewer `must` term, so it touches the same indices the time range already selects and does not fan out further. It does match more documents, and the cost of retrieving them is bounded by the existing `SearchDepth` cap that every search already carries. No new limit is proposed: the query is not structurally more expensive than a search over a very large single service, which Jaeger already permits.

Two risks are worth naming rather than mitigating in code. Users can now aim a wide-open query at a long lookback, and operators who need to prevent that have the existing `max_lookback`-style controls and the query-interceptor extension point. And a backend that declares the capability but performs poorly at it — Badger's scan path, were it to graduate — would degrade interactively rather than fail, which is the reason the declaration is per-backend rather than global.

## 4. Alternatives Considered

**Fan out over services in the query service.** Call `GetServices` and issue one `FindTraces` per service, merging results — the approach Zipkin's Cassandra support took, and the workaround agents perform today. It makes the feature universal without touching any backend, but it multiplies one user action by the service count, which is exactly the query pattern [jaeger#423](https://github.com/jaegertracing/jaeger/issues/423) rejected for large deployments, and it would be the *slowest* path on the backends that can answer natively. Rejected as a default. It remains a plausible opt-in mode for small Cassandra deployments, and this RFC leaves room for it: a Cassandra factory in that mode would declare `SearchWithoutServiceName` true, and nothing above needs to change.

**Fan out in the UI.** Same shape, worse: it also multiplies HTTP round trips and re-implements result merging and ranking in the browser.

**Probe with a query and fall back.** Submit the search, catch `ErrUnsupported`, and only then tell the user a service is required. The dropdown cannot be built from an answer that arrives after submission, and the failure surfaces as an error after the user has composed a query rather than as an unavailable option before.

**Make `ServiceName` optional in the interface contract with no capability at all.** Simplest change, and wrong: it hands Cassandra users a search that silently returns nothing, which is worse than the current explicit refusal.

## 5. Implementation Plan

Each milestone is independently shippable and leaves the product in a working state. The UI change lands only after a backend can advertise the capability.

**Milestone 1 — Capability declaration, backend side.** ✅ [#9256](https://github.com/jaegertracing/jaeger/pull/9256) Add `SearchCapabilities` to `tracestore.Reader`, implement it on the Elasticsearch/OpenSearch and in-memory readers, forward it through both reader decorators, and extend `querysvc.StorageCapabilities` and `internal.BackendCapabilities`. Remove the vestigial `ServiceName == "" && attributes > 0` guard from the ES/OS reader. No user-visible change yet; the blob gains a field nothing reads, which the UI ignores rather than rejects (§3.1), so this milestone does not depend on the UI work landing first.

**Milestone 2 — Enforcement at one boundary.** Move the requirement into the query service as a typed error, map it in the API v3 HTTP and gRPC layers, drop `errServiceParameterRequired` from the legacy parser, and make Cassandra return `ErrUnsupported` instead of an empty result. Integration test in the ES/OS matrix: write spans for two services, search by tag with no service name, and assert traces from both come back. The test gates on the suite's per-backend `integration/capabilities.Capabilities` — not on the e2e client's `SearchCapabilities`, which is a stub that would make the test skip everywhere (§3.7).

**Milestone 3 — MCP `search_traces`.** Make `service_name` optional in the handler and the tool schema, with the schema noting the cross-service semantics, and add the empty-service test path. Closes the substance of [jaeger#8492](https://github.com/jaegertracing/jaeger/issues/8492).

**Milestone 4 — UI.** In `jaeger-ui`: the reserved "All Services" option gated on `backendCapabilities.searchWithoutServiceName`, the operation field disabled in that mode, URL and `lastSearch` round-tripping, and the fallback for a sentinel-valued URL against a backend without the capability. Closes [jaeger-ui#180](https://github.com/jaegertracing/jaeger-ui/issues/180).

**Milestone 5 — gRPC remote storage.** Add the `search_without_service_name` config field to the gRPC storage backend, defaulting to unset, so an operator who knows the remote backend can declare on its behalf what the reader cannot introspect.

**Milestone 6 — Capability discovery in api_v3 (§3.7).** A `Capabilities` service in `jaeger-idl`, served by jaeger-query, reporting what the deployment supports; the e2e test client then reports what the query service reports instead of a stub, and the UI can read the same source it queries rather than a blob injected at boot. Independent of the milestones above and of §3.6's storage-side RPC.

**Milestone 7 — Documentation.** Document the capability in the storage backend comparison, state which backends support it, and describe the "All Services" option in the search documentation. Closes [jaeger#423](https://github.com/jaegertracing/jaeger/issues/423).

Badger is deliberately not on this list. It stays at `false`, and graduating it — by applying tag and operation filters inside the time-range scan — is a separate piece of work that this design does not block.

## 6. Open Questions

1. **Should the operation field become available in All Services mode?** Elasticsearch indexes `operationName` independently of the service, so an operation-only search is answerable there, but there is no API to enumerate operations across services to populate the dropdown. A free-text operation input, gated on a second capability, would be a follow-up rather than part of this work.
2. **Should Cassandra ship an opt-in fan-out mode?** §4 leaves the door open and the capability interface accommodates it. Whether it is worth the code depends on whether small-deployment Cassandra users actually want it.
3. **Does the sentinel need protection against a colliding service name?** The existing `'-'` placeholder has the same exposure and has never been reported as a problem, which suggests documenting the reserved value is enough.

## 7. References

- [Issue #423: Find Traces Matching Multiple Services](https://github.com/jaegertracing/jaeger/issues/423) — the backend issue, with the original implementation plan this RFC follows
- [jaeger-ui#180: Search all services](https://github.com/jaegertracing/jaeger-ui/issues/180) — the UI counterpart, open since 2018
- [Issue #8492: MCP `search_traces` requires `service_name`](https://github.com/jaegertracing/jaeger/issues/8492) — closed for overstating storage support; §3.3 makes its request true where the backend allows it
- [RFC 0011: Trace Summary API](./0011-trace-summary-api.md) — introduced `FindTraceSummaries` and the `ErrUnsupported` negotiation pattern reused here as the enforcement backstop
- [ADR-010: Trace Summary API](../adr/010-trace-summary-api.md) — the API v3 search endpoint the UI now uses, which already treats the service name as optional
