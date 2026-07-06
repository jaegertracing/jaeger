# RFC 0006: Unified Elasticsearch/OpenSearch Client

- **Status:** Draft
- **Author:** Yuri Shkuro
- **Created:** 2026-07-03
- **Last Updated:** 2026-07-03
- **Issue:** [#7612](https://github.com/jaegertracing/jaeger/issues/7612)
- **Related:** [#4708 Data Streams](https://github.com/jaegertracing/jaeger/issues/4708) · [RFC 0004](./0004-elasticsearch-data-streams.md) · [#2192](https://github.com/jaegertracing/jaeger/issues/2192) · [#8916](https://github.com/jaegertracing/jaeger/issues/8916) · [#8760](https://github.com/jaegertracing/jaeger/issues/8760)

---

## Abstract

Jaeger talks to Elasticsearch/OpenSearch through **two unrelated client abstractions**:

1. A **data-plane** client (`internal/storage/elasticsearch`, the `es.Client` interface) that wraps the deprecated [`olivere/elastic`](https://github.com/olivere/elastic) library (plus a second, `go-elasticsearch/v9`, client bolted on for one operation). It carries bulk writes, searches, and aggregations.
2. A **control-plane** client (`internal/storage/elasticsearch/esclient`, the `IndexAPI`/`ClusterAPI`/`IndexManagementLifecycleAPI` interfaces) built on raw `net/http`. It carries index/alias/template/rollover/ILM management for the `es-rollover` and `es-index-cleaner` tools.

The split is historical, not principled. The boundary is already leaky — the storage factory performs "control-plane" operations (`CreateTemplate` at bootstrap, `DeleteIndex` on purge) through the data-plane client — and several operations (`IndexExists`, `CreateIndex`, `DeleteIndex`, `CreateTemplate`, version detection) are implemented **twice or three times**.

This RFC proposes collapsing the two into a **single Jaeger-owned client** exposing two interfaces (data and admin) from one package, over one transport, with one version-detection path. It analyzes the central obstacle — **no official Go SDK can talk to both current Elasticsearch and OpenSearch** — and recommends a transport-level client that owns its request/response JSON, preserving the wide single-binary version matrix that `olivere` gives us today while unblocking bugs that `olivere` cannot fix.

This is a design exploration, not a committed decision. It builds on the investigation in [#7612](https://github.com/jaegertracing/jaeger/issues/7612) and the community analyses referenced there.

---

## 1. Motivation

### 1.1 Two clients, one backend

| | Data plane | Control plane |
|---|---|---|
| Package | `internal/storage/elasticsearch` (`es`) + `.../wrapper` | `internal/storage/elasticsearch/esclient` |
| Interface(s) | `es.Client` + 7 fluent service interfaces | `IndexAPI`, `ClusterAPI`, `IndexManagementLifecycleAPI` |
| Transport | `olivere/elastic/v7` (+ `go-elasticsearch/v9` for one op) | raw `net/http` |
| Operations | `_bulk`, `_search` (+aggs), `_msearch` (+`search_after`), template/index create, version | index/alias create+delete, template, rollover, ILM/ISM, list indices, version |
| Consumers | span/service/dependency/sampling/metrics read+write | `es-rollover`, `es-index-cleaner`, index filters |

There is no architectural reason for these to be different clients. They point at the same cluster, speak the same REST API, and must agree on the same backend version and auth. Maintaining two transports means two places to fix auth, two version-detection code paths, and two implementations of overlapping operations.

### 1.2 The boundary is already leaky

The "data plane never manages indices" premise is false today:

- the factory's `createTemplates` runs at startup — via the **data-plane** client.
- `CreateSamplingStore` (in the factory) creates the sampling index template — data-plane client.
- `Purge` (in the factory) issues `DeleteIndex("*")` — data-plane client.

Meanwhile the same logical operations exist on the control-plane client (`index_client.go`). `CreateTemplate` is implemented **three times**: `olivere` legacy `_template`, `go-elasticsearch/v9` composable `_index_template` (in the data-plane wrapper), and a raw-HTTP version-gated variant (in the control-plane client). Version gating (`UsesV8API`, `SupportsTypedIndices`, `DetectBackendVersion`) is duplicated on both sides and can drift.

### 1.3 `olivere/elastic` is a dead end

Jaeger pins `github.com/olivere/elastic/v7 v7.0.32` — the last release of a library that is **unmaintained** and whose typed line stops at Elasticsearch 7.x (there is no `olivere` v8/v9). Its virtue is accidental: it is essentially a JSON builder with **no product/version gatekeeping**, so a single build talks to Elasticsearch 6/7/8/9 *and* OpenSearch 1/2/3. Any replacement must consciously preserve that property (see §4).

Its deadness actively blocks bug fixes:

- **[#2192](https://github.com/jaegertracing/jaeger/issues/2192)** — unbounded bulk memory / "Request Entity Too Large". `olivere`'s `BulkProcessor` has no hard byte ceiling; the fix (a size-bounded bulk indexer) lives in the maintained clients.
- **[#8916](https://github.com/jaegertracing/jaeger/issues/8916)** — `custom_headers` are wired only into the `go-elasticsearch/v9` client, but the `olivere` client carries **all** data-path traffic, so custom headers are silently dropped on every write/search.
- **[#8760](https://github.com/jaegertracing/jaeger/issues/8760) / [#8307](https://github.com/jaegertracing/jaeger/issues/8307)** — SigV4 auth fails on body-bearing requests to AWS-managed OpenSearch because the body isn't available to the signer at signing time.

These are not incidental; they are symptoms of building on a frozen library and papering over it with a second client.

### 1.4 Why now

A rewrite of this size should not happen in isolation. Per the direction in [#7612](https://github.com/jaegertracing/jaeger/issues/7612), it should be **sequenced with the Data Streams work** ([RFC 0004](./0004-elasticsearch-data-streams.md), [#4708](https://github.com/jaegertracing/jaeger/issues/4708)): data streams reduce the need for the external `es-rollover`/`es-index-cleaner` tools, where most of the control-plane surface lives. This RFC unifies the *client* those tools run on; retiring the tools by folding index-management bootstrap into the storage factory is the complementary data-streams half (out of scope here — §8). The two together deliver the "one client, no external tools" goal, which is why the sequencing needs coordinating (§9).

---

## 2. Current API surface (what we actually use)

A precise inventory matters: the smaller the real surface, the more viable a hand-owned client is.

### 2.1 Data plane (via `es.Client`)

- **Bulk index** (`_bulk`): spans, services, dependencies, sampling throughput/probabilities. Op-type toggles `index`/`create` for data streams; `_type` suppressed when the backend doesn't support typed indices.
- **Search** (`_search`): service/operation lookup, trace summaries, `findTraceIDs` via terms aggregation, metrics via date-histogram aggregations. `IgnoreUnavailable`, `Size(0)`, `RestTotalHitsAsInt` for older backends.
- **MultiSearch** (`_msearch`): batched trace-by-ID fetch with **`search_after`** pagination. **No Scroll API is used anywhere.**
- **Template/index create + version**: `CreateTemplate` at bootstrap, `IndexExists` (sampling), `DeleteIndex` (purge), cached `GetVersion` from a startup ping.

Leaked dependency: `olivere` types (`elastic.Query`, `elastic.Aggregation`, `elastic.SearchResult`, `elastic.SearchRequest`, `elastic.MultiSearchResult`) appear in `es.Client`/service signatures and in caller code (reader/metricstore), so callers are coupled to `olivere` at the type level.

### 2.2 Control plane (via `client.Client`, raw HTTP)

`GetJaegerIndices` (`GET .../jaeger-*`), `CreateIndex` (`PUT`), `DeleteIndices` (batched `DELETE`, splits at URL-length limit), `CreateAlias`/`DeleteAlias` (`POST /_aliases`), `AliasExists`/`IndexExists` (`HEAD`), `CreateTemplate` (version-gated `_template` vs `_index_template`), `Rollover` (`POST /_rollover`), ILM/ISM `Exists` (`GET /_ilm/policy/...` vs `GET /_plugins/_ism/policies/...`), `Version` (`GET /`).

### 2.3 Overlap (the duplication a unified client removes)

| Operation | Data plane | Control plane | Endpoint |
|---|---|---|---|
| Version detection | `GetVersion` (cached ping) | `Version` | `GET /` |
| IndexExists | ✔ (sampling) | ✔ | `HEAD /{index}` |
| CreateIndex | ✔ | ✔ | `PUT /{index}` |
| DeleteIndex(es) | ✔ (purge) | ✔ (batched) | `DELETE /{index}` |
| CreateTemplate | ✔ (×2: olivere + v8) | ✔ (raw, gated) | `PUT /_template` or `/_index_template` |

The realistic total surface is **small and REST-shaped** — a strong signal that Jaeger can own it.

---

## 3. Goals and non-goals

### Goals

- **G1.** One client for both data plane and control plane, meaning **one shared low-level transport and one version-detection path** — the horizontal concerns are not duplicated. Payload APIs may be one struct or several cohesive structs composed over that shared transport (a free choice; §6.1). Callers see **small, focused role interfaces** (`Searcher`, `BulkWriter`, `IndexManager`, `LifecycleManager`, …) — segregated interfaces are explicitly wanted (easier to mock, no coupling to unused methods), a fat interface is not.
- **G2.** Compatible with both Elasticsearch and OpenSearch **without leaking backend differences to callers.** ILM-vs-ISM, `_template`-vs-`_index_template`, typed-vs-untyped indices are resolved *inside* the client.
- **G3.** Preserve the current single-binary version matrix: **Elasticsearch 7/8/9 and OpenSearch 1/2/3** from one build (Elasticsearch 6 reached EOL and support was removed). Do not regress supported backends (§4, §6).
- **G4.** Unblock the bugs `olivere` cannot fix: bounded bulk memory (#2192), universal `custom_headers` (#8916), correct SigV4 body signing (#8760).
- **G5.** Remove `olivere` and the duplicated version/operation logic.
- **G6.** A testing model that makes the emitted wire format explicit and regression-sensitive (§7).

### Non-goals

- Changing the on-disk document schema (native OTLP is orthogonal; see RFC 0004 §2).
- Redesigning index-management *strategies* (data streams is RFC 0004's job). This RFC changes the *client*, not the rotation model — though it should make the data-streams path cleaner.
- Migrating the query **semantics**. Queries should produce byte-identical requests before/after where possible (§7 makes this testable).

---

## 4. The compatibility constraint (the crux)

The naive reading of G1–G2 is "adopt the official SDK." Research into the SDK landscape shows this **cannot** satisfy G3 with a single official SDK. The findings:

### 4.1 There is no single official Go client for both products

- **The fork.** Elastic relicensed at 7.11 (Jan 2021); AWS forked the last Apache-2.0 release, **7.10.2**, into OpenSearch. Everything ≤7.10.2 is shared lineage; everything after diverges.
- **The deliberate anti-fork gate.** Elasticsearch **7.14+** stamps `X-Elastic-Product: Elasticsearch` on responses, and the official [`go-elasticsearch`](https://github.com/elastic/go-elasticsearch) client **hard-requires** it: on the first 2xx response, `BaseClient.Perform` runs a product check and, if the header is absent (OpenSearch, or open-source ES ≤7.13), fails with *"the client noticed that the server is not Elasticsearch and we do not support this unknown product."* **There is no config flag or env var to disable it** — the check sits in the lowest-level `Perform`, so even hand-writing JSON through the client is gated. (Elastic explicitly declined to add an opt-out: [elastic/elasticsearch#73424](https://github.com/elastic/elasticsearch/issues/73424).)
- **The separate `compatible-with` media type** (opt-in versioned `Accept`/`Content-Type`) is a *one-major, newer-server-honors-older-client* bridge, **not** a multi-version strategy, and OpenSearch/ES7 reject it with HTTP 406.

**Consequence:** `go-elasticsearch` v8/v9 cannot talk to OpenSearch at all, and no single official ES major spans 7→8→9 (they are separate modules `/v7`, `/v8`, `/v9`, forward-compatible only).

### 4.2 `opensearch-go` is a pre-fork snapshot, not a bridge

[`opensearch-go`](https://github.com/opensearch-project/opensearch-go) is a **direct fork of `go-elasticsearch` 7.10.2**, taken *before* the product check. It therefore has **no product check** and will talk to OpenSearch 1/2/3 **and** open-source Elasticsearch 7.10-era — but it is a 7.10 client with **no documented ES 8/9 support**, and no single `opensearch-go` major spans OpenSearch 1.x+2.x+3.x (the v4.6.x line is the widest: 1.3.20–3.6.0). Its API is low-level `esapi`-style (build JSON yourself), not a fluent builder.

### 4.3 API divergence beyond the wire gate

Even where the wire is reachable, ILM (`_ilm/policy`) vs ISM (`_plugins/_ism/policies`), composable-template semantics (`priority` vs `order`), data-stream differences (ES migrate API, OpenSearch's required `timestamp_field`), and `_type` removal timelines differ. A shared *typed* client cannot paper over these; they must be branched deliberately.

### 4.4 What everyone else does

There is **no** "works with both" official Go client. Real-world patterns: **plain HTTP + runtime version detection** (Jaeger today; Vector's ES sink with `api_version: auto`); **two separate integrations** (Grafana ships distinct ES and OpenSearch plugins); or **pick one** (Graylog dropped ES for OpenSearch). Even the fluent-builder world re-forked rather than unified (`disaster37/opensearch` is `olivere` re-forked for OpenSearch).

> **The uncomfortable truth:** `olivere`'s wide compatibility comes precisely from being a dumb JSON mover with no product check. To keep G3 with a maintained stack, we must **reproduce that property**, not inherit it from a typed SDK.

---

## 5. Options considered

### 5.1 The candidate options

- **Baseline — status quo:** `olivere/elastic` (data plane) + raw-HTTP `client` (control plane) + `go-elasticsearch/v9` for one op. Listed to make "do nothing" a scored option, not an implicit escape.
- **A — Owned client:** one Jaeger client in one package, implemented over an HTTP transport, **Jaeger owns request/response JSON** at the ES-primitive (driver-neutral) level; backend/version differences resolved internally. The transport tier reuses our existing `clientbuilder.GetHTTPRoundTripper` (TLS + auth + **SigV4**) layered under `elastic-transport-go`'s *product-check-free* connection pool (multi-node round-robin, retry) — a transport, never the product-checked client (§6.1, §6.3).
- **B — Two SDKs behind a facade:** `go-elasticsearch` for ES, `opensearch-go` for OpenSearch, dispatched by detected backend behind one Jaeger-concept interface.
- **C1 — `go-elasticsearch` for both:** single official SDK; reach OpenSearch by a custom transport that **forges** `X-Elastic-Product` and strips the `compatible-with` media type.
- **C2 — `opensearch-go` for both:** single official SDK; it has no product check and talks to OpenSearch 1/2/3 and ES-OSS 7.10, but has no supported path to ES 8/9.

### 5.2 Evaluation criteria

| # | Criterion | 🟢 means |
|---|---|---|
| K1 | **Backend coverage** | ES 6/7/8/9 **and** OS 1/2/3 all reachable from one binary |
| K2 | **Future-version resilience** | a new ES/OS release "just works" without a client upgrade or code change |
| K3 | **No caller leakage (G2)** | callers see one Jaeger-concept API; ILM/ISM, template, typed-index differences hidden |
| K4 | **Single client / low duplication (G1)** | one code path, one transport, one version-detection; no per-backend fork |
| K5 | **Unblocks olivere bugs (G4)** | we control bulk buffering (#2192), headers (#8916), SigV4 body signing (#8760) |
| K6 | **Upstream health** | not dead-ended; receives fixes for new backend versions |
| K7 | **Dependency footprint** | small, clean dependency surface; low CVE/churn exposure |
| K8 | **Build effort & risk** | little to build/own ourselves; low migration risk |

Note K8 is deliberately included as the axis where the recommended option scores *worst* — the matrix is meant to expose the real trade-off, not to rig the result.

### 5.3 Comparison matrix

🟢 good · 🟡 partial / caveated · 🔴 poor

| Criterion | Baseline (status quo) | **A: Owned client** | B: Two SDKs | C1: go-es (forged) | C2: opensearch-go |
|---|:---:|:---:|:---:|:---:|:---:|
| K1 Backend coverage | 🟢 | 🟢 | 🟢¹ | 🟡² | 🔴³ |
| K2 Future-version resilience | 🟢 | 🟢 | 🔴 | 🔴 | 🟡 |
| K3 No caller leakage | 🔴 | 🟢 | 🟢 | 🟡 | 🟡 |
| K4 Single client / low duplication | 🔴 | 🟢 | 🔴 | 🟢 | 🟢 |
| K5 Unblocks olivere bugs | 🔴 | 🟢 | 🟢 | 🟢 | 🟢 |
| K6 Upstream health | 🔴 | 🟢⁴ | 🟢 | 🟡⁵ | 🟢 |
| K7 Dependency footprint | 🟡 | 🟢 | 🔴⁶ | 🟡 | 🟡 |
| K8 Build effort & risk | 🟢 | 🔴⁷ | 🟡 | 🔴 | 🟡 |

- ¹ Achievable but needs `go-elasticsearch` **v7 *and* v8/v9** modules (no single ES major spans 7→9), compounding K7.
- ² OpenSearch only via forged headers — brittle and unsupported.
- ³ No supported ES 8/9.
- ⁴ We own it, but the surface is small (§2) and the control plane already works this way. The ongoing-maintenance *cost* is captured under K8, not here.
- ⁵ SDK is healthy, but we depend on an unsupported bypass that upstream may break at will.
- ⁶ Ships two near-duplicate forks with colliding symbols (`Config`, `Client`, `BulkIndexer`) plus a third ES major.
- ⁷ Smaller than it looks: auth/SigV4 already exist as `clientbuilder.GetHTTPRoundTripper`, and pool/round-robin/retry/discovery are reused from the *product-check-free* `elastic-transport-go` with our RoundTripper as its base (§6.1). The genuine new build is the query AST, the response types, and the bounded bulk indexer (#2192).

### 5.4 Reading the matrix

- **Baseline** is green only on "easy" (K8) and raw reachability — and red on every axis that motivates this RFC (leakage, duplication, bugs, dead upstream). Doing nothing scores worst where it matters.
- **Options C1 and C2** collapse on the two hard requirements: C1 can't cover OpenSearch without header forgery, C2 can't cover modern Elasticsearch. Both fail G3 (K1).
- **B** is genuinely viable and the main alternative to A. It wins K5/K6 and ties on K3, but pays permanently on K4 (two code paths), K7 (fork bloat), and K2 (typed SDKs are version-coupled and product-gated). It also doesn't truly deliver "one client" (G1) — two behind a curtain.
- **A** is the only option green across K1–K7, at the cost of a red on K8. That is the crux trade: **we accept building and owning a small HTTP client in exchange for the wide matrix, zero leakage, single code path, and bug fixes.** The §2 surface is small and REST-shaped, the control plane already demonstrates the pattern, and the §7 snapshot suite de-risks the build — so the K8 cost is bounded and one-time, while A's green column is permanent.

### 5.5 Backend reachability detail

The matrix's K1/K2 rows summarize this per-version reachability table:

🟢 reachable · 🟡 reachable with caveats · 🔴 not reachable

| Approach | ES 6 | ES 7 | ES 8 | ES 9 | OS 1 | OS 2 | OS 3 | One binary? |
|---|:---:|:---:|:---:|:---:|:---:|:---:|:---:|:---:|
| Baseline (`olivere`) | 🟢 | 🟢 | 🟢¹ | 🟢¹ | 🟢 | 🟢 | 🟢 | 🟢 |
| **A: owned client** | 🟢 | 🟢 | 🟢 | 🟢 | 🟢 | 🟢 | 🟢 | 🟢 |
| B: two SDKs | 🟡⁴ | 🟢² | 🟢 | 🟢 | 🟢 | 🟢 | 🟢 | 🟢 (2+ deps) |
| C1: go-es forged | 🔴 | 🟢³ | 🟢 | 🟢 | 🟡 forge | 🟡 forge | 🟡 forge | 🟢 |
| C2: opensearch-go | 🔴 | 🟢 (OSS 7.10) | 🔴 | 🔴 | 🟢 | 🟢 | 🟢 | 🟢 |
| Typed go-es as-is | 🔴 | 🔴 (v9) | 🟡 v8 only | 🟢 | 🔴 | 🔴 | 🔴 | 🔴 |

¹ `olivere` reaches ES 8/9 for the REST subset Jaeger uses because it doesn't gate on version; the one composable-template gap is why `go-elasticsearch/v9` was bolted on. ² needs `go-elasticsearch/v7` too. ³ requires stripping the compat header. ⁴ B would additionally need the EOL `go-elasticsearch/v6` module.

**Only Option A is green across coverage, resilience, leakage, single-client, and bugs simultaneously** — which is why §6 adopts it.

---

## 6. Proposed approach

**Adopt Option A.** Build a client that owns its wire format and detects backend/version once, sharing a **single low-level transport** by composition and exposing **many small, focused interfaces** to callers.

### 6.1 One shared transport; API structs by composition

The load-bearing invariant is **one shared low-level client** — call it `rawClient` — that owns the *horizontal* concerns: connection pooling, multi-node round-robin, retry, gzip, auth (basic/API-key/**SigV4**), `custom_headers`, the JSON round-trip, and **a single version-detection / capability probe**. Everything that must not be duplicated lives here.

The transport is **largely reused, not built**, composed from two pieces:

- Our existing `clientbuilder.GetHTTPRoundTripper` (TLS + auth + **SigV4** + `GetBody` fix + header-forwarding) — today only the data plane goes through it; routing the admin path through it too unifies auth and closes the `es-rollover` SigV4/bearer gap.
- **`elastic-transport-go`** — the layer *beneath* `go-elasticsearch`, Apache-2.0, already a transitive dependency, and (verified) **carrying no product check**: the `X-Elastic-Product` gate lives in the `go-elasticsearch` *client*, not the transport. It supplies the connection pool, **multi-node round-robin + failover**, retry, and opt-in node discovery. Our `GetHTTPRoundTripper` plugs in as its base `Config.Transport`, so signing/headers stay ours while the pool/retry/failover come from a battle-tested library; `rawClient` drives it via `Perform`.

This is not just convenience: Jaeger already load-balances across multiple `server_urls` (olivere round-robins them today), so a single-endpoint `http.Transport` would **regress** multi-node support — reusing `elastic-transport-go`'s pool preserves it, without reintroducing the product check. *(The transport-reuse layering was pointed out by [@Manik2708](https://github.com/Manik2708) and [@Me-Priyank](https://github.com/Me-Priyank) in the [#8919](https://github.com/jaegertracing/jaeger/pull/8919) review.)*

The *payload*-level APIs are **distinct structs composed over that `rawClient`**, grouped by cohesion — e.g. a data-plane struct (search/bulk) and one or more admin structs (index/alias/template, lifecycle, cluster). This is deliberately *not* prescribed as a single omni-struct:

- **Composition, not a god object.** Each API struct embeds/holds the shared `rawClient` and adds only its own payload logic. This is exactly today's control-plane pattern generalized — `IndicesClient`, `ClusterClient`, `ILMClient` already each embed the base `client.Client`. We keep that shape and bring the data plane into it.
- **The number of API structs is a free variable.** One struct satisfying every interface, or several cohesive ones, are both fine — the choice is about cohesion and testability, not correctness. What is *fixed* is a single `rawClient` beneath them, so there is one transport and one version-detection (killing §1.2's triple `CreateTemplate` and the two version paths).
- **Small role interfaces regardless of struct count.** Consumers depend on narrow, consumer-defined interfaces — `Searcher`, `BulkWriter`, `IndexManager`, `AliasManager`, `TemplateManager`, `LifecycleManager` (ILM/ISM), `ClusterInfo` — each satisfied by whichever struct implements it. A reader depends only on `Searcher`; `es-rollover` init on `IndexManager`/`AliasManager`/`LifecycleManager`. This preserves the granularity the control plane already mocks per-interface and **fixes the data plane**, where `es.Client` is today a fat 8-method interface that should be split rather than carried over whole.

```
internal/storage/elasticsearch/esclient/   (rename of today's .../client pkg, grown upward — see §6.4)
  transport.go   // rawClient: elastic-transport-go pool (multi-node round-robin, retry, discovery)
                 //            over our GetHTTPRoundTripper (TLS, auth/SigV4, custom_headers). Horizontal only.
  client.go      // Client{ *rawClient }: request primitive (timeout, JSON, errors); the base the API structs embed
  version.go     // single DetectBackendVersion resolved once, held on Client; capability flags (UsesV8API, …, UseISM)
  data.go        // Data struct{ Client }      — Search, MultiSearch, Bulk
  admin.go       // Index/Alias/Template/Lifecycle/Cluster struct(s){ Client } — mgmt payloads
  api.go         // small role interfaces: Searcher, BulkWriter, IndexManager, AliasManager,
                 //                        TemplateManager, LifecycleManager, ClusterInfo
  bulk.go        // bounded bulk indexer (FlushBytes + workers) — fixes #2192
```

- **Backend differences live behind these interfaces**, resolved once from the shared capability struct: ILM vs ISM, template endpoint selection, `rest_total_hits_as_int`, typed-index suppression, data-stream op-type. Callers pass Jaeger concepts, never `elastic.*` structs (removes the §2.1 leak).
- **Auth/headers/signing are `rawClient` concerns**, applied uniformly to *every* request from *every* API struct — which is exactly what fixes #8916 (headers everywhere) and #8760 (SigV4 sees the body). Concentrating them in one place is the whole point of the shared transport.
- **Multi-node round-robin on; node discovery (sniffing) off by default.** The `elastic-transport-go` pool round-robins across the configured `server_urls` with failover (preserving today's `olivere` behavior), while its node discovery (`DiscoverNodesInterval`) stays opt-in — matching the official SDKs and unlike `olivere`, whose default-on sniffing is a known source of proxy/AWS misconfig, which is why Jaeger already disables it.
- **Retry comes from the transport for reads; the bulk indexer owns write retry.** `elastic-transport-go` retries whole requests on 502/503/504 with backoff — safe for idempotent searches (a mild improvement over today, where reads aren't retried) and disable-able if we choose to match exactly. The bulk *write* path is deliberately different: the bounded bulk indexer keeps `BulkProcessor`'s **per-item** retry (backoff, re-enqueue on 408/429/503/507) rather than a blind whole-batch replay, which would duplicate writes. Preserving that per-item behavior is a requirement of the #2192 replacement, not just a byte cap.
- **Mockability (ties to §7):** narrow interfaces make focused fakes cheap — a one-method `Searcher` spy can assert *which indices* a reader selected without touching the query body. Snapshots and mocks are complementary and chosen by the test's subject (wire format → snapshot; computed decision like index selection → focused mock); see §7.4. The only thing the strategy retires is re-mocking the query *builder* to re-check the wire format, which proved to be coverage-filler (§7.1).

### 6.2 What `esclient` is — and is not

`esclient` is an **Elasticsearch/OpenSearch client**, not a storage backend. It speaks the *ES/OS* vocabulary — search, bulk, index/alias/template/rollover/lifecycle — and its one hard requirement is to be **driver-neutral**. Jaeger *storage* vocabulary — traces, spans, dependencies — lives one layer up, in `spanstore`/`tracestore`/`depstore`, which build queries and call `esclient`. Two distinct boundaries:

```
tracestore.Reader.FindTraceIDs(ctx, TraceQueryParameters)   ← storage layer: Jaeger domain. NOT esclient.
        │  builds the ES query, decides indices, paginates
        ▼
esclient  Searcher.Search(ctx, indices, q) / BulkWriter.Add(doc) / IndexManager… ← ES/OS domain, driver-neutral
        │  serializes to wire JSON, signs, sends
        ▼
Elasticsearch / OpenSearch REST
```

`FindTraceIDs(params)` is emphatically **not** an `esclient` method — it's what the storage reader *is about*. Hoisting it into the client would merge the storage implementation into the driver layer. So this RFC scopes `esclient` at the ES-primitive altitude and leaves the trace-domain facade where it belongs.

**The problem `esclient` must fix — driver-neutrality.** Today's `es.Client` mirrors `olivere`'s objects: `Search()` returns a `SearchService` you chain (`.Query(elastic.Query).Aggregation(elastic.Aggregation).Do(ctx) → *elastic.SearchResult`), so *driver* types cross the boundary and appear in Jaeger's own signatures (§2.1). Swap the driver and every caller changes. The fix: **no `elastic.*` type crosses the `esclient` interface** — parameters and returns are Jaeger-owned, so which driver serializes them is invisible above. That is the property the §6.1 interfaces must have and today's lack; it's what the #7612 steer ("Jaeger concepts, not driver concepts") means *at this layer*.

**The query representation is a small AST that `esclient` owns and serializes.** The reader constructs queries from Jaeger-owned node types; `esclient` renders them to per-backend wire JSON. It does *not* accept a pre-marshaled JSON body from the reader — that would force the storage layer to emit backend-specific JSON, pushing ES-vs-OS differences up into storage and defeating the client's purpose. Backend-agnosticism only holds if the DSL boundary sits inside `esclient`.

A single neutral AST suffices because Jaeger's query surface is small, closed, and identical across backends:

- **The surface is ~17 node types:** **8 query nodes** (`bool`, `term`, `terms`, `match`, `range`, `exists`, `nested`, `regexp`) + **9 aggregations** (`terms`, `min`, `max` incl. Painless-scripted, `date_histogram`, `percentiles`, `cumulative_sum`, `filter`, `top_hits`) + a small search envelope (`size`, `sort`, `search_after`, `track_total_hits`, `_source`, `ignore_unavailable`, `_msearch`). No scroll; no `wildcard`/`prefix`/`ids`/`query_string`.
- **ES and OS request bodies are byte-identical over this subset.** These are pre-fork core DSL that OpenSearch inherited unchanged; the fork diverged on *management* APIs (templates, ILM/ISM, data streams), which the query AST does not touch — so the AST carries essentially no ES-vs-OS branching.
- **The only frictions are version gates within the ES lineage, all already centralized behind `BackendVersion`:** `range` emits only `gt/gte/lt/lte` (ES9-safe), `date_histogram` uses `fixed_interval` (ES8+), `hits.total` is normalized via `rest_total_hits_as_int`, `_type` is suppressed for ES7+/OS (write path). None add branching to the query JSON.

The AST is **built, not borrowed.** `internal/storage/elasticsearch/query/range_query.go` already demonstrates the pattern — a hand-written, ES9-safe range node with a `Source() (any, error)` returning `map[string]any`. Reusing `olivere`'s builders as a pure-JSON DSL is possible (their `Source()` methods have no client dependency) but strictly worse: it keeps a dead dependency, still requires overriding `RangeQuery`, and re-leaks the `elastic.*` type across the boundary. `go-elasticsearch`'s `esdsl` (ES-spec-only, product-checked, v8/v9-only) and `opensearch-go` (no builder) are unfit for a driver-neutral boundary.

The AST covers **only the search/aggregation layer**. The genuine ES/OS divergence lives in the management APIs and is branched deliberately elsewhere in `esclient` (via `UsesV8API`/ISM), never inside the query AST.

**Responses are symmetric to the request AST.** `esclient` returns its own response type — ES-*shaped* (hits / total / aggregation buckets) but trace-domain-agnostic: it never knows what a span is, so `_source` document bodies come back as raw `json.RawMessage` for the storage reader to unmarshal into `dbmodel`. No `elastic.SearchResult` crosses the boundary, just as no `elastic.Query` does. (How fully typed that response struct is — versus `map[string]any`, with heterogeneous aggregation buckets the awkward part — is an implementation detail, not an architectural choice.)

The driver-*independent* read/write logic (trace assembly, tag processing, metrics math, pagination) already lives **above** this boundary, in the storage reader/writer, and stays there. Because this design keeps a single backend-agnostic implementation (no ES/OS fork), that logic does **not** need extracting into shared packages; only the reader's driver-*coupled* parts change, in place: query construction (`elastic.Query`/`elastic.Aggregation` → the AST) and response parsing (`elastic.SearchResult` → the owned response type). The reader keeps its shape; its `es.Client` dependency is swapped for `esclient`. The smaller the driver-specific core below the interface, the cheaper the client is to own (§5, K8).

### 6.3 What we keep from the SDKs

Nothing from the product-checked *client* or the typed API — but the **transport** is worth reusing. `elastic-transport-go` (the layer *beneath* `go-elasticsearch`) is Apache-2.0, already a transitive dependency, and — verified — carries no product check (that gate lives in the `go-elasticsearch` client above it). It provides the connection pool, multi-node round-robin, failover, retry, and opt-in discovery; our own `GetHTTPRoundTripper` (TLS + auth + **SigV4** via the OTel `sigv4auth` extension + `GetBody` fix + header-forwarding) plugs in as its base transport, so signing/headers remain ours. This is "SDK *transport*, not SDK *client*" — fully consistent with Option A, and it preserves the multi-node behavior a plain single-endpoint transport would lose (§6.1). The components still owned outright are the query AST, the response types, and the bounded bulk indexer.

### 6.4 Package: `esclient` is the renamed `client` package, grown upward

**Recommendation:** `esclient` is the former `internal/storage/elasticsearch/client` package **renamed** (M2), not a greenfield package built alongside it. That package is already the right foundation — a raw-HTTP, driver-neutral ES/OS client with a strong `httptest`-based test suite and the small `IndicesClient`/`ClusterClient`/`ILMClient` structs this RFC wants to generalize. We keep its structs and tests and **grow the data-plane surface (`Searcher`, `BulkWriter`) into the same package**, over the shared `rawClient` (§6.1).

This framing matters: `esclient` becomes **the foundation of Jaeger's own ES/OS SDK** — the single place that owns wire format, versioning, auth, and the neutral query DSL — rather than a second client bolted next to the old one. Renaming (not rewriting) also means the migration starts from a green, tested baseline: the control-plane behavior is preserved by construction, and the data plane is added incrementally under the snapshot suite (§7). It also disposes of the old data/control-plane split at the package level, not just the interface level.

### 6.5 The client structs, layered — today and target

The design above (§6.1–6.4) is easier to hold as one picture. Two follow: today's **transitional** shape (Stage A — the admin plane already rides the shared pool, the data plane still runs on `olivere`), and the **target** shape (post Stage B — one shared client).

**Today (Stage A): two stacks meeting at one transport.**

```
   DATA PLANE  (storage read/write)                 ADMIN PLANE  (es-rollover, es-index-cleaner)
   spanstore · tracestore · depstore                init · rollover · lookback · index-cleaner
   samplingstore · metricstore
        │ depends on                                     │ depends on
        ▼                                                ▼
   es.Client  (one fat interface:                   esclient role interfaces:
   Search, Bulk, CreateTemplate, GetVersion…)       IndexAPI · ClusterAPI · IndexManagementLifecycleAPI
        │ implemented by                                 │ implemented by
        ▼                                                ▼
   ClientWrapper {                                   IndicesClient · ClusterClient · ILMClient
     olivere/elastic v7  *elastic.Client,             (each embeds esclient.Client)
     go-elasticsearch v9 *esv8.Client,                     │
     *elastic.BulkProcessor }                              ▼
        │ built by clientbuilder.NewClient           esclient.Client { *rawClient, version }
        │                                                  │
        │ olivere keeps its OWN round-robin                ▼
        │ (adopts the pool in Stage B)                rawClient { elastic-transport-go pool:
        │                                               multi-node round-robin, retry, discovery-off }
        └────────────────────┬─────────────────────────────┘
                             ▼
        shared base transport:  GetHTTPRoundTripper
        (TLS · basic/bearer/API-key · SigV4 · GetBody fix · custom_headers)
                             ▼
                  Elasticsearch / OpenSearch REST
```

The two implementation stacks are independent above the transport and converge only at `GetHTTPRoundTripper`. The data plane reaches ES/OS through the fat `es.Client` interface implemented by `ClientWrapper` (`olivere` v7 + `go-elasticsearch` v9 + a bulk processor); the admin plane reaches it through small role interfaces implemented by `IndicesClient`/`ClusterClient`/`ILMClient` over the `elastic-transport-go` pool. `olivere` keeps its own round-robin, so it cannot sit on the pool yet (the decisive constraint from §6.1) — the admin plane adopts the pool in M3, the data plane in Stage B.

**Target (post Stage B): one shared client.**

```
   STORAGE  (Jaeger domain)                          ADMIN  (es-rollover · es-index-cleaner)
   tracestore · spanstore · depstore                 init · rollover · lookback · index-cleaner
   samplingstore · metricstore
        │ builds query AST, decides indices               │
        ▼ (depends on small role interfaces)              ▼ (depends on small role interfaces)
   Searcher · BulkWriter        IndexManager · AliasManager · TemplateManager · LifecycleManager · ClusterInfo
        └───────────────┬─────────────────────────────────────────┬───────────────┘
                        │   implemented by cohesive API structs (composition, no god object)
                        ▼                                          ▼
                 Data{ esclient.Client }                   Admin structs{ esclient.Client }
                 Search · MultiSearch · Bulk               Index · Alias · Template · Lifecycle · Cluster
                 owned query AST → wire JSON
                        └────────────────────┬────────────────────┘
                                             ▼
                 esclient.Client{ *rawClient }  — request primitive (timeout · JSON · errors);
                   holds the resolved backend version + capability gates (UsesV8API · ISM-vs-ILM ·
                   rest_total_hits_as_int · typed-index), set once at construction, never exposed up
                                             ▼
                 rawClient{ elastic-transport-go pool }  — multi-node round-robin · retry · discovery-off
                                             ▼
                 GetHTTPRoundTripper  — TLS · basic/bearer/API-key · SigV4 · GetBody fix · custom_headers
                                             ▼
                               Elasticsearch / OpenSearch REST
```

`olivere`, `go-elasticsearch`, `ClientWrapper`, and the fat `es.Client` interface are gone. The `esclient.Client`-over-`rawClient` split stays — `Client` is the request primitive (and holds the resolved version + capability gates), `rawClient` the transport pool — but now one such base underlies *both* a `Data` struct (search/bulk) and the admin structs, all behind small role interfaces, so storage readers and CLIs alike depend only on narrow, Jaeger-vocabulary contracts. Auth/signing, the owned query AST, and — crucially — backend-version resolution and its capability gates all live **below the interface line**: the version is resolved once at construction and never surfaces to a caller (the M4 objective, §8).

---

## 7. Testing strategy

The current tests do not give us the confidence a driver swap requires, and this is the single most important enabler of the migration.

### 7.1 What we have (assessment)

- **Data-plane `olivere` mocks — mostly coverage-filler.** Generated for `es.Client` and every fluent service interface. In practice, reader/writer tests match `Query` with `mock.Anything` and assert the fluent call *sequence* the code just made — a tautology coupled to the implementation. They exercise **response deserialization** (real, narrow value) but **never assert the query DSL actually sent.** A query regression passes today.
- **Control-plane tests — genuinely valuable.** `esclient/*_test.go` stand up an `httptest.Server` and assert real HTTP: method, path, auth header, query params, URL-length batching, error handling. Keep and extend this pattern.
- **Integration matrix — the real safety net.** `internal/storage/integration/*` drives a live cluster across **ES 6–9 and OpenSearch 1–3** via docker-compose + CI. This is the only layer that validates query semantics, mappings, and ILM/rollover against a real backend.

### 7.2 What we adopt: snapshot testing of the wire format

The migration *is* a change to the code that serializes queries. So pin the wire contract:

- For each storage operation, point the client at a recording `httptest.Server` and snapshot the exact **`{method, path, sorted query params, canonicalized JSON body}`** to a committed `testdata/*.json` snapshot. NDJSON (`_bulk`/`_msearch`) handled as multi-doc; timestamps via injected fixed clock; JSON canonicalized (sorted keys) for determinism.
- **Property:** after swapping `olivere` → owned client, a **green snapshot diff means the change is behavior-preserving on the wire.** Every diff is exactly the bytes that changed — reviewable in the PR.
- **Backend divergence becomes reviewable:** parameterize by backend/version so ES and OS emit separate snapshots (`testdata/find_trace_ids.es8.json`, `testdata/find_trace_ids.os1-2.json` — naming per §7.3). ILM-vs-ISM, template-endpoint, typed-index differences appear as concrete diffs instead of hidden branches — directly serving G2's "no leakage" as an auditable artifact.
- **Precedent — closest to home: index mappings.** `internal/storage/v1/elasticsearch/mappings/` already does exactly this for ES payloads: it renders each template and asserts it against committed snapshot fixtures **parameterized by backend × version** — `fixtures/jaeger-{span,service,dependencies,sampling}-{6,7,8}.json` for Elasticsearch, plus a separate `TestMappingBuilderGetMapping_OpenSearch` for `OpenSearch1/2` (`mapping_test.go`). That is the same "one snapshot per backend/version, full-JSON compare" model this section proposes, just applied to *mapping* JSON rather than *request* JSON — so the pattern is proven and idiomatic here, not new. (Other in-repo snapshot users: metricstore responses, apiv3 gateway.)

**Caveats (stated honestly):** snapshots assert what we *send*, not that the server accepts it or returns correct results — they are **complementary to**, not a replacement for, the integration matrix (the authority on semantics/version behavior). The `-update` regeneration flow needs review discipline: a wrong query change is easy to rubber-stamp when the tool rewrites the snapshot. That is the one real risk and reviewers must diff snapshots deliberately.

### 7.3 Fixture naming taxonomy (converge all snapshots)

Two problems with today's snapshots: the ES fixtures use an ad-hoc scheme (`jaeger-span-7.json`), and — worse — **version overlap lives in code, not in the names**. When ES 6 and ES 7 share a mapping, the fixtures don't say so; a `v <= 7` branch does. Which versions share a snapshot is invisible from `ls`. As part of this refactor, converge **every** ES/OS snapshot — the migrated mapping fixtures *and* the new request snapshots — on one scheme.

**Pattern:**

```
testdata/<subject>[.<variants>].json
```

- `<subject>` — the operation/artifact in snake_case: `find_trace_ids`, `get_services`, `write_span`, `create_template`, `rollover`, `alias_exists`, `span`, `service`, … `<subject>` may nest with `/` to group a family, but only when the enclosing directory does not already imply it — a mapping snapshot under `mappings/testdata/` is `dependencies.es7.json`, not `mapping/dependencies.es7.json`.
- **There is exactly one file per distinct wire format.** When every supported backend and version emits the same request, the variant tail is omitted and the snapshot is the bare `<subject>.json`.
- **Otherwise a `.<variants>` tail lists the version ranges that share that wire format** — a dot-separated list of `<backend><range>` tokens:
  - `<backend>` — `es` or `os`.
  - `<range>` — a single major (`8`) or an **inclusive major range** (`6-7`) of consecutive versions.
  - Backends that emit byte-identical output share one file, so the token list can span both — `get_services.es7-9.os1-3.json`.

Examples: `testdata/alias_exists.json` (all versions); `testdata/create_template.es7.os1-3.json` + `testdata/create_template.es8-9.json` (ES 7 and OpenSearch share the `_template` endpoint, ES 8-9 use `_index_template`); `testdata/span.es8-9.json` (mapping distinct per backend).

**Rules:**

- **The variant set is content-derived, not backend-derived.** Regeneration groups versions by the exact bytes they emit and writes one file per group, naming it with every range in the group. A resolver maps `(backend, major) → the unique file one of whose ranges contains it`, and every supported major must resolve; two files claiming the same major is a test error. This **replaces `v <= 7`-style branches with data in filenames** — "which versions share this request?" is answered by `ls testdata/`.
- **No duplication.** Two backends (or majors) that emit identical bytes are never stored twice; they collapse into one file whose name enumerates both.
- **Version changes are reviewed diffs, never silent.** Adding a supported major: regenerate; if its output matches an existing group its range extends or merges (`.es7-9.` → `.es7-10.`, or `.es8-9.json` → `.es8-9.os1-3.json`); if it differs, a new file appears. Coverage is always visible in the diff.
- **The bare `<subject>.json` is the explicit claim "byte-identical on every backend and version."** Common in the admin plane (e.g. `HEAD /_alias/{name}`, whose client code has no backend/version branch), rare in the query plane. The self-describing bare name (not an `any` token) keeps it honest.

**Why one file per wire format (not per backend).** Always giving ES and OpenSearch their own file is simpler to resolve but duplicates every backend-agnostic request — and since the data plane is backend-agnostic by design, that is most of them. A third option — a base file plus per-version overrides — reads best for the data plane but makes a file's coverage implicit. Trade-offs (🟢 good / 🟡 mixed / 🔴 poor):

| Criterion | ① One file per wire format<br>`es7-9.os1-3.json` **(chosen)** | ② Base + overrides<br>`.json` + `<variant>.json` | ③ Separate per backend<br>`es7-9.json` + `os1-3.json` |
|---|:---:|:---:|:---:|
| Eliminates duplication | 🟢 identical content → one file | 🟢 identical content → base | 🔴 `es7-9` == `os1-3` duplicated |
| Coverage is explicit | 🟢 filename lists every range | 🔴 base = "whatever isn't overridden" | 🟢 one file per backend |
| Keeps `bare.json` = "all identical" | 🟢 unchanged | 🔴 redefines bare as "default" | 🟢 unchanged |
| Reads cleanly for the data plane | 🟡 `es7-9.os1-3` is busy | 🟢 "the query, + one variant is special" | 🔴 one file is a pure duplicate |
| Reads cleanly for the control plane (backends differ) | 🟢 one explicit file each | 🟡 one variant becomes the implicit base | 🟢 one explicit file each |
| Filename grammar simplicity | 🟡 dotted range-list | 🟢 simplest | 🟢 simplest |
| Upkeep when a new ES/OS major is supported | 🟡 regenerate; range extends/merges in the name | 🟢 matches the base ⇒ no new file | 🔴 always a new (often duplicate) file |
| Scales as the data plane grows | 🟢 | 🟢 | 🔴 duplication multiplies |

① and ② both eliminate the duplication; they differ on where the cost lands. ② has the lowest upkeep and the cleanest data-plane read, but a file's coverage becomes implicit and the bare-file meaning is overloaded — awkward for the control plane, where no variant is a natural default. We choose **①**: an unambiguous resolver and every file's coverage being explicit in its name matter more here than shaving a regeneration step, and the fixture tree stays a literal, no-magic compatibility matrix.

The payoff: **the fixture tree *is* the compatibility matrix.** One convention spans mappings and request snapshots; converging the existing `jaeger-*-{7,8}.json` files onto it collapses any accidental duplication (identical adjacent-version files merge into a range, and identical ES/OS files merge into one `.es*.os*` file). This convergence is milestone M1 (§8) so the baseline lands in the final naming.

### 7.4 Snapshot vs. focused mock — pick the altitude

Snapshots and mocks are not competitors; they answer different questions, and using the wrong one makes tests verbose *and* less clear. **The subject of the test picks the tool:**

- **Assert the wire format → snapshot.** When the test *is about* the serialized request — query DSL structure, aggregation shape, op-type (`index` vs `create`), `_type` suppression, NDJSON framing — a snapshot is the right, self-documenting artifact. Budget **one snapshot per distinct request *shape***, not per input value.
- **Assert a Jaeger-level decision → focused mock/spy on a small interface.** When the test is about a value the code *computes* — "given time range `T`, did we query indices `[jaeger-span-2024-01-01 … -01-03]`?", "is `IgnoreUnavailable` set?", "did the service cache dedupe the write?", "did the `search_after` cursor advance?" — capture that argument through a narrow fake and assert it directly.

This is exactly where the **small role interfaces (§6.1) pay off.** A one-method `Searcher` fake that records its `(indices, query)` arguments lets a test assert *index selection* in one line and ignore the query body entirely.

> **Worked example (the motivating case).** A method takes `1..N` index names, and we have `M` tests covering the *index-selection* logic across time ranges/rotation modes. Writing `M` full-body snapshots is verbose and **obscures intent** — a reader can't tell whether the test is about the index list or the wire JSON, and every unrelated query tweak churns all `M` files. Instead: **one snapshot** pins the request shape for that operation, and a **table of `M` focused assertions** on the captured `indices` argument covers the selection logic. Right altitude, minimal noise, intent obvious.

Anti-patterns this rules out: (a) a snapshot per input permutation of the same query shape (snapshot sprawl); (b) hand-asserting a whole request body when the test cares about one field; (c) re-mocking the query *builder* to re-check the wire format — that was the coverage-filler failure mode of the current `olivere` mocks (§7.1), and it's what snapshots replace.

### 7.5 Sequencing the tests

**Build the snapshot suite against the current `olivere` client first**, freezing today's wire behavior as the baseline. Then the migration is "make the new client reproduce these snapshots" — and the fluent-mock query tests can be retired as low-value. Net testing model:

1. **Snapshot** — request wire-format, hermetic, per backend/version.
2. **Response-parsing unit tests** — keep the genuinely-useful half of today's mocks.
3. **Live integration matrix** — semantics/version authority; unchanged.

---

## 8. Migration plan

The work decomposes into small, independently-shippable milestones — each one PR-sized, guarded by the snapshot + integration suites, with an explicit exit bar. They group into four stages; within the data-plane stage each storage path migrates on its own so no single PR is large. The snapshot suite (M1) is what makes the per-path migrations safe and small: each is "migrate this path, snapshots stay green."

**Stage A — Foundation (no behavior change)**
- **M1 — Snapshot baseline + fixture taxonomy. ✅ Done (#8921, #8922, #8929).** Add the request-snapshot suite (§7.2) over the *current* clients; converge existing snapshots onto §7.3 naming. *Exit:* every data-plane and admin operation has a snapshot resolving for each supported backend/version in §7.3 naming; CI runs it; diff is tests + fixtures only. (Carve-out: the sampling `InsertThroughput`/`InsertProbabilitiesAndQPS` writes stamp the document body with `time.Now()` internally and have no clock seam, so their bodies are frozen during the migration when a fixed clock is injected, not in the baseline.)
- **M2 — Rename `client` → `esclient`. ✅ Done (#8930).** Mechanical package rename (§6.4), imports updated. *Exit:* `internal/storage/elasticsearch/client` gone; all tests green; zero behavior change.
- **M3 — One shared transport for *both* planes (admin + data).** Establish the shared `rawClient` transport (`GetHTTPRoundTripper` layered under `elastic-transport-go`'s pool) and route every request through it — the admin structs (`IndicesClient`/`ClusterClient`/`ILMClient`) *and* the existing data-plane client — so TLS/auth/SigV4/`custom_headers` are applied in one place for all traffic. *Exit:* admin **and** data-path requests all carry SigV4/bearer/API-key/`custom_headers`, proven by httptest — closing the admin gap in `es-rollover`'s `newESClient`. **Pool adoption:** the admin plane adopts the `elastic-transport-go` pool in M3; the data plane keeps olivere's own round-robin and adopts the pool in Stage B when olivere is replaced (olivere exposes only an `*http.Client`/`RoundTripper` and already round-robins, so it cannot sit on the pool until then — the M3 data plane simply *shares the RoundTripper stack*). Delivered as self-contained PRs:
  - **M3.1 — Fix SigV4 body signing. ✅ Done (#8768).** `getBodyFixRoundTripper` now wraps the authenticator on the outside, so `req.GetBody` is populated before signing. Fixes **#8760** (body-bearing writes were signed as empty → 403).
  - **M3.2 — Apply `custom_headers` + `Host` in the shared RoundTripper. ✅ Done (#8917).** One header-injecting layer covering the olivere v7, go-elasticsearch v8, and admin paths (`Host` via `req.Host`); removes the v8-only header block. Fixes **#8916** (headers reached only the v8 client).
  - **M3.3 + M3.4 — Introduce `rawClient` over the `elastic-transport-go` pool and route the admin `esclient.Client` onto it.** New `esclient/transport.go`: a multi-node connection pool (round-robin, failover, sniffing off, retry off for byte-parity) over an injected RoundTripper stack; `esclient.Client` composes over it, so `es-rollover`/`es-index-cleaner` run through the pool — exercised by their real-DB integration tests. Behavior-preserving (TLS + basic, single endpoint): the M1 admin snapshots stay byte-identical, proving the refactor is wire-preserving.
  - **M3.5 — Full admin auth + CLI config.** Give the admin plane the full auth stack, delivered in three steps:
    - **M3.5a — Relocate the RoundTripper stack. ✅ Done (#8936).** Move `GetHTTPRoundTripper` + `getBodyFix` + `customHeaders` + auth helpers from `clientbuilder` into `esclient` (no cycle, olivere-free); `clientbuilder` calls into `esclient`. Pure move, zero behavior change.
    - **M3.5b — Wire `esclient.NewClient` through the stack. ✅ Done (#8937).** `NewClient` takes a `*config.Configuration` and builds its base via `GetHTTPRoundTripper`, so the admin plane inherits TLS + basic/bearer/API-key/`custom_headers`; `Client.basicAuth`/`setAuthorization` and the `esclient.BasicAuth` helper are removed (auth lives in the stack). The M1 admin snapshots stay byte-identical.
    - **M3.5c — CLI auth config. ✅ Done (#8939).** Add `--es.token-file` (bearer) and `--es.api-key-file` flags to `es-rollover`/`es-index-cleaner`, mirroring the retired v1 ES flag names, so those CLIs can authenticate to token/API-key-secured clusters. `custom_headers` stays YAML-only (it never had a CLI flag and these binaries have no YAML path); reload-interval and from-context knobs are omitted since the CLIs are one-shot. (Standalone SigV4 for the CLIs is a follow-up — the `sigv4auth` extension is collector-host-only.)
- **M4 — Encapsulate the backend version.** Resolve the version once — an explicit `config.Version` override, else a single ping through one shared `es.ResolveBackendVersion` — and inject it into the client at construction. From there it is **fully encapsulated**: no business-logic-facing surface exposes or accepts a `BackendVersion`. Version-dependent choices (`_template` vs `_index_template`, ILM vs ISM, `rest_total_hits_as_int`, typed-index suppression) live *inside* the client/domain APIs; callers invoke them in Jaeger terms (§6.5, "below the interface line"). *Exit:* (1) exactly one version-resolution path; (2) **no `Version()` accessor, no `Version` field, and no `BackendVersion` parameter on any caller/orchestrator** (e.g. `es-rollover`'s `init.Action`) — the CLIs say "create the templates" / "ensure the policy" without ever holding a version; (3) the `UseOpenSearchISM` type-assertion in `cmd/es-rollover/app/init/action.go` is gone, the ISM-vs-ILM choice living inside a version-aware `ILMClient`. *(The "one detection path" framing alone was too weak — it's satisfied by relocating the leak; the real bar is non-exposure.)*

  Delivered incrementally:
  - **M4a — Version resolution + admin encapsulation (callback). ✅ Done (#8938).** `esclient.NewClient` resolves the backend version at construction — honoring `config.Version`, else probing once via the shared `es.ResolveBackendVersion` (the data-plane `clientbuilder` uses the same resolver) — and stores it on an unexported `Client.version` (the low-level `GET /` probe is `Client.ping`; there is no post-construction override). Version-dependent admin methods read it internally: `IndicesClient.CreateTemplate` takes a version-receiving render callback (so `es-rollover` `init` selects the mapping type but never holds a `BackendVersion`), and the ILM-supported gate is the capability method `ILMClient.SupportsILM()`. The `Version()` accessor and `init.Action.Version` are gone, and `ClusterClient` (which only wrapped version detection) is removed. The callback is a **transitionary** encapsulation: the caller no longer stores or branches on the version, but the mapping is still rendered by the app-layer `mappings` package (invoked with the client's own version). Meets exit-(1), exit-(3), and exit-(2) for the admin plane.
  - **M4b — `esclient` owns index templates.** Collapse the per-version `jaeger-*-{7,8}.json` files — whose *only* differences are wire-envelope gates (`template` vs `index_patterns`, the ES8 composable wrapper), not the field schema — into a single neutral template representation owned by `esclient`, rendered to the per-version envelope internally. `CreateTemplate` then takes pure Jaeger intent (mapping type), retiring the callback — fully symmetric with the query AST. This closes exit-(2) on the data plane too (retiring `es.Client.GetVersion` consumption in `factory`/`mappings`), and naturally lands with Stage B. `BackendVersion.TemplateVersion()` is removed here as well: it exists only to select the per-version `<mapping>-N.json` file in `mappings`, so once `esclient` renders the envelope internally it has no remaining caller.

**Stage B — Migrate storage paths, growing the API on demand (one PR per path).** Each slice is *vertical*: it adds only the AST nodes, response fields, and bulk features its caller needs, and migrates that caller in the same PR — so the caller's snapshot + integration tests validate the new API immediately. There is no unvalidated client layer sitting ahead of its users; a design flaw in the AST or response type surfaces in the first slice that hits it, not three PRs later. The first read and first write slices carry the scaffolding (the AST core, the response type, the bulk indexer); later slices are small deltas. Every slice's exit bar is "this path's snapshots stay green and its integration passes."

The **small role interfaces (§6.1) are what make this clean**: a slice introduces just the interface its caller needs (`Searcher` in M5, `BulkWriter` in M6, …), and each caller depends only on its own narrow interface — so slices don't touch each other's surface and, apart from the two that bootstrap shared scaffolding, can proceed in parallel. A single fat `DataAPI` would have coupled every path to one growing interface and serialized the work.
- **M5 — Service/operation read (first read slice; bootstraps `Searcher` + AST core).** ✅ Done ([#8943](https://github.com/jaegertracing/jaeger/pull/8943)). Introduces the AST's `term` query + `terms`-aggregation nodes (alongside the pre-existing `range` query) and the owned response type (terms buckets), migrating the `getServices`/`getOperations` search path onto `esclient.Searcher` over the shared transport pool. The write and trace-read paths stay on `olivere` for later slices. *Exit:* service/operation snapshots byte-identical; the new AST nodes and response fields are exercised by real caller tests, not stubs.
- **M6 — Span writer (first write slice; bootstraps `BulkWriter` + bounded bulk indexer).** ✅ Done ([#8944](https://github.com/jaegertracing/jaeger/pull/8944)). Introduces the narrow `esclient.BulkWriter` (`Add` only) and a `BulkIndexer` that wraps the official `esutil.BulkIndexer` driven by **our** transport pool, and migrates the span + service:operation write paths onto it. *Exit:* span-write snapshots byte-identical; bounded bulk memory (#2192); write integration green.

  **Decision (during M6 review): use the official `esutil.BulkIndexer`, not a hand-rolled one.** M6 first shipped a from-scratch bounded indexer; review established that `esutil.BulkIndexer` (`go-elasticsearch/v9`, Apache-2.0, already a dep) takes an `esapi.Transport` — a bare `Perform(*http.Request)` interface our `elastic-transport-go` pool already satisfies — so it runs on **our** transport with **no product-checked `*elasticsearch.Client`**. It is battle-tested, handles the buffering/flush/#2192 byte-cap itself, and its `OnSuccess`/`OnFailure` callbacks feed the `bulk_index` metrics.

  | Criterion | A: hand-write | B: use `esutil` ✅ | C: fork `esutil` |
  | --- | --- | --- | --- |
  | Production-tested | 🔴 new | 🟢 upstream | 🟡 forked |
  | Code we maintain | 🟡 ~250 lines | 🟢 config + glue | 🔴 ~700 lines |
  | Upstream bug fixes | 🔴 none | 🟢 automatic | 🔴 manual |
  | ES6 typed bulk¹ | 🟢 emits `_type` | 🔴 typeless-only | 🟡 refork |

  🟢 good · 🟡 partial · 🔴 poor. ¹ `esutil` is typeless-only and ES6 `_bulk` requires `_type` (verified: ES 6.8.23 rejects a typeless `_bulk` with `HTTP 400 "type is missing"`), so B required removing ES6 first ([#8948](https://github.com/jaegertracing/jaeger/pull/8948)). Consequence: `go-elasticsearch` stays a **transport-level** dependency; M11 is narrowed to removing its product-checked *client*.
- **M7 — Span reader** (find-traces / find-trace-IDs / get-trace). Extends the AST with `nested`/`regexp`/`range`/`match`, `search_after`, `_msearch`. *Exit:* reader snapshots green; find-traces integration green across the matrix.
- **M8 — Dependency store.** *Exit:* snapshots green.
- **M9 — Sampling store.** *Exit:* snapshots green.
- **M10 — Metricstore.** Extends the AST with `date_histogram`/`percentiles`/`cumulative_sum`/`filter`/`top_hits` aggregations. *Exit:* metrics snapshots green; metrics integration green.

**Stage C — Cleanup**
- **M11 — Retire `olivere`.** Delete `olivere` + the `go-elasticsearch/v9` template special-case (now unused, since every caller moved in Stage B). *Exit:* no `github.com/olivere/elastic` or `go-elasticsearch` import under `internal/storage`; no `elastic.*` in any Jaeger signature (§2.1 leak closed); full ES 6–9 / OS 1–3 matrix passes.

Backward-compatibility integration tests across backends ([#8691](https://github.com/jaegertracing/jaeger/issues/8691), [#8896](https://github.com/jaegertracing/jaeger/pull/8896)) protect the version matrix throughout. Driver-independent extraction PRs in flight ([#8538](https://github.com/jaegertracing/jaeger/pull/8538), [#8503](https://github.com/jaegertracing/jaeger/pull/8503)) are complementary but **not** prerequisites — this design keeps one implementation, so no logic needs relocating to share between backends.

**Out of scope (follow-up this enables):** folding index-management bootstrap into the storage factory to retire the standalone `es-rollover`/`es-index-cleaner` tools. That is orchestration, not the client, and belongs to the data-streams effort ([RFC 0004](./0004-elasticsearch-data-streams.md) / #4708); this RFC only makes those tools *use* the unified client. Sequencing of the two efforts is the one remaining open question (§9).

---

## 9. Open questions

1. **Coupling to data streams.** Combined single refactor, or client-first-then-data-streams? This RFC assumes client work *enables* data streams and can precede/parallel it, but the sequencing needs owner sign-off (see #7612 discussion).

---

## 10. Relationship to prior proposals

This RFC builds on the investigation in [#7612](https://github.com/jaegertracing/jaeger/issues/7612) — principally @thc1006's client survey and @Amaan729's research PR [#8205](https://github.com/jaegertracing/jaeger/pull/8205), plus the driver-independent extraction PRs from @madhav-murali/@hharshhsaini ([#7917](https://github.com/jaegertracing/jaeger/pull/7917), [#8538](https://github.com/jaegertracing/jaeger/pull/8538), [#8503](https://github.com/jaegertracing/jaeger/pull/8503)). It reaches a different architectural conclusion, for reasons worth stating explicitly since a reviewer arriving from #7612 will ask them.

### The core divergence: one owned client vs. two official SDKs

The community investigations converged on a **dual-client** strategy: adopt `go-elasticsearch` for Elasticsearch and `opensearch-go` for OpenSearch, dispatch by detected backend behind a facade. @thc1006 recommended exactly that ("go-elasticsearch/v9 for ES, opensearch-go/v4 for OS, runtime detection"); #8205 proposed `es/` and `os/` sub-packages selected at runtime.

This RFC evaluates that approach as **Option B (§5)** and does not adopt it. It recommends **Option A** — a single Jaeger-owned, driver-neutral client that owns its wire JSON. The matrix in §5.3 is the argument: the dual-client path is two code paths forever, ships two near-duplicate SDK forks (`opensearch-go` is a fork of `go-elasticsearch`), narrows the version matrix (a single `go-elasticsearch` major cannot span ES 6→9; `opensearch-go` cannot reach ES 8/9), and does not actually deliver "one client" — it delivers two behind a curtain.

### Two findings that change the conclusion

The dual-client proposals leaned on the official SDKs largely on the assumption that Jaeger needs their machinery (transport, signing, bulk, retries), and priced the work accordingly (~8–12 weeks in @thc1006's estimate). Two facts, established here and not load-bearing in the prior investigations, undercut that assumption:

1. **The transport is solvable without the SDK *client* (§6.1, §6.3).** Auth/SigV4/headers already exist as `clientbuilder.GetHTTPRoundTripper`, and the connection-pool / round-robin / retry machinery is reusable from `elastic-transport-go` — the *product-check-free* layer beneath `go-elasticsearch` — with our RoundTripper as its base. So the dual-client premise that you must adopt the SDK *clients* to get transport machinery does not hold: you get the battle-tested transport *without* the product-checked client. (And the admin path bypassing our transport today is a pre-existing SigV4/bearer gap, not merely duplication.)

2. **The query DSL is byte-identical across ES and OS over Jaeger's actual subset (§6.2).** The ES/OS fork diverged on *management* APIs (ILM/ISM, templates, data streams), not the search DSL. So a small (~17-node) owned AST hides all backend differences with essentially no branching. This makes "own the query layer" cheap rather than the large rewrite the dual-client framing implied.

Neither the "40–60% shareable, keep a facade" analysis nor the dual-client proposals rested on these two points; together they are what justify *not forking the implementation at all*.

### Other improvements over the prior proposals

- **Version-matrix preservation is a first-class goal (G3).** The dual-client path would *narrow* support; this RFC keeps ES 6/7/8/9 + OS 1/2/3 from one binary and treats any regression as a failure condition.
- **A concrete migration safety net (§7).** Freeze today's wire behavior as request **snapshots** first, then migrate under green snapshots — plus an honest audit that the current `olivere` mocks are largely coverage-filler, and a single fixture-naming taxonomy. The prior discussion got as far as "raw JSON vs typed API, lean on integration tests"; it did not propose a wire-contract baseline.
- **The facade altitude is scoped correctly (§6.2).** Per the #7612 steer toward "Jaeger concepts, not driver concepts," but `esclient` is ES-primitive and driver-neutral — trace-domain methods like `FindTraceIDs` stay in the *storage* layer. The earlier "facade" discussion blurred these levels.
- **The extraction phase is shown to be unnecessary.** The proposals treated "extract the 40–60% driver-independent logic into shared packages" as a prerequisite. That only pays off if the implementation is *forked* per backend. Because this design keeps one implementation, that logic stays where it is; the in-flight extraction PRs are complementary, not gating.
- **PR-sized, vertical, snapshot-guarded milestones with exit criteria (§8)**, and tighter scope — folding index management into the factory is explicitly out of scope (data-streams territory), which the prior proposals tended to bundle in.

### What it keeps from the community

This is not a replacement for the prior work. The **product-check finding** — that no single official Go SDK can serve both current Elasticsearch and OpenSearch — is @thc1006's, adopted wholesale and central to §4. The **#2192 / bounded-bulk priority** is kept. The existing `esquery.RangeQuery` is cited as the working AST prototype (§6.2). The extraction PRs are credited as complementary cleanups (§8).

**In one line:** the community concluded "adopt two official SDKs and fork the implementation"; this RFC concludes "own one driver-neutral client and don't fork" — a conclusion that only becomes correct once you notice the transport is already Jaeger's and the query DSL does not actually diverge across backends.

---

## 11. References

**Jaeger issues/PRs**
- [#7612](https://github.com/jaegertracing/jaeger/issues/7612) — Investigate the path to replace `olivere/elastic` (tracking issue with the full design discussion)
- Prior community investigations (referenced from #7612):
  - @thc1006 — [ES/OS client investigation + analysis](https://github.com/thc1006/jaeger/tree/research/jaeger-7612/docs/jaeger-7612) (fork research branch; found ~40–60% driver-independent code, recommended dual-client)
  - @Amaan729 — [research report & migration roadmap, PR #8205](https://github.com/jaegertracing/jaeger/pull/8205) (`docs/elasticsearch-client-migration.md`: candidate-client comparison + method-mapping table; ILM-vs-ISM the one hard gap)
  - @madhav-murali / @hharshhsaini — Phase-1 driver-independent extraction ([#7917](https://github.com/jaegertracing/jaeger/pull/7917), [#8538](https://github.com/jaegertracing/jaeger/pull/8538), [#8503](https://github.com/jaegertracing/jaeger/pull/8503))
- [#4708](https://github.com/jaegertracing/jaeger/issues/4708) / [RFC 0004](./0004-elasticsearch-data-streams.md) — Data Streams
- [#2192](https://github.com/jaegertracing/jaeger/issues/2192) — unbounded bulk memory
- [#8916](https://github.com/jaegertracing/jaeger/issues/8916) — `custom_headers` dropped on olivere data path; fix PR [#8917](https://github.com/jaegertracing/jaeger/pull/8917)
- [#8760](https://github.com/jaegertracing/jaeger/issues/8760) / [#8307](https://github.com/jaegertracing/jaeger/issues/8307) — SigV4 body signing; fix PR [#8768](https://github.com/jaegertracing/jaeger/pull/8768)
- [#8842](https://github.com/jaegertracing/jaeger/pull/8842) — clientbuilder extraction (merged)

**External**
- go-elasticsearch product check — [`elasticsearch.go`](https://github.com/elastic/go-elasticsearch/blob/main/elasticsearch.go); opt-out refused — [elastic/elasticsearch#73424](https://github.com/elastic/elasticsearch/issues/73424)
- `elastic-transport-go` (Apache-2.0; connection pool, round-robin, retry, discovery; no product check) — [repo](https://github.com/elastic/elastic-transport-go) · `Config.Transport` custom base RoundTripper + `Client.Perform`
- opensearch-go (fork of go-elasticsearch 7.10.2) — [repo](https://github.com/opensearch-project/opensearch-go) · [COMPATIBILITY.md](https://github.com/opensearch-project/opensearch-go/blob/main/COMPATIBILITY.md)
- Keeping clients compatible (AWS) — [blog](https://aws.amazon.com/blogs/opensource/keeping-clients-of-opensearch-and-elasticsearch-compatible-with-open-source/)
- REST API compatibility (`compatible-with`) — [Elastic docs](https://www.elastic.co/docs/reference/elasticsearch/rest-apis/compatibility)
- ILM vs ISM — [Opster guide](https://opster.com/guides/opensearch/opensearch-data-architecture/elasticsearch-ilm-vs-opensearch-ism-policy/)
- Prior-art patterns — Vector ES sink `api_version: auto` ([docs](https://vector.dev/docs/reference/configuration/sinks/elasticsearch/)); Grafana OpenSearch plugin ([docs](https://grafana.com/grafana/plugins/grafana-opensearch-datasource/))
