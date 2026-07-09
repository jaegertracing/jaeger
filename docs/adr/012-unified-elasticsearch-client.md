# ADR-012: Unified Elasticsearch/OpenSearch Client

* **Status**: Implemented — graduated from [RFC 0006](../rfc/0006-unified-elasticsearch-client.md)
* **Date**: 2026-07-08

## Context

Jaeger reached Elasticsearch/OpenSearch through two unrelated client stacks — a data-plane client wrapping the deprecated [`olivere/elastic`](https://github.com/olivere/elastic) library and a separate control-plane client on raw `net/http` — with several operations implemented two or three times and TLS/auth applied inconsistently between them.

[RFC 0006](../rfc/0006-unified-elasticsearch-client.md) analyzes that problem, surveys the alternatives, and lays out the migration; it was delivered across milestones M1–M11 (umbrella issue [#7612](https://github.com/jaegertracing/jaeger/issues/7612)). **This ADR records only the resulting architecture** — see the RFC for the motivation, trade-off analysis, and milestone-by-milestone history.

The implementation lives in:

* [`internal/storage/elasticsearch/esclient/`](../../internal/storage/elasticsearch/esclient/) — the client, transport pool, RoundTripper stack, role interfaces, and response types
* [`internal/storage/elasticsearch/query/`](../../internal/storage/elasticsearch/query/) — the request-body query/aggregation AST
* [`internal/storage/elasticsearch/indices/`](../../internal/storage/elasticsearch/indices/) — index rotation strategies
* [`internal/storage/elasticsearch/snapshottest/`](../../internal/storage/elasticsearch/snapshottest/) — the request-snapshot suite that pins wire format

## Decision

A single Jaeger-owned client, **`esclient`**, carries **all** Elasticsearch/OpenSearch traffic — searches, bulk writes, and index/alias/template/rollover/ILM administration — over one transport with one TLS/auth/SigV4/`custom_headers` stack. `olivere/elastic` is removed entirely. `go-elasticsearch/v9` is retained **only for its `esutil.BulkIndexer`**, driven over the shared [`elastic-transport-go`](https://github.com/elastic/elastic-transport-go) connection pool via the `esapi.Transport` interface — not the product-checked `elasticsearch.Client`, which is unsuitable for OpenSearch.

## Architecture

### One client over one transport

`esclient.Client` is a pointer handle composed over a low-level `rawClient`, which owns an [`elastic-transport-go`](https://github.com/elastic/elastic-transport-go) connection pool (multi-node round-robin with failover; node discovery/sniffing off; library retry off). The pool sends every request through a base `http.RoundTripper` built by `GetHTTPRoundTripper`: TLS, then basic/bearer/API-key auth, SigV4 signing, a `custom_headers`/`Host` layer, and a `getBodyFix` layer that populates `req.GetBody` so signers can re-read the payload.

Both planes share this one stack: a search, a bulk flush, and an `es-rollover` alias swap all traverse the same pool and the same auth chain. This is what made SigV4 body signing (#8760) and `custom_headers` (#8916) work uniformly, and gave the admin CLIs the full auth stack.

### Requests: an owned query AST

`internal/storage/elasticsearch/query` builds the request-body JSON — the query and aggregation AST — that the storage layer sends. It carries exactly the nodes Jaeger uses (`bool`/`term`/`terms`/`match`/`regexp`/`nested`/`range`/`exists` queries; `terms`/`date_histogram`/`percentiles`/`min`/`max`/`filter`/`top_hits`/`cumulative_sum` and scripted aggregations), each rendering its wire form via `Source()`. There is no third-party query builder.

### Responses: owned decode types

Responses decode straight from the wire into owned types: `SearchResponse` with a lazily-decoded, accessor-based `Aggregations` type (so a top-level `date_histogram`'s numeric-keyed buckets don't collide with strict string-keyed terms buckets), `HitsResult`/`SearchHit` (with `_source` as `json.RawMessage`), and typed `HistogramResult`/`PercentilesResult`.

### Role interfaces, not a fat client

Consumers depend on narrow interfaces, not one god-object:

* `Searcher` — `Search` (`_search`) and `MultiSearch` (`_msearch`)
* `BulkWriter` — `Add` only
* `IndexAPI` (via `IndicesClient`) — index/alias/template/rollover administration
* `IndexManagementLifecycleAPI` (via `ILMClient`) — lifecycle-policy existence
* `IndexExistenceChecker` — the sampling store's one-method probe

The storage factory constructs one `esclient.Client` and composes these surfaces over it, so a single version probe backs everything.

### Bulk writes via `esutil` over our transport

`BulkIndexer` wraps the official `go-elasticsearch/v9` `esutil.BulkIndexer` — a bounded, worker-pooled indexer that flushes on a byte threshold or interval (the fix for unbounded bulk memory, #2192) — but drives it through Jaeger's `esclient.Client`, which satisfies `esapi.Transport` via `Perform`. Bulk therefore runs on the same multi-node pool and auth/TLS/SigV4 stack as everything else; no product-checked go-elasticsearch client is constructed. Hand-writing the bulk indexer was considered and rejected (RFC 0006 M6 trade-off matrix).

### Backend version resolved once, then encapsulated

The backend version is resolved at construction — an explicit `config.Version`, else a single `GET /` ping through the shared `es.ResolveBackendVersion` — and stored on an unexported field. Version-dependent choices (`_template` vs `_index_template`, ILM vs ISM endpoints, `rest_total_hits_as_int`, typed-index suppression) live *inside* the client. No caller or orchestrator holds or branches on a `BackendVersion`; the CLIs say "create the templates" / "ensure the policy" in Jaeger terms.

### Wire-format stability

A request-snapshot suite (`snapshottest`) pins the exact bytes each operation emits for every supported backend/version. Each migration slice kept its snapshots byte-identical, which is how the `olivere`→`esclient` move was proven behavior-preserving path by path rather than trusted wholesale.

## Consequences

**Positive**

* One place applies TLS/auth/SigV4/`custom_headers`, fixing #8760 and #8916 and giving `es-rollover`/`es-index-cleaner` the full auth stack (bearer/API-key, multi-node, failover) they previously lacked.
* Bounded bulk-write memory (#2192).
* Exactly one version-detection path; the version is fully encapsulated, so callers never branch on ES-vs-OS.
* Jaeger owns the connection lifecycle (`Close` → `CloseIdleConnections`), releasing pooled connections on shutdown.
* `olivere/elastic` and its transitive dependencies are gone.

**Negative / trade-offs**

* Jaeger now owns the query AST and response decoding a library previously provided — more code to maintain, but scoped to exactly what is used and directly snapshot-tested against real backends.
* `go-elasticsearch/v9` remains as a transport-level dependency for `esutil.BulkIndexer`; it is not fully eliminated. Forking or hand-rolling `esutil` was judged not worth the maintenance cost.

## References

* [RFC 0006: Unified Elasticsearch/OpenSearch Client](../rfc/0006-unified-elasticsearch-client.md) — the proposal, alternatives, and full milestone history (M1–M11).
* Umbrella issue [#7612](https://github.com/jaegertracing/jaeger/issues/7612).
* Fixes that fell out of unification: [#2192](https://github.com/jaegertracing/jaeger/issues/2192) (bounded bulk), [#8760](https://github.com/jaegertracing/jaeger/issues/8760) (SigV4 body signing), [#8916](https://github.com/jaegertracing/jaeger/issues/8916) (`custom_headers`).
