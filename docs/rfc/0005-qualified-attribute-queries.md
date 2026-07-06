# RFC 0005: Qualified Attribute Queries

- **Status:** Draft
- **Author:** Yuri Shkuro
- **Created:** 2026-06-19
- **Last Updated:** 2026-06-19

---

## Abstract

The Jaeger query API accepts attribute (tag) filters as unqualified key-value pairs. The backend must search for each attribute across every possible location in the span data model: span attributes, resource attributes, scope attributes, event attributes, and link attributes. This was acceptable when Jaeger used a flat inverted index (Cassandra), but becomes increasingly expensive with columnar stores (ClickHouse) and richer data models (OpenTelemetry). This RFC explores options for allowing users to optionally qualify attribute queries by scope, while preserving backwards compatibility.

---

## 1. Motivation

### 1.1 Historical Context

In the OpenTracing era, a span had three tag locations: `span.tags`, `span.process.tags`, and `span.logs[].fields`. Cassandra's storage schema maintained a single inverted index of all tags regardless of origin — querying was cheap because the index was pre-built.

### 1.2 The OpenTelemetry Data Model

OTLP spans have five distinct attribute scopes:

| Scope | OTLP Location | Semantic Meaning |
|-------|---------------|------------------|
| Resource | `ResourceSpans.resource.attributes` | Service/host-level metadata |
| Scope | `ScopeSpans.scope.attributes` | Instrumentation library metadata |
| Span | `Span.attributes` | Per-operation metadata |
| Event | `Span.events[].attributes` | Timestamped annotations |
| Link | `Span.links[].attributes` | Cross-trace references |

### 1.3 The Performance Problem

When a user queries `http.status_code=500`, the backend must search all five scopes using OR logic. In ClickHouse, this translates to five separate `arrayExists()` calls (three top-level, two nested within arrays), each scanning typed Map columns. The attribute metadata optimization (via `attribute_metadata` materialized view) narrows both the *types* and the *scopes* checked — if `http.status_code` appears only at the span level with integer type, only the integer span column is queried. However, this optimization requires an extra round-trip and a materialized view that may lag.

In Elasticsearch, each unqualified tag expands to a `bool.should` query across 5 field locations, increasing the number of sub-queries and reducing cache effectiveness.

### 1.4 The Semantic Problem

Users cannot express intent:
- "Find spans where the *span* has `deployment.environment=staging`" vs.
- "Find spans whose *resource* has `deployment.environment=staging`"

These are semantically different queries (the first finds spans explicitly tagged; the second finds spans from services in staging), but the current API conflates them.

### 1.5 Design Constraints

The solution must account for four layers of the system, each with different compatibility requirements:

| Layer | Compatibility Requirement |
|-------|--------------------------|
| **UI** | Existing search must keep working; new affordances are progressive enhancement |
| **API v3 (public gRPC/HTTP)** | Existing callers must not break; new fields are additive |
| **Internal Storage API** | Can change freely (internal, versioned with the binary) |
| **Remote Storage API (gRPC)** | Third-party plugins must degrade gracefully if they don't support scoped queries |

A key architectural choice is that **the old unqualified path can coexist with a new qualified path** at every layer — they are not mutually exclusive.

### 1.6 Per-Backend Tag Query Summary

Each backend has a different storage model for attributes, which affects how easily scoped queries can be implemented:

| Backend | Storage Model | Scope Differentiation | Scoped Query Feasibility |
|---------|--------------|----------------------|--------------------------|
| **ClickHouse** | Typed Map columns per scope (`str_attributes`, `resource_str_attributes`, etc.) + nested arrays for events/links | Full — each scope is a separate column family | Native support. Scoped query skips irrelevant columns. |
| **Elasticsearch/OpenSearch** | Denormalized object fields (`tag.*`, `process.tag.*`) + nested arrays (`tags`, `process.tags`, `logs.fields`) | Partial — span vs. resource vs. logs are separate fields, but no scope/event/link distinction in v1 schema | Good support for span/resource/log scoping. Event/link would need schema evolution. |
| **Cassandra** | Flat inverted index (`tag_index` table) keyed by `service_name + tag_key + tag_value` | None — all tags indexed identically regardless of origin | Cannot restrict scope at query time. Would return superset results (same as today). Acceptable: Cassandra is a legacy backend. |
| **Badger** | Flat KV index keyed by `service_name + key + value` | None — same as Cassandra | Same as Cassandra — returns superset results. Acceptable: Badger is intended for local/dev use. |

**Key insight:** Backends that lack scope differentiation (Cassandra, Badger) can simply ignore the scope qualifier and return results from all scopes — this is semantically a superset of the correct answer, which is safe (the user gets extra results, not missing results). The API contract can document this as "scope restriction is best-effort; backends that do not support it return unfiltered results."

---

## 2. Options

### Option A: Status Quo with Default Scope Restriction

Keep the current API shape but change the default search behavior from "all 5 scopes" to "span + resource only" (covers >95% of real queries). Provide an opt-in mechanism to expand to all scopes.

**Current REST API** (unchanged):
```
GET /api/v3/traces?query.attributes={"http.status_code":"200"}
```

**Proto change:**
```protobuf
message TraceQueryParameters {
  map<string, string> attributes = 3;
  // When true, attributes are also matched against scope, event, and link attributes.
  // Default (false) searches only span and resource attributes.
  bool search_all_attribute_scopes = 10;
}
```

**UI change:** Advanced toggle "Search all scopes" (default off).

**Rationale as baseline:** This represents the minimal delta — same API shape, behavior change only.

### Option B: Key Prefix Convention

Encode scope as a prefix in the attribute key string. Unqualified keys retain existing "search all" behavior.

**REST API:**
```
GET /api/v3/traces?query.attributes={"span.http.status_code":"200","resource.k8s.namespace.name":"prod"}
```

Unprefixed keys retain "search all" behavior:
```
GET /api/v3/traces?query.attributes={"http.method":"GET"}
```

**Proto change:** None — reuses existing `map<string, string> attributes`.

**Parsing rule:** If the key starts with a recognized prefix (`span.`, `resource.`, `scope.`, `event.`, `link.`) followed by at least one character, strip the prefix and restrict the search. Otherwise, search all scopes.

**UI change:** Documentation/tooltip explaining the prefix convention. No structural UI change required.

### Option C: Structured Attribute Filters

Introduce a new repeated message type that pairs key-value with an explicit scope enum. Designed from the start to accommodate future query operators.

**Proto change:**
```protobuf
enum AttributeScope {
  ATTRIBUTE_SCOPE_UNSPECIFIED = 0; // search all scopes
  ATTRIBUTE_SCOPE_SPAN = 1;
  ATTRIBUTE_SCOPE_RESOURCE = 2;
  ATTRIBUTE_SCOPE_SCOPE = 3;
  ATTRIBUTE_SCOPE_EVENT = 4;
  ATTRIBUTE_SCOPE_LINK = 5;
}

enum FilterOperator {
  FILTER_OPERATOR_EQUALS = 0;       // default: exact match
  FILTER_OPERATOR_NOT_EQUALS = 1;
  FILTER_OPERATOR_GREATER_THAN = 2;
  FILTER_OPERATOR_LESS_THAN = 3;
  FILTER_OPERATOR_REGEX = 4;
  FILTER_OPERATOR_EXISTS = 5;       // value field ignored
}

message AttributeFilter {
  string key = 1;
  string value = 2;
  AttributeScope scope = 3;         // default: search all scopes
  FilterOperator op = 4;            // default: equals
}

message TraceQueryParameters {
  // Legacy: unqualified equality search across all scopes. Retained for backwards compat.
  map<string, string> attributes = 3;

  // Structured filters with optional scope and operator qualification.
  repeated AttributeFilter attribute_filters = 10;
}
```

**REST API — current (unchanged for backwards compat):**
```
GET /api/v3/traces?query.attributes={"http.status_code":"200"}
```

**REST API — new structured parameter:**
```
GET /api/v3/traces?query.attributeFilters=key:http.status_code,value:200,scope:span,op:eq
```

Multiple filters as repeated params:
```
GET /api/v3/traces?query.attributeFilters=key:http.status_code,value:200,scope:span
                  &query.attributeFilters=key:k8s.namespace.name,value:prod,scope:resource
```

Or as a JSON array (matching protobuf JSON encoding):
```
GET /api/v3/traces?query.attributeFilters=[{"key":"http.status_code","value":"200","scope":"ATTRIBUTE_SCOPE_SPAN"}]
```

**Semantics:** All filters (from both `attributes` and `attribute_filters`) are ANDed. Within `attribute_filters`, `scope=UNSPECIFIED` means search all scopes (same as the legacy `attributes` field). Only `FILTER_OPERATOR_EQUALS` and `FILTER_OPERATOR_EXISTS` need to be implemented initially; others can return "unsupported" until backends add support.

**UI change:** Tag builder widget with optional scope dropdown per chip. Existing text box continues to work via the legacy `attributes` field.

### Option D: Backend Metadata Optimization (Transparent)

No API change. The ClickHouse backend already maintains an `attribute_metadata` materialized view that tracks which keys appear at which scopes. Extend this optimization to proactively skip scopes where a key has never been observed.

**Proto change:** None.

**UI change:** None.

**Mechanism:** Before executing the find-traces query, look up each queried attribute key in metadata. If metadata shows `http.status_code` only exists at the span level, generate conditions only for span attributes. If metadata is missing (cold key), fall back to searching all scopes.

---

## 3. Evaluation Criteria

| # | Criterion | Description |
|---|-----------|-------------|
| 1 | **Query Performance** | Reduces the number of storage scopes scanned per attribute |
| 2 | **Semantic Precision** | User can express exactly which scope(s) to search |
| 3 | **Additive Coexistence** | New qualified path can live alongside old unqualified path without conflict |
| 4 | **UI Compatibility** | Existing UI (single logfmt text box) keeps working unchanged |
| 5 | **API v3 Compatibility** | Public gRPC/HTTP contract does not break existing callers |
| 6 | **Remote Storage API Impact** | Minimal burden on third-party storage plugin authors |
| 7 | **Implementation Effort** | Total work across internal storage backends (ClickHouse, ES, Cassandra) |
| 8 | **API Complexity** | Surface area added to the public API |
| 9 | **Extensibility** | Paves the way for richer operators (`!=`, `>`, regex, `exists`) |
| 10 | **Cross-Backend Consistency** | All backends can implement the same semantics |
| 11 | **Discoverability** | Users can discover valid attribute keys and their scopes |
| 12 | **Migration Path** | Smooth transition without a flag day |

---

## 4. Comparison Matrix

Legend: 🟢 = strong, 🟡 = adequate, 🔴 = weak

| Criterion | A: Status Quo + Restrict | B: Key Prefix | C: Structured Filter | D: Metadata Opt. |
|-----------|:---:|:---:|:---:|:---:|
| 1. Query Performance | 🟡 | 🟢 | 🟢 | 🟡 |
| 2. Semantic Precision | 🔴 | 🟡 | 🟢 | 🔴 |
| 3. Additive Coexistence | 🟡 | 🟢 | 🟢 | 🟢 |
| 4. UI Compatibility | 🟢 | 🟢 | 🟡 | 🟢 |
| 5. API v3 Compatibility | 🟡 | 🟢 | 🟢 | 🟢 |
| 6. Remote Storage API Impact | 🟡 | 🟡 | 🟡 | 🟢 |
| 7. Implementation Effort | 🟢 | 🟢 | 🟡 | 🟢 |
| 8. API Complexity | 🟢 | 🟢 | 🟡 | 🟢 |
| 9. Extensibility | 🔴 | 🔴 | 🟢 | 🔴 |
| 10. Cross-Backend Consistency | 🟢 | 🟢 | 🟢 | 🟡 |
| 11. Discoverability | 🔴 | 🟡 | 🟡 | 🟡 |
| 12. Migration Path | 🟡 | 🟢 | 🟢 | 🟢 |

---

## 5. Detailed Analysis

### 5.1 Query Performance

| Option | Analysis |
|--------|----------|
| **A** | Removes 3 scopes (scope, event, link) from the default path. Helps the common case but is a blunt instrument — users searching resource-only attributes still scan span too. When opt-in flag is set, performance is same as today. |
| **B** | When a prefix is present, the backend skips 4 of 5 scopes. Unqualified keys still scan all. Net effect depends on user adoption of prefixes. |
| **C** | Scope enum tells the backend exactly where to look. Same as B when qualified; same as today when `scope=UNSPECIFIED`. The operator field enables future optimizations (e.g., `EXISTS` can skip value comparison). |
| **D** | Reduces scopes to only those observed in metadata. Effectiveness depends on metadata freshness and key cardinality. New/rare keys still scan all scopes. No user action required. |

### 5.2 Semantic Precision

| Option | Analysis |
|--------|----------|
| **A** | Binary: "span+resource" vs. "all". Cannot target a single scope or exclude one. |
| **B** | Supports all 5 scopes via prefix, but the convention is implicit — no schema validation. Users must know the prefix vocabulary. Ambiguity risk: an attribute key that naturally starts with `resource.` (unlikely but possible in user-defined attributes). |
| **C** | Fully explicit with per-filter granularity. Can mix scoped and unscoped filters in a single query. The scope enum is self-documenting via proto. Operator field adds predicate precision beyond just scope. |
| **D** | No precision at all — the system makes an opaque best-guess. Users cannot override. |

### 5.3 Additive Coexistence

| Option | Analysis |
|--------|----------|
| **A** | Changes the *default behavior* of the existing `attributes` field. This is technically a semantic breaking change: queries that previously found results in event/link attributes would stop finding them. Mitigated by the opt-in flag, but existing clients get different behavior without changing their code. |
| **B** | Unprefixed keys retain existing behavior. Prefixed keys are new. Both coexist in the same `attributes` map. No conflict, but the overloading of a single field introduces parsing ambiguity — a documented-but-implicit contract. |
| **C** | Old `attributes` field untouched with identical semantics. New `attribute_filters` is a separate additive field. Clear separation — no ambiguity about which code path is active. |
| **D** | No API change at all — purely additive at the backend. |

### 5.4 UI Compatibility

| Option | Analysis |
|--------|----------|
| **A** | Existing UI works but produces narrower results by default. A "search all scopes" toggle is trivial to add. |
| **B** | The existing logfmt text box works unchanged — users can optionally type `resource.service.name:foo`. No new UI components required; power users learn the syntax organically. |
| **C** | The existing text box continues to populate the legacy `attributes` field unchanged. To access structured filters, the UI needs a new tag builder widget (scope dropdown per chip). This is a progressive enhancement, not a requirement. |
| **D** | Completely transparent — no UI involvement. |

### 5.5 API v3 Compatibility

| Option | Analysis |
|--------|----------|
| **A** | Existing `attributes` field changes default behavior. A client that relied on finding event attributes will silently get fewer results after upgrade. This is a semantic break even though the wire format is unchanged. |
| **B** | Wire-compatible: same field, same type. Old clients send unprefixed keys and get existing behavior. Fully backwards compatible. |
| **C** | New repeated field defaults to empty (no-op). Old clients don't set it and get identical behavior. Fully backwards compatible. |
| **D** | No API change. |

### 5.6 Remote Storage API Impact

| Option | Analysis |
|--------|----------|
| **A** | Remote plugins must implement the `search_all_attribute_scopes` flag. If they ignore it, they may return too many or too few results depending on their existing implementation. |
| **B** | Remote plugins receive the same `map<string, string>`. They must understand the prefix convention to benefit. Plugins that ignore prefixes still work (they'll search all scopes for the prefixed key — likely returning no results for the prefixed form). Documentation + a utility function can ease adoption. |
| **C** | Remote plugins receive a new `attribute_filters` field. Plugins that ignore it degrade gracefully — only the legacy `attributes` field is used. The Jaeger query service could optionally fall back to populating `attributes` from `attribute_filters` (stripping scope info) when talking to plugins that don't advertise support. |
| **D** | No impact — optimization is internal to specific backends. Remote plugins are unaffected. |

### 5.7 Implementation Effort

| Option | Analysis |
|--------|----------|
| **A** | Low: flip default, add one boolean parameter. But testing the behavior change and documenting the migration is non-trivial. |
| **B** | Minimal: add prefix parsing in the query parser layer (one function), pass a `scope` field alongside each attribute into the storage interface. Each backend adds a branch to skip scopes. Estimate: small PR per backend. |
| **C** | Moderate: new proto message types (in jaeger-idl), regenerate, update query parsers (HTTP + gRPC), update internal storage interface to carry the filter struct, each backend adds handling. The proto and interface changes are one-time; per-backend work is comparable to B. Operator support beyond `EQUALS` can be deferred. |
| **D** | Low: ClickHouse already has this partially. Extend the metadata cache to also skip scopes (not just types). ES and Cassandra don't benefit (no metadata). |

### 5.8 API Complexity

| Option | Analysis |
|--------|----------|
| **A** | Adds 1 boolean field. Minimal surface, but the behavioral implication of the boolean is non-obvious and the semantics feel accidental rather than designed. |
| **B** | No new fields, no new types. Complexity lives in documentation of the prefix convention. Simple to explain, but "magic prefixes in a string" is an implicit schema that cannot be validated by proto tooling. |
| **C** | Adds 1 repeated message field + 2 enums. More concepts to learn upfront, but the schema is self-documenting and tooling-friendly. The complexity is *explicit* rather than hidden in parsing conventions. |
| **D** | Zero API complexity increase. |

### 5.9 Extensibility

| Option | Analysis |
|--------|----------|
| **A** | Dead end — only controls scope breadth. Adding operators later requires a separate mechanism. |
| **B** | Dead end for operators. Cannot encode `http.status_code > 400` or `regex(url, ".*foo.*")` in a string key-value pair. A future RFC would need to introduce a different mechanism anyway. |
| **C** | Designed for extension. The `FilterOperator` enum accommodates `!=`, `>`, `<`, `regex`, `exists` from day one. Adding new operators is a proto enum value — no structural API change. This is the only option that avoids a second API redesign when operators are needed. |
| **D** | No extensibility — pure optimization of existing behavior. |

### 5.10 Cross-Backend Consistency

| Option | Analysis |
|--------|----------|
| **A** | All backends can implement the boolean. But backends with flat indexes (Cassandra) can't actually restrict scope — they'd need to post-filter, which is expensive. |
| **B** | All backends can implement prefix parsing identically. Cassandra (flat index) ignores the scope restriction and returns superset results — acceptable for a backend that doesn't distinguish scopes. Documented as "best effort." |
| **C** | Same as B — scope restriction is advisory for flat-index backends. Operators can similarly be "best effort" (return error for unsupported operators rather than wrong results). The proto enum makes the capability contract explicit. |
| **D** | Only benefits backends with metadata infrastructure. Creates invisible performance divergence — same query is fast on ClickHouse, slow on ES. Not a correctness issue, but surprising. |

### 5.11 Discoverability

| Option | Analysis |
|--------|----------|
| **A** | The boolean flag is documented but doesn't help users understand the scope model. Users have no way to know which attributes live at which scopes. |
| **B** | Users must know the prefix vocabulary. Could be aided by autocomplete in the UI text box (e.g., typing `resource.` triggers suggestions). No intrinsic schema-level discoverability. |
| **C** | The scope enum is self-documenting in proto and OpenAPI specs. Can be combined with a `GetAttributeKeys(scope?)` API in the future. Tooling (CLI, UI) can enumerate valid scopes from the enum. |
| **D** | No discoverability improvement — users don't even know the optimization exists. But the underlying metadata table could power a future discovery API. |

### 5.12 Migration Path

| Option | Analysis |
|--------|----------|
| **A** | **Requires migration.** Existing queries may return fewer results. Options: (1) default `search_all_attribute_scopes=true` initially, flip later; (2) announce in release notes as a behavior change. Either way, a flag day exists. |
| **B** | No migration needed — old queries work identically. Users adopt prefixes when they learn about them. Gradual, organic adoption. |
| **C** | No migration needed — old `attributes` field works identically. New filter mechanism is opt-in. The query parser can internally support prefix syntax (Option B) as sugar that normalizes into `AttributeFilter` messages, providing two on-ramps to the same backend path. |
| **D** | No migration at all — transparent backend change. |

---

## 6. Discussion

### 6.1 Options Are Not Mutually Exclusive

Option D (metadata optimization) is purely internal and can be implemented regardless of which API option is chosen. It provides an immediate performance improvement with zero coordination cost.

Options A, B, and C are API-level changes that differ in their approach. B and C can coexist — the query parser can accept prefix syntax in the `attributes` field and normalize it into `AttributeFilter` structs internally, making B a convenient text syntax for C's structured semantics.

Option A is the weakest standalone choice: it has a backwards-compatibility hazard, provides no semantic precision, and doesn't extend.

### 6.2 The Extensibility Argument

Queries like `http.status_code >= 400` or `url matches ".*checkout.*"` are frequently requested. Today they are impossible — the API only supports string equality. Option C is the only shape that accommodates operators without a second API redesign. The `FilterOperator` enum is included in the proto from day one, but backends can return "unsupported" for operators they haven't implemented yet. This avoids both premature implementation *and* premature API ossification.

### 6.3 The Prefix Ambiguity in Option B

A risk in Option B is collision with real attribute keys. OTel semantic conventions don't use `resource.`, `span.`, etc. as prefixes — but user-defined attributes could. Escape hatches (e.g., `span.span.foo` to mean "key `span.foo` at span scope") add documentation burden. Option C avoids this entirely because scope is a separate field.

However, if B is treated as *syntax sugar for C* (i.e., the query parser recognizes prefixes and translates them to `AttributeFilter{scope: ..., key: ...}`), then B's ambiguity risk is contained to a convenience layer rather than baked into the API contract.

### 6.4 REST API Design for Structured Filters

The current v3 REST API for attributes:
```
GET /api/v3/traces?query.attributes={"http.status_code":"200","error":"true"}
```

For the structured `attribute_filters` field, the natural proto3 JSON-over-HTTP encoding would be:
```
GET /api/v3/traces?query.attributeFilters=[{"key":"http.status_code","value":"200","scope":"ATTRIBUTE_SCOPE_SPAN"}]
```

This is verbose but unambiguous, matches the gRPC-Gateway convention for repeated message fields, and is the format that generated OpenAPI clients will use. A shorter logfmt-style encoding could be supported as a convenience alias parsed by the HTTP gateway:
```
GET /api/v3/traces?query.filter=span.http.status_code=200&query.filter=resource.k8s.namespace.name=prod
```

This gives us the best of both worlds: a machine-friendly canonical form and a human-friendly shorthand.

### 6.5 UI Considerations

The Jaeger UI currently has a single "Tags" text input in logfmt format. Option B is unique in that it can be adopted *inside the existing text box* without any UI change — users simply learn to type `resource.service.name:foo`.

For Option C, the UI can:
1. Keep the existing text box populating the legacy `attributes` field (zero change).
2. Add a new "attribute filter" builder with per-chip scope dropdown as progressive enhancement.
3. Support prefix syntax in the text box as convenience (parser normalizes to C's format).

These are not mutually exclusive — (1) ships immediately, (2) and (3) follow.

---

## 7. Recommendation

**Phased approach combining D + C (with B as text syntax):**

### Phase 1: Backend Metadata Optimization (immediate, no coordination)

Implement Option D — extend the ClickHouse attribute metadata cache to also filter by scope, not just type. Skip `arrayExists()` calls for scopes where a key has never been observed. This provides an immediate performance win with zero API/UI/plugin impact.

### Phase 2: Structured Filters in API (the long-term solution)

Implement Option C as the canonical API shape:
- Add `AttributeFilter`, `AttributeScope`, and `FilterOperator` to the proto (jaeger-idl).
- Add `attribute_filters` repeated field to `TraceQueryParameters`.
- Retain `attributes` map field with identical semantics (search all scopes, equality only).
- Update internal storage interface to carry `[]AttributeFilter` alongside the legacy map.
- Each backend routes scoped filters to the appropriate storage location.
- Initially only `FILTER_OPERATOR_EQUALS` and `FILTER_OPERATOR_EXISTS` are implemented; others return an error.

### Phase 3: Prefix Syntax as Convenience (minimal effort, high usability)

Support Option B's prefix syntax in both the HTTP query parser and the UI text box as *sugar* that normalizes into `AttributeFilter` messages:
- `resource.service.name:foo` → `AttributeFilter{key: "service.name", scope: RESOURCE, op: EQUALS, value: "foo"}`
- `http.method:GET` (no prefix) → `AttributeFilter{key: "http.method", scope: UNSPECIFIED, op: EQUALS, value: "GET"}`

This gives users immediate access to scope qualification via the existing text box, without requiring UI redesign.

### Phase 4: UI Enhancement (progressive)

Add an optional scope dropdown to each tag chip in the search form. Default is "Any" (maps to `UNSPECIFIED`). Power users get precision; casual users aren't disrupted.

---

## 8. Open Questions

1. **Conjunction semantics across scopes:** If a user specifies `resource.service.name:foo` AND `span.http.status_code:500`, should both conditions match the *same* span, or can `service.name` match at the resource level while `http.status_code` matches at the span level of a different span in the trace? (The internal storage API explicitly leaves this implementation-dependent — see `TraceReader.FindTraces` contract.)

2. **Attribute key listing API:** Should we add a `GetAttributeKeys(scope?)` API to enable discoverability? This would complement the structured filters and help UI autocomplete. ClickHouse's metadata table already supports this.

3. **Negation and absence:** The `FilterOperator` enum includes `NOT_EQUALS` and `EXISTS` — should these be required in the initial implementation, or can they be deferred?

4. **Remote Storage v2 timeline:** Is a new major version of the remote storage protocol planned? If so, `AttributeFilter` could be included in that version, simplifying the migration for plugin authors.

5. **Prefix collision escape hatch:** If we support prefix syntax (Phase 3), do we need an explicit escape for attribute keys that start with `resource.`, `span.`, etc.? Or is the structured JSON form (`query.attributeFilters=[...]`) sufficient as the unambiguous alternative?

---

## 9. References

- [Internal Storage API: `TraceQueryParams`](../../internal/storage/v2/api/tracestore/reader.go) — current unqualified `Attributes pcommon.Map` field
- [ClickHouse query builder](../../internal/storage/v2/clickhouse/tracestore/query_builder.go) — `buildStringAttributeCondition` showing 5-scope OR expansion
- [ClickHouse attribute metadata](../../internal/storage/v2/clickhouse/tracestore/attribute_metadata.go) — existing type-level metadata cache
- [Elasticsearch tag query](../../internal/storage/v2/elasticsearch/tracestore/core/reader.go) — `buildTagQuery` showing multi-field OR expansion
- [API v3 HTTP query parser](../../cmd/jaeger/internal/extension/jaegerquery/internal/apiv3/query_parser.go) — current `query.attributes` JSON parameter parsing
- [OpenTelemetry Trace Data Model](https://opentelemetry.io/docs/specs/otel/trace/api/) — defines the 5 attribute scopes
