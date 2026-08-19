# ADR-013: Storage Capability Declaration

* **Status**: Implemented — graduated from [RFC 0013](../rfc/0013-optional-service-name-in-search.md)
* **Date**: 2026-08-08, extended 2026-08-16 with the [RFC 0005](../rfc/0005-structured-query-filters.md) filter capabilities

## Context

`tracestore.Reader` presents one search contract, but the backends behind it do not all honour it the same way. `TraceQueryParams` has fields some backends cannot accept, cannot honour exactly, or cannot combine — and the interface said nothing about which, so a caller could neither avoid an unanswerable query nor interpret a surprising result. Three divergences that exist today, none of them hypothetical:

* **A field that must be present.** Cassandra and Badger key every search index by service name, so a query that omits it cannot be served. Until this work Cassandra answered such a query with an empty result, which is worse than an error, because the query looked answered.
* **A combination that does not hold.** Cassandra serves `DurationMin`/`DurationMax` from a separate `duration_index` which cannot be combined with tag filters, so it rejects that combination outright ([ADR-001](./001-cassandra-find-traces-duration.md)). The API layer stopped requiring the two to be used separately in [#1047](https://github.com/jaegertracing/jaeger/issues/1047), which did not make the storage limitation go away.
* **A field honoured approximately.** `SearchDepth` is an exact limit on some backends and a hint on others — api_v3's own contract warns that "some implementations might not support precise limits" — which leaves a caller unable to tell whether a short result set means that there are no more matches or merely that this backend stopped early.

All three have the same shape. Only the backend knows which of these limits apply to it. A caller needs that answer before it builds a query, not after one has failed. And a caller that guesses wrong often gets no signal at all: an empty result set and a silently truncated one both look like correct answers. A backend therefore has to state its limits up front, because an error returned from a failed query would come too late — and in these cases would never arrive.

[RFC 0013](../rfc/0013-optional-service-name-in-search.md) hit the first of the three, analyzed the alternatives, and was delivered across milestones M1–M5. **This ADR records the mechanism that came out of it** — how a backend declares what it can do, and how that declaration reaches the callers that need it. Search without a service name is the mechanism's first user, and so far its only populated field; the other two divergences are what the shape is designed to accommodate.

The implementation lives in:

* [`internal/storage/v2/api/tracestore/reader.go`](../../internal/storage/v2/api/tracestore/reader.go) — `SearchCapabilities` and the `Reader` method that reports it
* [`internal/storage/v2/grpc/`](../../internal/storage/v2/grpc/) — the `jaeger.storage.v2.Capabilities` client and server
* [`cmd/jaeger/internal/extension/jaegerquery/querysvc/service.go`](../../cmd/jaeger/internal/extension/jaegerquery/querysvc/service.go) — enforcement
* [`cmd/jaeger/internal/extension/jaegerquery/internal/static_handler.go`](../../cmd/jaeger/internal/extension/jaegerquery/internal/static_handler.go) — reporting to the UI

## Decision

A backend declares its own capabilities through a method on the storage reader interface. The declaration is fallible, its zero value is the least capable backend, the query service is the single place that enforces it, and no caller keeps a copy of the answer.

## Architecture

### Declaration

`tracestore.Reader` carries `SearchCapabilities(ctx) (SearchCapabilities, error)`. Four properties of that signature do the work:

* **It is a required interface method, not an optional interface.** A wrapper that fails to forward it would silently report the wrapper's capabilities instead of the backend's. As a method, the compiler enumerates everything that must answer, including every reader decorator.
* **The zero value is the least capable reader.** A field added to `SearchCapabilities` therefore leaves every existing implementation declaring the new capability unsupported, rather than accidentally claiming it.
* **It returns an error, because "declares nothing" and "cannot say" are different answers.** A reader whose backend sits behind an API that cannot be asked returns `errors.ErrUnsupported`; callers read that as the least capable backend but can tell it apart from a backend that answered.
* **It takes a context, because the answer may arrive over the wire.** Only the remote-storage reader does I/O; every other reader returns a constant.

`SearchCapabilities` carries `WithoutServiceName`: Elasticsearch/OpenSearch, ClickHouse and memory report `true`; Cassandra and the v1 adapter (which fronts Badger) report `false`; gRPC remote storage reports whatever its backend says. The duration-combination and exact-limit divergences above are the next candidates, and land as sibling fields rather than as new methods.

Two sibling fields have since been added for [RFC 0005](../rfc/0005-structured-query-filters.md)'s structured query filter, which is where the shape's two predictions were first tested:

* **`SameSpanConjunction`** is reported, not enforced. A flat inverted index intersects at trace granularity, so it may satisfy different conjuncts of one query from different spans of the same trace. Refusing every multi-predicate query on those backends would regress a search they have always served, so this one is surfaced to the caller instead of gating anything. It is the first capability that is not a refusal gate, which is why the enforcement rule below is stated per field rather than for the message as a whole.
* **`Filter`**, a nested `FilterCapabilities` naming the attribute levels and the operators a backend evaluates. Grouping them in a sub-message rather than adding flat fields keeps the least-capable zero value: an absent `Filter`, or one naming nothing, means the backend serves only the legacy predicate fields, and the query service then rewrites a filter into them or refuses it. The boolean combinators are declared as operators like any other, so a backend confined to a conjunctive search is one that lists `and` and omits `or` and `not` — there is no separate flag for structural expressiveness, and nesting is not declared at all, because `and` is associative and the caller flattens it.

A declaration is also where a backend puts a feature gate, when its support for something is new enough to want one. It declares the capability only while its gate is enabled, and the zero value does the rest: a gated-off backend simply reports what it could always do, and the query service serves the query the way it did before. Two gates therefore compose without either knowing about the other — `jaeger.query.structuredFilters` admits a filter into the query path, checked by the query service alongside its other refusals, and `jaeger.<backend>.structuredFilters` decides whether that backend evaluates one, which is the naming pattern a backend should follow (the same leaf as the query-side gate, with the backend in place of `query`). The gates are separate because the API and each backend's implementation stabilize on separate schedules. One consequence worth handling: a backend refusing a predicate *because* its gate is off should say so, rather than reporting through `ErrFilterUnsupported` that it cannot serve the query at all, which would send an operator looking at their schema instead of their `--feature-gates`.

### Declaration across a remote boundary

`jaeger.storage.v2` has a `Capabilities` service ([jaeger-idl#211](https://github.com/jaegertracing/jaeger-idl/pull/211)), kept separate from `TraceReader` so a backend can serve one without the other. The remote-storage server answers from the reader it fronts, mapping a reader that cannot determine its own capabilities to `UNIMPLEMENTED` and any other failure to `INTERNAL`. The client maps `UNIMPLEMENTED` back to `errors.ErrUnsupported`, which is the same reading it already gives a backend without `FindTraceSummaries`, so a backend predating the service keeps working unchanged.

The client asks once and remembers the answer, which the proto declares stable for the lifetime of a connection. Failures are not remembered: a backend that is not up yet must not be mistaken for one that has answered. This is the only reader whose answer costs a round trip, so it is the only one that caches.

### Enforcement

The query service refuses a query whose shape the backend cannot serve, before it reaches storage, with a typed error the API layers map to `InvalidArgument` / HTTP 400. Storage readers guard themselves too, but they answer differently, so the single answer callers see is decided in one place. A reader that could not be asked is refused exactly like one that answered no, because rejecting is the safe reading either way.

Refusal is not the only answer a capability can produce, and which one applies is a property of the field rather than of the mechanism. A gate — `WithoutServiceName`, or a level or operator absent from `FilterCapabilities` — refuses. `SameSpanConjunction` reports instead, because the alternative would withdraw a search those backends have always served. And where a query can be expressed in terms a less capable backend does serve, the query service rewrites it rather than refusing or gating: a structured filter reaching a backend that declares no `Filter` support becomes the legacy predicate fields, and only what those cannot express is refused.

### Reporting

Two consumers read the declaration, and neither captures it:

* The query service asks the reader on every search that omits the service name.
* The static handler asks on every SPA serve, to build the `JAEGER_BACKEND_CAPABILITIES` blob the UI reads.

Nothing evaluates the capability when jaeger-query starts, because jaeger-query can come up before a remote backend is reachable; a value captured then would pin the process to the least capable behaviour for its whole life, including in the UI. Where asking is expensive the reader caches, which keeps that concern in the one place it applies.

### Testing

The storage integration suite gates capability-dependent tests on its own per-backend `integration/capabilities.Capabilities`, populated from the `STORAGE` under test, not on what the reader reports. The harness knows what it started; a backend that misreported its own capability would otherwise make the test skip, and a silently-skipped test reads like a passing one.

## Consequences

* Adding a capability is a two-line change to the struct plus one honest answer per backend, and the compiler lists the backends. The cost is that all of them must be touched.
* A backend can be graduated later — Badger stays at `false` today, and applying tag and operation filters inside its time-range scan would flip it — without any caller changing.
* API clients other than the UI cannot discover capabilities; they learn the limit by being refused. RFC 0013 §3.7 designed an api_v3 `Capabilities` service for this and rejected it, on the grounds that the UI already reads a live answer and that a test gating on a self-report is weaker than one gating on the harness's own knowledge. A deployment-capability-discovery API remains open as a feature in its own right, covering archive, metrics and AI storage as much as search.
* Capabilities are assumed stable for the lifetime of a connection. A backend whose abilities change under a running jaeger-query would not be noticed until the connection is re-established.

## References

* [RFC 0013: Optional Service Name in Trace Search](../rfc/0013-optional-service-name-in-search.md)
* [jaeger-idl#211](https://github.com/jaegertracing/jaeger-idl/pull/211) — the `Capabilities` service
* [#9256](https://github.com/jaegertracing/jaeger/pull/9256) — declaration on `tracestore.Reader`
* [#9259](https://github.com/jaegertracing/jaeger/pull/9259) — enforcement in the query service
* [#9268](https://github.com/jaegertracing/jaeger/pull/9268) — reporting without capturing the answer
* [#9269](https://github.com/jaegertracing/jaeger/pull/9269) — declaration across gRPC remote storage
* [ADR-001](./001-cassandra-find-traces-duration.md) — a Cassandra query restriction of the same kind, predating this mechanism
