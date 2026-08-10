# RFC 0005: Structured Query Filters for Trace Search

- **Status:** Draft
- **Author:** Yuri Shkuro
- **Created:** 2026-06-19
- **Last Updated:** 2026-07-09

---

## Abstract

Jaeger's trace-search API filters spans by unqualified key-value tag pairs, implicitly ANDed, matched against the span's attribute locations without distinguishing among them (how many locations, and which, is backend-dependent). This RFC defines a **structured query-filter model** for trace search that (1) lets a predicate reference a specific attribute *level* (span / resource / instrumentation / event / link) or a built-in *property* (duration, name, status, …), (2) composes predicates with **boolean operators** (`AND`/`OR`/`NOT`), and (3) keeps the existing unqualified tag filter working unchanged.

The model is a **fully structured AST** (proto/JSON), *not* a free-text query language, and its reach is deliberately bounded by what Jaeger's storage backends (Elasticsearch/OpenSearch, ClickHouse, Cassandra, Badger) can implement — it covers filtering and stops short of result shaping, aggregation, and trace-tree/structural queries.

---

## 1. Motivation

### 1.1 Historical context

In the OpenTracing era a span had three tag locations — `span.tags`, `span.process.tags`, and `span.logs[].fields` — and Cassandra maintained a single inverted index over all of them. Querying was cheap because the index was pre-built and undifferentiated: a tag match was a tag match, regardless of where the tag came from.

### 1.2 The OpenTelemetry data model

OTLP spans carry attributes at five distinct levels:

| Level | OTLP location | Semantic meaning |
|-------|---------------|------------------|
| Resource | `ResourceSpans.resource.attributes` | Service / host-level metadata |
| Instrumentation | `ScopeSpans.scope.attributes` | Instrumentation library (`InstrumentationScope`) metadata |
| Span | `Span.attributes` | Per-operation metadata |
| Event | `Span.events[].attributes` | Timestamped annotations |
| Link | `Span.links[].attributes` | Cross-trace references |

(OTLP's own name for the instrumentation level is the `InstrumentationScope`, carried inside `ScopeSpans`. This RFC uses **level** for the qualifier and **instrumentation** for that value, to avoid overloading the word "scope" — see §5.1.)

### 1.3 The performance problem

When a user queries `http.status_code=500`, an unqualified backend must fan the query out across every attribute level it indexes with OR logic. In ClickHouse this expands to five separate `arrayExists()` calls (three top-level columns, two nested within event/link arrays), each scanning a typed Map column. In Elasticsearch each unqualified tag expands to a `bool.should` across the field locations, increasing sub-query count and reducing cache effectiveness. The cost is paid on every attribute of every search, even though the user almost always knows which level they mean.

### 1.4 The semantic problem

The unqualified API cannot express intent. "Find spans where the *span* has `deployment.environment=staging`" and "find spans whose *resource* has `deployment.environment=staging`" are different questions — the first finds spans explicitly tagged, the second finds spans emitted by services in staging — but today they are the same query. Nor can the API express `duration > 2s`, `http.status_code >= 400`, or `A OR B`: it supports only string equality, ANDed.

### 1.5 Two axes, not one

Level qualification alone is too narrow: attaching a level to each attribute leaves an API that still cannot express `OR` or `duration > 2s`. A complete answer must settle two independent axes:

- **What a predicate can reference** (the *leaf*): a level-qualified attribute, but also built-in span/trace *properties* (`duration`, `name`, `status`, …) that are not attributes at all, and an *operator* richer than equality.
- **How predicates combine** (the *composition*): equality-only conjunction is the floor; a boolean expression is the natural ceiling; aggregation and trace-tree navigation lie beyond.

This RFC designs both axes together (§3–§5) rather than adding the level qualifier alone.

### 1.6 The storage-backend landscape

Feasibility is dominated by how each backend physically stores and indexes attributes. This table is load-bearing for every decision below.

| Backend | Attribute storage | Level differentiation | Consequence |
|---------|-------------------|-----------------------|-------------|
| **ClickHouse** | Typed Map columns per level (`str_attributes`, `resource_str_attributes`, …) + nested arrays for events/links | Full — each level is a distinct column family | Native level filtering; a level-qualified query skips irrelevant columns |
| **Elasticsearch / OpenSearch** | Denormalized object fields (`tag.*`, `process.tag.*`) + nested arrays (`tags`, `process.tags`, `logs.fields`) | Partial — span / resource / log are distinct; no instrumentation/event/link distinction in the v1 schema | Span/resource/event levels work; instrumentation and link need schema evolution |
| **Cassandra** | One flat inverted index (`tag_index`) keyed by `service + key + value` | None | Cannot restrict level at query time; only the indexed levels exist at all |
| **Badger** | Flat KV tag index (span tags + process tags + log fields) | None | Same as Cassandra |

**The flat backends flatten on write, and that constrains what any query can honor.** Cassandra and Badger both index exactly three of the five levels — span attributes, resource (process) attributes, and event (log-field) attributes — merged into one undifferentiated index. Instrumentation-scope attributes are collapsed into span tags (indistinguishable), and **link attributes are dropped entirely** (the v1 model has no field for them). So a "just ignore the level and return everything" fallback is a genuine superset *only for the levels that were actually indexed* (span/resource/event). For a level the backend never indexed (link, and arguably instrumentation), widening does not return a superset — it returns the wrong set. The best-effort contract in §7 is written to this reality: honor levels that are indexed, reject (not silently widen) levels that are not.

---

## 2. Goals and non-goals

### Goals

- **G1 — Level-qualified attributes.** A predicate may target a single OTLP attribute level (span/resource/instrumentation/event/link) or leave it unqualified (search the default level set — span-or-resource; §5.1).
- **G2 — Properties.** A predicate may target a built-in span/trace property (`duration`, `name`, `status`, `kind`, service, trace-level values) uniformly with attributes (§5).
- **G3 — Richer operators.** Beyond equality: `ne`, `gt`, `lt`, `regex`, `exists` — extensible without a second API redesign.
- **G4 — Boolean composition.** Predicates combine with `AND`/`OR`/`NOT` and nesting (§4 tier L2).
- **G5 — Backward compatibility.** The existing unqualified `attributes` filter keeps working byte-for-byte; the new model is additive at every layer (public API, internal storage API, remote-storage gRPC).
- **G6 — Structured AST.** The query is a typed proto/JSON structure, machine-first, self-documenting via schema.
- **G7 — Cross-backend implementability with graceful degradation.** Fully supported on ClickHouse and Elasticsearch/OpenSearch; backends that cannot honor a level or operator reject that predicate rather than returning wrong results.

### Non-goals

- **A free-text query language.** No lexer/grammar for a TraceQL/SQL-like string surface. If such a surface is ever wanted it can compile *to* this AST; the AST is the contract.
- **Result shaping** — projection / `SELECT` / column selection, ordering, paging (§4 tier L3).
- **Aggregation and metrics** — `count`/`GROUP BY`/`rate()` over spans (§4 tier L4). This overlaps Jaeger's existing metrics/SPM query service and belongs there.
- **Structural / trace-tree queries** — ancestor/descendant/sibling navigation (§4 tier L5). Implementable only post-fetch (assemble each candidate trace, evaluate relationships in memory) since no backend can prune them in storage; a distinct, larger execution model deferred to a future effort.
- **Storage-schema changes.** The model is designed to fit existing schemas; where a backend's schema cannot express a level (ES event/link, flat-index link), that is a documented limitation, not a schema migration mandated by this RFC.

---

## 3. The two design axes

The model factors cleanly into two orthogonal axes, addressed in the next two sections:

- **Composition (§4)** — *how predicates combine.* This is the "how expressive?" question, mapped as a continuum from today's flat conjunction up to a full trace query language, with an explicit decision on where Jaeger stops.
- **Predicate anatomy (§5)** — *a single predicate's operands (level-qualified attributes, properties, or constants), operator, and value typing.*

They are independent: the composition tier could be chosen with or without properties, and vice versa. §6 combines the two into one proto/AST.

---

## 4. Composition — the query-complexity continuum

The central design question is *how expressive should the structured filter be?* Below is the continuum from today's API to a full trace query language, calibrated against three well-known structured query systems as prior art. Jaeger targets a structured AST, so these matter only for the *expressiveness tier* each represents — not their surface syntax.

**Prior art:**

- **SQL over a flat span table** — boolean `WHERE`, projection, `ORDER BY`/`LIMIT`, `GROUP BY` aggregation. No notion of the trace tree.
- **Braintrust BTQL** — a structured, SQL-like query language (also expressible as a JSON AST): boolean filters over dotted field paths, `IN`/`LIKE`/`MATCH`, functions, `sort`/`limit`, and `dimensions`/`measures` aggregation. Document/row-oriented; no trace-tree operators.
- **Grafana TraceQL** — trace-native: scope-qualified attributes (`span.`, `resource.`, `event.`, `link.`, `parent.`, unscoped `.`), built-in span/trace fields (`duration`, `name`, `status`, `kind`, `rootName`, `traceDuration`, …), boolean field expressions, **structural operators** over the trace tree (`>>` descendant, `<<` ancestor, `~` sibling), spanset aggregation/grouping, and a metrics extension (`rate()`, `quantile_over_time()`). It occupies the top of the continuum; its structural and metrics tiers are the frontier this RFC declines.

**The expressiveness ladder** (each tier cumulative):

| Tier | Capability | Prior art |
|------|-----------|----------|
| **L0** | Unqualified conjunction of `key=value` equalities, search-all-levels — **today** | — |
| **L1** | Level/property predicates `{level\|property, key, op, value}`, still all-ANDed | — |
| **L2** | Boolean expression tree: `AND`/`OR`/`NOT` + nesting over L1 leaves | SQL `WHERE`, BTQL filter |
| **L3** | Result shaping: projection, ordering, limit/paging | SQL `SELECT/ORDER BY/LIMIT`, TraceQL `select()` |
| **L4** | Aggregation & grouping: `count/sum/avg/quantile` by field, optionally over-time | SQL `GROUP BY`, BTQL measures, TraceQL `by()`+`rate()` |
| **L5** | Structural / trace-tree operators: ancestor/descendant/sibling/child, `parent.` | TraceQL `>>`/`<<`/`~` |

**Feasibility across backends** (🟢 good · 🟡 partial or costly · 🔴 poor or infeasible):

| Criterion | L0 | L1 | L2 | L3 | L4 | L5 |
|-----------|:-:|:-:|:-:|:-:|:-:|:-:|
| User expressiveness | 🔴 | 🟡 | 🟢 | 🟢 | 🟢 | 🟢 |
| Elasticsearch/OpenSearch | 🟢 | 🟢 | 🟢¹ | 🟢 | 🟢² | 🟡³ |
| ClickHouse | 🟢 | 🟢 | 🟢 | 🟢 | 🟢 | 🟡⁴ |
| Cassandra / Badger | 🟢 | 🟡⁵ | 🔴⁶ | 🟡 | 🔴 | 🟡³ |
| AST / API surface (🟢 = small) | 🟢 | 🟢 | 🟢⁷ | 🟡 | 🔴 | 🔴 |
| UI query builder (🟢 = simple) | 🟢 | 🟢 | 🟡⁸ | 🟡 | 🔴 | 🔴 |
| Cross-backend uniformity | 🟢 | 🟡 | 🟡⁹ | 🟡 | 🔴 | 🟡 |

¹ ES `bool` query (`must`/`should`/`must_not`) nests arbitrarily. ² ES aggregations exist but overlap Jaeger's metrics/SPM path. ³ structural operators are evaluated *post-fetch* — the query service assembles each candidate trace and checks ancestor/descendant/sibling in memory — so they work on any backend; but no Jaeger schema encodes the trace tree, so they cannot be pushed into storage to prune candidates, which makes them **inefficient at scale, not infeasible**. ⁴ ClickHouse could additionally push some structural checks into a self-join within a trace partition, though not with today's schema/query builder; otherwise it is post-fetch as in ³. ⁵ superset-safe only for the levels the flat index actually contains — span/resource/event; link is unrepresentable and instrumentation indistinguishable (§1.6). ⁶ a flat inverted index has no `OR`/`NOT`. ⁷ L2 is not a delta in message types at all — boolean `and`/`or`/`not` are just more `op` values on the same `Call` node the conjunctive subset already uses; see §6. ⁸ the API need not wait for the UI: a builder can render the conjunctive subset first and add nesting later. ⁹ capable backends evaluate the full tree; flat backends evaluate the conjunctive subset and reject `OR`/`NOT` — the same posture they already take for unsupported operators.

**Decision — target L2 (the full boolean tree); conjunction is the subset every backend supports.**

- **L1 is not a coherent stopping point.** In SELECT/FILTER/GROUP-BY terms, L3 adds SELECT and L4 adds GROUP BY — separate clauses, principled to defer. But L1 stops *inside* the FILTER clause: it has conjunction and lacks disjunction/negation, which is no natural boundary. The complete FILTER is the boolean expression — L2.
- **The backend-uniformity concern does not favor L1.** A flat-index backend handles the conjunctive *subset* of an L2 tree exactly as it would handle L1 — walk the ANDs, reject anything containing `OR`/`NOT`. So L1 buys the weak backends no simplicity; it only removes power from ClickHouse and ES/OS, the backends that motivate this RFC. L1 is L2 with the other node types deleted from the schema.
- **API expressiveness is decoupled from UI expressiveness.** The API can be L2 while the UI ships only a conjunctive-subset builder and adds nested groups later.
- **Stopping at L1 would cost two API changes** to the same surface and leave a flat predicate-list field as legacy baggage beside the legacy `attributes` map.

So the committed filter API is the **L2 boolean expression tree** (§6). "L1" is retained only as a *capability tier* — the conjunctive subset that every backend, including the flat ones, supports. **L3 is deferred** (awkward against Jaeger's whole-trace result model, and inert until L4 exists). **L4 is excluded** (belongs to the metrics/SPM subsystem; a separate RFC). **L5 is excluded** — not for infeasibility (structural predicates can be evaluated post-fetch on any backend, assembling each candidate trace) but because it is a distinct fetch-then-filter execution model that cannot prune in storage, is inefficient at scale, and is a large surface; deferred as a separate effort. The one honest internal nuance is that a pure conjunction admits a fast all-predicates-pushdown path while a tree with `OR` needs fuller evaluation — an optimization inside the capable backends, not an API concern.

**Why the excluded tiers are bounded, not dead ends.** This RFC's `Expression` is the *filter* layer — the per-span predicate. The deferred tiers (§4) extend it rather than replace it, which is why declining them now does not paint the design into a corner. L3 (projection) and L4 (aggregation/grouping) add sibling clauses over the same `Expression`: a projection is a list of expressions, a group key is an expression, an aggregate is a `Call`. L5 (structural / trace-tree queries) adds an *outer* layer — a query over relationships between sets of spans, whose per-set filter is an `Expression` — so structural queries would wrap this AST, not force a redesign of it. The only capabilities the current shape genuinely could not grow into are narrow: **set membership over a list** (already addressed via `in`/`not_in` + `Array`, §6.1) and a parent-scope modifier (a flag over a level, orthogonal to `level` itself; it belongs with the deferred structural tier). Everything else — richer operators (`>=`/`<=`, `!~`), arithmetic, aggregates, semantic literal types (duration/status/kind) — is a pure addition to the open `op`/`type` vocabularies or the `Call` node, with no new message types. (The fuller trace query languages surveyed in §4 occupy exactly these upper tiers.)

---

## 5. Predicate anatomy — operands, operators, and value types

A predicate is a `Call` (§6.1): an **operator** (§5.3) applied to **operand** expressions. Each operand is either a *reference* — a level-qualified attribute (§5.1) or a built-in property (§5.2) — or a *constant* (a scalar, or an array for `in`/`not_in`). The operands are the same kind of thing, so neither side is privileged: the everyday `reference op constant` shape (`span.http.status_code = 500`) and a `reference op reference` shape (`span.a > span.b`) are equally expressible. A constant carries an optional **type** (§5.3–§5.4) telling the backend how to interpret it.

### 5.1 Attribute levels

An attribute reference is a `(level, key)` pair. The level vocabulary follows the OTLP model (§1.2). We call the qualifier **level** and the instrumentation-scope value **instrumentation**, so that the field name never collides with one of its own values and never overloads OTLP's `InstrumentationScope`:

| `level` | Targets | Notes |
|---------|---------|-------|
| *(empty)* | span **or** resource attributes | default; the levels Jaeger already indexes and searches by default (span + resource/process tags) |
| `span` | `Span.attributes` | |
| `resource` | `resource.attributes` | |
| `instrumentation` | `ScopeSpans.scope.attributes` | `InstrumentationScope` attributes |
| `event` | `Span.events[].attributes` | |
| `link` | `Span.links[].attributes` | |

The empty default means span-or-resource rather than "all five" as a deliberate choice for the new `filter` model: span and resource (process) attributes are the tags reliably indexed across every backend, so this default covers the high-value common case without paying to scan levels that are unindexed or costly. It is *not* a claim that span-or-resource matches today's unqualified behavior — that behavior is backend-dependent and generally searches *more*: ClickHouse ORs across all five levels (§1.3), and Elasticsearch across its indexed span/resource/event locations. The legacy `attributes` map keeps that existing behavior unchanged; the span-or-resource default applies only to an empty `level` in the new `filter` field. A backend that indexes or scans more simply returns a superset (§1.6). A further level such as `parent.` (the parent span's attributes) could be added later — the vocabulary is an open string set (§6).

### 5.2 Properties

Much of what users filter on is not an attribute at all but a built-in value of the span or trace — its duration, name, status, and so on. This RFC calls these **properties**. Modeling them as predicate targets unifies several of Jaeger's ad-hoc top-level query parameters into one vocabulary:

| `property` | Meaning | Today in Jaeger's API |
|-------------|---------|-----------------------|
| `duration` | span duration | dedicated `duration_min`/`duration_max` fields |
| `name` | span (operation) name | dedicated `operation_name` field |
| `service` | service name | dedicated `service_name` field |
| `status` | OTel status (`ok`/`error`/`unset`) | ad-hoc `error=true` tag |
| `kind` | span kind (`server`/`client`/…) | ad-hoc `span.kind` tag |
| `traceDuration` | whole-trace duration | not expressible |
| `rootName` / `rootService` | root span's name / service | not expressible |
| `spanID` / `traceID` | identifiers | ID lookup only |

The value of the property model is that it is *uniform*: `duration > 2s`, `status = error`, and `span.http.method = GET` are all the same shape (a predicate with an operator), instead of three unrelated mechanisms (a dedicated duration field, a magic `error` tag, and a tag map). It also makes queries expressible that are impossible today (`traceDuration`, `rootName`). The dedicated top-level query fields (`service_name`, `operation_name`, the paired `duration_min`/`duration_max`) remain supported for backward compatibility, but the query service **normalizes them into property predicates in `filter`** (`duration_min`/`duration_max` become a pair of `duration` range predicates) so that storage backends implement one filtering model rather than a growing mix of scalar fields *plus* `attributes` *plus* `filter`. That normalization is an architectural choice with a compatibility wrinkle at the remote-storage boundary — see §7.

Properties are a natural extension of the same leaf, but they are not required on day one: the initial implementation can support level-qualified attributes plus a small property set (`duration`, `name`, `service`, `status`, `kind`) and phase in the trace-level ones (§9). Like levels and operators, the property set is an open, documented string vocabulary.

### 5.3 Operators and value typing

The operator set is `eq` (default), `ne`, `gt`, `lt`, `regex`, `exists`, and set membership `in`/`not_in` (whose right operand is an `Array`, §6.1). A constant `value` is a string on the wire and carries an **optional `type`** (`string` — the default — `int`, `double`, or `bool`) telling the backend how to interpret it (on the `Scalar`/`Array` term, §6.1). Omit `type` and the backend resolves it as it does today, matching wherever the key actually lives; supply it and the backend can route straight to the correctly-typed storage with no metadata lookup. `type` is an *optimization hint, not an authority* — §5.4 works through why it must stay optional (multi-type keys, backends with no metadata, and the silent-mismatch hazard). Numeric operators (`gt`/`lt`) imply a numeric interpretation regardless. A backend that does not implement an operator rejects the predicate (§7) rather than guessing.

**Units of numeric values (decision point).** For a value with an implied unit — chiefly `duration` — the wire value should carry the unit *explicitly*, in Go duration syntax (`2s`, `1h30m`), matching today's `duration_min`/`duration_max` fields, rather than a bare number in an assumed unit (which is ambiguous — nanoseconds? milliseconds?). A bare-number value (e.g. a numeric attribute like `http.response.size`) is compared numerically and carries no RFC-defined unit: the caller and the stored data share whatever unit the attribute was recorded in, exactly as today. See §10 Q7.

### 5.4 Typed values — an exploration

Carrying the value's type in the query (§5.3) targets the *other half* of what ClickHouse's `attribute_metadata` view resolves per query today — not just the level (§1.6) but the **type** — so a backend could skip that lookup. Attractive, but relocating the type decision to the query has consequences that decide whether it can be *mandatory*, *optional*, or is not worth it at all.

**(1) A wrong type silently under-matches.** A hand-composed query (a script, a `curl`) that declares `type=int` for a value stored as a string routes to the wrong typed storage and returns *nothing* — a silent false negative, not an error. Today's metadata-driven resolution cannot be wrong this way: it queries wherever the key actually lives. So a caller-supplied type must be a hint the backend may fall back from, never an authority — and `eq` in particular can compare the string form on most backends regardless of the declared type.

**(2) Most Jaeger backends cannot expose type metadata.** The autocomplete that makes a typed query pleasant is fed by a tag-values API that returns each value *with its type*. Only ClickHouse has the equivalent (`attribute_metadata`). ES/OS have no cheap keys/values/types enumeration (it is an expensive aggregation, and tag types are not readily available); Cassandra and Badger have none at all (a flat string index with no enumeration API). So typed authoring assistance is a ClickHouse-mostly luxury; elsewhere the caller falls back to untyped/string.

**(3) A key legitimately has more than one type.** ClickHouse's metadata deliberately records that the same key can appear with *different* types across services — `http.status_code` as an int from one service, a string from another. Today's storage-side resolution searches *all* observed types and matches both. A single `type` in the query cannot express "any type" — declaring one silently drops the others. This is decisive: **an unspecified type must mean "match any type" (today's behavior); a specified type is a narrowing the caller opts into.**

**(4) What each backend would need, and whether it can.** For type-in-query to pay off, a backend needs (i) typed predicate evaluation and (ii), for authoring help, a typed discovery API. (🟢 native · 🟡 partial / costly · 🔴 not feasible)

| Capability | ClickHouse | Elasticsearch/OpenSearch | Cassandra / Badger |
|------------|:---:|:---:|:---:|
| (i) typed predicate evaluation | 🟢 typed columns | 🟡 `eq` is a string term; numeric `gt`/`lt` needs the tag value indexed numerically (a schema question) | 🔴 string `eq` only; no numeric range |
| (ii) typed discovery API | 🟢 `attribute_metadata` | 🟡 expensive aggregation; type not exposed | 🔴 no enumeration at all |

The relocation is fully realizable only on ClickHouse; ES/OS partially (and only after a schema decision for numeric tags); the flat backends not at all — but they never supported numeric range anyway and store everything as strings, so `type` is simply moot for them.

**(5) Rollout before autocomplete exists.** The high-value typed cases need no discovery: **properties** carry an intrinsic type (`duration`, `status`, `kind`), so `duration > 2s` works from day one; and scoped **string-`eq`** attributes are the default (today's behavior). Only typed predicates over *arbitrary user attributes* (numeric range on `http.request.size`) need the caller to know the type or a discovery API — those light up later, ClickHouse first. Structured queries therefore roll out immediately for properties + string attributes, with typed attribute predicates and the discovery API following.

**(6) Verdict — worth it, but only as an optional hint.** Mandating typed values would break multi-type correctness (3), be undeliverable for discovery on most backends (2), and turn caller mistakes into silent wrong answers (1). Making `type` **optional** — default "any type" (= today's resolution), present = a typed fast path — captures the upside (skip the type-lookup and enable numeric operators where the type is known: ClickHouse, and all properties) at no correctness or compatibility cost and with no new *mandatory* backend capability. Three consequences follow:

- ClickHouse's `attribute_metadata` view (Option D, §8) is **not eliminated** — it becomes the *fallback* that resolves untyped predicates, and the source a discovery API would expose. Relocation makes the lookup *avoidable* when the type is supplied, not obsolete.
- The discovery API (§10 Q2) is the load-bearing piece for good typed UX, and it is realistically **ClickHouse-first**; other backends default to untyped.
- The flat backends ignore `type` (they store strings) and reject numeric operators (§7) — unchanged by any of this.

---

## 6. Proposed API

The two axes combine into one structured AST: a single, uniformly recursive **`Expression`**. An expression is either an *atom* — a reference (a level-qualified attribute or a property, §5) or a constant (a scalar, or a homogeneous array for `in`/`not_in`) — or a *call* applying an operator or function to argument expressions. Boolean combination (`and`/`or`/`not`), comparison (`eq`/`gt`/…), set membership, and future arithmetic/aggregation are all the same `Call` node, so `a AND b`, `span.a > span.b`, and `(a + b) > c` compose uniformly, and the expression is the one reusable term a future projection, grouping, or named function (§4 L3/L4) would operate on. The AST deliberately does **not** encode value types: a filter is an expression that *type-checks* to boolean, and `duration > "x"` is a type error but a valid graph — validated separately, as expression ASTs conventionally are (§6.1). `level`, `op`, and the optional `type` (§5.4) are **typed string enumerations** (documented closed value sets) rather than proto enums — see §6.2 for why; `property` is an open documented vocabulary.

### 6.1 Proto

```protobuf
// Expression is a node in the filter AST: either an atom — a reference (attr or
// prop) or a constant (scalar or array) — or a Call applying an operator or
// function to argument Expressions. The tree is uniformly recursive: a call's
// args are themselves Expressions, so boolean combination, comparison, set
// membership, and (later) arithmetic and aggregation are all the same shape,
// and `(a + b) > c` composes as naturally as `a AND b`.
//
// The AST does not encode value types. A well-formed filter is an Expression
// that evaluates to a boolean; `duration > "x"` and `"a" + 3` are type errors
// but valid AST graphs — rejected by a separate validation pass, not by the
// grammar. This keeps the node set minimal: as in most expression languages,
// type validity is a separate concern from grammatical structure.
message Expression {
  oneof term {
    Attribute attr   = 1;  // reference: level-qualified attribute
    string    prop   = 2;  // reference: built-in property — duration|name|service|status|kind|…
    Scalar    scalar = 3;  // constant: single typed value
    Array     array  = 4;  // constant: homogeneous list (right arg of in / not_in)
    Call      call   = 5;  // an operator/function applied to argument Expressions
  }
}

message Attribute {
  string key   = 1;  // attribute key, e.g. "http.status_code"
  string level = 2;  // span|resource|instrumentation|event|link; empty = span-or-resource
}

message Scalar {
  string value = 1;
  string type  = 2;  // optional hint: string(default)|int|double|bool; empty = any type (§5.4)
}

message Array {
  repeated string values = 1;
  string type = 2;          // optional hint applied to every element; empty = any type
}

// Call applies operator/function `op` to argument Expressions. (Named Call, not
// Operation, because api_v3 already has an `Operation` message — the span
// operation-name metadata returned by GetOperations.) Arity is implied by `op`:
// `not`/`exists` are unary; `and`/`or` take two or more args; the comparisons
// and `in`/`not_in` are binary ([left, right]). Because args are Expressions,
// `span.a > span.b` and `(a + b) > c` are expressible, not only `attr op scalar`;
// `in`/`not_in` take an Array as the right arg. Named scalar functions and
// aggregates (avg, count, coalesce, …) are future `op` values needing no new
// message — a function is just a call.
message Call {
  string op = 1;              // and|or|not | eq|ne|gt|lt|regex|exists|in|not_in | (future: gte|lte|not_regex|add|sub|avg|count|coalesce|…); empty = eq
  repeated Expression args = 2;
}

message TraceQueryParameters {
  // Legacy: unqualified AND-equality over the tag map. Retained unchanged.
  map<string, string> attributes = 3;

  // Structured filter: a single boolean-valued Expression. It is an
  // alternative to the legacy `attributes` map, not combined with it — a
  // request sets one or the other, and `filter` is authoritative when set. A
  // multi-predicate conjunction is an explicit `and` call; `or`/`not` nest the
  // same way.
  Expression filter = 10;
}
```

The filter is a single `Expression`, so there is exactly one way to express a conjunction — an `and` call — rather than a second, implicit one (a top-level list). This is the canonical, uniform shape: the filter *is* an expression, `or`/`not` at the top read directly instead of as a one-element list, and it matches how the prior-art structured query languages (§4) carry their filter. The extra `and` wrapper for the common multi-predicate case is trivial for a machine API and is emitted by the builder (§6.3) or shorthand (§7), never hand-written. (A top-level implicit-AND list was the alternative; see §10.)

### 6.2 REST/JSON encoding, and why string enumerations

Jaeger's api_v3 HTTP endpoint serializes with gogo/protobuf `jsonpb` at its defaults, so a proto *enum* would cross the wire as its full `CONSTANT_CASE` name (`"level":"ATTRIBUTE_LEVEL_SPAN"`) with no short-alias option, and proto3 enums are *open* (an unknown number is accepted, not rejected). Plain `string` fields avoid the verbosity, and the value set is still declared in the generated OpenAPI schema via the gnostic `enum` annotation, which validates it for generated clients and request validators (stricter there than an open proto enum). The closure is a schema-layer guarantee, not a proto one: the field stays a plain `string`, so at runtime an unknown `level`/`op` is caught by the backend rejecting it as *unsupported* (§7), not by the type system. That is deliberate — it is exactly what lets a backend treat an unrecognized value as "unsupported" rather than fail a type check.

```yaml
level: { type: string, enum: [span, resource, instrumentation, event, link] }  # Attribute.level
op:    { type: string, enum: [and, or, not, eq, ne, gt, lt, regex, exists, in, not_in] }  # Call.op
type:  { type: string, enum: [string, int, double, bool] }                      # Scalar.type / Array.type; optional, empty = any type
```

Legend: 🟢 strong · 🟡 adequate · 🔴 weak

| Criterion | Proto enums | Typed string constants¹ |
|-----------|:-:|:-:|
| REST/UI payload ergonomics | 🔴 `ATTRIBUTE_LEVEL_SPAN` | 🟢 `"span"` |
| Schema-level validation | 🟡 open enum (unknown ints pass) | 🟢 closed enum (rejects unknowns) |
| Discoverable / self-documenting | 🟢 proto + OpenAPI | 🟢 OpenAPI `enum` + codegen |
| Operator/level extensibility | 🟢 add enum value | 🟢 add a constant |
| Generated enum type for gRPC clients | 🟢 | 🔴 bare string |

¹ `string` proto field + OpenAPI `enum` annotation.

The only thing string constants give up is a generated enum *type* for strongly-typed gRPC clients — acceptable for a query surface, and the open string set is precisely what lets a backend treat an unrecognized level/operator as "unsupported" rather than failing a type check.

The recursive `Call` shape makes the raw JSON verbose — each call carries an `args` array whose entries name their kind (`attr`/`prop`/`scalar`/`array`/`call`). That verbosity is the deliberate cost of one uniform node that expresses `attr op attr` and keeps future L3/L4 in reach; humans are not expected to author it by hand — the §7 prefix shorthand does that. Spelled out, `span.http.status_code = 500` and `duration > 2s AND http.status_code in [500,503]` are:

```
GET /api/v3/traces?query.filter={"call":{"op":"eq","args":[{"attr":{"key":"http.status_code","level":"span"}},{"scalar":{"value":"500"}}]}}
```
```json
{ "query": { "filter": {
  "call": { "op": "and", "args": [
    { "call": { "op": "gt", "args": [
        { "prop": "duration" },
        { "scalar": { "value": "2s" } } ] } },
    { "call": { "op": "in", "args": [
        { "attr": { "key": "http.status_code", "level": "span" } },
        { "array": { "values": ["500", "503"], "type": "int" } } ] } } ] } } } }
```

A single predicate is the filter directly (the first example); a conjunction wraps its predicates in an `and` call (the second). The membership test is a single `in` call over an array, and `or`/`not` nest the same way as `and`.

Comparing two attributes is just another call with two `attr` args — "spans whose end-user id differs between the span and its resource":

```json
{ "call": { "op": "ne", "args": [
  { "attr": { "key": "enduser.id", "level": "span" } },
  { "attr": { "key": "enduser.id", "level": "resource" } } ] } }
```

### 6.3 Programmatic construction — a fluent builder

The verbose AST is comfortable for machines to *transport* but unpleasant to *assemble by hand*: a client SDK or automation that composes queries programmatically (as opposed to a human typing into a search box) should not be hand-building nested `call`/`args` dictionaries. The recommended ergonomics is a thin **fluent builder** in each client language that emits the §6.1 AST. It is the programmatic counterpart to the §7 prefix shorthand (the human on-ramp): a convenience layer over the same contract, not a second contract — a Go or TypeScript builder would compile to the identical AST. The builder is not a bespoke DSL: it follows the operator-overloading idiom well established across the Python ecosystem — SQLAlchemy, pandas, Django's `Q`, elasticsearch-dsl — so it reads as familiar to anyone who has composed queries in those libraries. A Python sketch:

```python
from jaeger.query import span, resource, event, prop, attr, Query

# References — level-qualified attributes and built-in properties
span("http.status_code")          # attr at the span level
resource("service.name")          # attr at the resource level
prop.duration                     # a property; prop("traceDuration") for others
attr("k8s.pod.name")              # unqualified (span-or-resource)

# Predicates — Python comparison operators build a `call`
prop.duration > "2s"                                  # gt
span("http.status_code") == 500                       # eq
span("http.method").matches("GET|POST")               # regex
resource("service.name").one_of(["cart", "checkout"]) # in   (also .not_one_of / .exists)
span("a") > span("b")                                 # attribute vs attribute

# Composition — &, |, ~  (or and_(...), or_(...), not_(...))
flt = (prop.duration > "2s") & span("http.status_code").one_of([500, 503])
flt = flt | ~resource("service.name").eq("healthcheck")

# Terminal — multiple .where() calls are ANDed into one Expression
q = (Query()
     .where(prop.duration > "2s")
     .where(span("http.status_code").one_of([500, 503]))
     .build())          # -> TraceQueryParameters.filter (an `and` of the two)
```

Each fragment lowers directly to the AST — `prop.duration > "2s"` produces `{"call":{"op":"gt","args":[{"prop":"duration"},{"scalar":{"value":"2s"}}]}}`. Two builder conveniences carry their weight:

- **Type-hint inference.** The builder derives the optional `type` (§5.4) from the Python value: `== 500` emits `{"scalar":{"value":"500","type":"int"}}`, `one_of([500,503])` emits an `int`-typed `array`, and a bare string stays untyped (any-type resolution). The caller opts out by passing an explicit string.
- **Operator mapping.** `== != > < >= <=` map to `eq/ne/gt/lt/gte/lte`; `& | ~` to `and/or/not`; and the operators Python cannot overload get method forms — `.matches()` (regex), `.exists()`, `.one_of()`/`.not_one_of()` (in/not_in), since `x in [...]` and `and`/`or` keywords cannot be intercepted. Method aliases (`.eq()`, `.gt()`, …) exist for callers who prefer them or want to avoid overloading `==` (which, as in SQLAlchemy/pandas, returns a query fragment, not a bool).

This is illustrative, not normative: the wire contract is the AST (§6.1), and each SDK is free to shape its builder idiomatically as long as it emits that AST.

---

## 7. Backward compatibility and degradation

**Coexistence.** The legacy `attributes` map is untouched and keeps its exact semantics (unqualified AND-equality). `filter` is a new additive field that defaults to empty, so old clients are byte-for-byte unaffected. `attributes` and `filter` are **alternatives, not a combination**: a request uses the legacy `attributes` map or the new `filter` for attribute matching, not both, and when `filter` is set it is authoritative. The query service builds one effective filter per request — from `filter` when present, otherwise by normalizing `attributes` — so a backend only ever sees a single attribute-filter model. This holds at all layers — public api_v3, internal storage API, and the remote-storage gRPC protocol.

**Normalizing legacy query parameters into `filter` (proposed architectural decision).** Most of today's top-level `TraceQueryParameters` fields are already properties (§5.2) — `service_name` → `service`, `operation_name` → `name`, `duration_min`/`duration_max` → a pair of `duration` range predicates — and `attributes` is a set of unqualified equality predicates. The query service should **normalize all of them into the single `filter` expression** before dispatching to a storage backend, so each backend implements exactly one filtering model (the AST) instead of the growing mix of scalar fields *plus* `attributes` *plus* `filter`. (`start_time_min`/`start_time_max` and `search_depth` stay as envelope parameters: they bound the scan window and the result count, they are not span predicates. Inclusive duration bounds imply `gte`/`lte`, which the extensible operator set can add — §5.3.)

This is clean for the **internal `TraceReader`** API, which is versioned with the binary and can simply drop the redundant scalar fields once the query service populates `filter`. It is harder at the **Remote Storage gRPC API**: those scalar fields are part of the published `storage.v2` contract and existing third-party plugins read them.

A plain additive `filter` field on the existing `FindTraces`/`FindTraceIDs` RPCs would by itself be a *silent* gap at the remote boundary: a plugin that predates `filter` ignores the unknown field and answers from the scalar fields alone — under-filtering with no signal to the query service. What closes that gap is a **separate capability declaration that gates the field**, the mechanism recorded in [ADR-013](../adr/013-storage-capability-declaration.md): a backend declares what it can search through `SearchCapabilities` on `tracestore.Reader`, enforced up front by the query service and carried across the remote boundary by the `jaeger.storage.v2.Capabilities` service ([jaeger-idl#211](https://github.com/jaegertracing/jaeger-idl/pull/211)). RFC 0005's filter capabilities — which levels a backend can filter, which operators it implements, whether it honors boolean `or`/`not` — are declared there as sibling fields, alongside the `WithoutServiceName` field ADR-013 shipped first.

So the query service asks before it dispatches: it sends the rich `filter` only to a backend that declares support, and **down-converts** to the legacy scalar fields + `attributes` (rejecting what those cannot express — e.g. `or`/`not`) for one that does not. A plugin predating the capability declares itself least capable — its `Capabilities` service is absent, which maps `UNIMPLEMENTED` → `errors.ErrUnsupported` → least capable, exactly as ADR-013 specifies — so the query service never sends it `filter` and the under-filtering case cannot arise; declaring the capability *is* the backend's promise to honor the field. No bespoke filter-aware `FindTraces` RPC is therefore needed: the capability declaration is the up-front signal, living on the main interface rather than a side one — the same lesson [#9067](https://github.com/jaegertracing/jaeger/pull/9067) drew when it removed the optional `SummaryReader` interface (which imposed a composition tax on every decorator and never protected the boundary anyway). The internal `TraceReader` cleanup — dropping the redundant scalar fields once the query service populates `filter` — can proceed independently. (Heavier fallbacks — mirroring the scalars alongside `filter` forever, or a whole-protocol major-version bump — apply only if the capability-declaration route is rejected; §10 Q5.)

**Capability-based degradation.** The backend-wide limits are *declared* through `SearchCapabilities` (ADR-013), so the query service refuses an unserviceable filter before it dispatches and the UI builder (M7) grays out what a backend cannot serve; a per-query predicate that no declared capability covers is *rejected* at query time as the backstop. Either way a backend never silently returns wrong results:

- **Levels** — ClickHouse honors all five. ES/OS honor span/resource/event today; instrumentation and link await schema evolution. The flat backends honor only the levels their write path indexes — span/resource/event — because instrumentation-scope attributes are merged into span tags and **link attributes are not stored at all** (§1.6). The honored level set is declared, so a predicate naming an unsupported level is refused — not widened, since widening would be a superset only for indexed levels and plain wrong for link.
- **Operators** — the implemented operator set is declared; a backend that has not implemented `regex`/`gt`/… does not advertise it, and the query service refuses such a predicate rather than letting the backend approximate.
- **Boolean structure** — ClickHouse and ES/OS declare full boolean support; the flat backends declare conjunction-only, so an `or`/`not` operation is refused up front while their conjunctive subset still runs.
- **Remote-storage plugins** — a plugin that declares no filter capability (or predates the `Capabilities` service, and so reads as least capable) receives only the legacy `attributes` and behaves exactly as today; the query service populates `attributes` from a purely-conjunctive, unqualified `filter` for it.

**Prefix syntax as the human on-ramp.** The verbose structured form is machine-first. For humans (the UI text box, `curl`), the query parser accepts a prefix shorthand that normalizes into the structured expression — `resource.service.name:foo` → an `eq` operation over `attribute{key:"service.name",level:"resource"}` and `scalar{"foo"}`; `duration>2s` → a `gt` operation over `property:"duration"` and `scalar{"2s"}`. This is a convenience layer over the same AST, not a second contract, and it means the UI need never emit the verbose operand JSON by hand.

---

## 8. Considered alternatives

Three alternative API shapes were considered and not adopted; the structured model of §4–§6 is preferred to each:

- **A — change the default level of the existing `attributes` field** (a `search_all_attribute_scopes` boolean). *Rejected.* It silently changes the semantics of an existing field (a migration flag-day), offers only binary "span+resource vs all" precision, and extends to neither operators nor boolean composition. A dead end.
- **B — encode the level as a key prefix** (`resource.k8s.namespace.name`). *Not a competing data model — adopted as text sugar* (§7). As an API contract it is rejected: the convention is implicit and unvalidated, collides with user keys that happen to start with a level name, and cannot express operators or booleans.
- **D — backend metadata level-skipping** (ClickHouse consults its `attribute_metadata` view to skip levels a key was never seen at). *Orthogonal.* A ClickHouse-local performance optimization requiring no API change; out of scope here and free to proceed independently on its own track.
- **A free-text query language** (parse a TraceQL/BTQL/SQL string). *Non-goal* (§2). Jaeger commits to a structured AST; a text surface, if ever desired, can compile to this same AST without changing the contract.

---

## 9. Implementation roadmap

PR-sized milestones with explicit exit bars, grouped into stages. The API is L2 from the start; capable backends (ClickHouse, ES/OS) evaluate the full tree, while the flat backends support only its conjunctive subset. The north star is the cross-backend API contract, where the coordination cost lives.

**Stage A — API foundation (additive, no behavior change)**

- **M1 — Proto types in jaeger-idl.** Add `Expression`, `Attribute`, `Scalar`, `Array`, and `Call` (with `level`/`op`/`type` as string enumerations whose closed sets are declared in the OpenAPI schema, and `prop` as an open documented string — §6.2) and the `filter` field on `TraceQueryParameters`, in both the public api_v3 and the storage/v2 protos. Legacy `attributes` untouched. *Initial delivery may ship the attr and scalar terms and add the `prop` and `array` terms in a follow-up, since the oneof is additive.* **In flight — [jaeger-idl#206](https://github.com/jaegertracing/jaeger-idl/pull/206), which encodes the recursive `Expression` + `Call` AST (the `attr`/`prop`/`scalar`/`array`/`call` terms and the `level`/`op`/`type` string enumerations) per §6.1–§6.2.** *Exit:* generated types compile and vendor cleanly; existing api_v3 callers byte-for-byte unaffected.
- **M2 — Plumb the filter through the query service to the storage interface.** Extend the internal `TraceQueryParams` ([`reader.go`](../../internal/storage/v2/api/tracestore/reader.go)) to carry the expression tree alongside the legacy `Attributes` map, and translate the proto field in the api_v3 handler. With no backend routing yet, a purely-conjunctive tree is treated as unqualified search-all (today's results); non-conjunctive trees and unsupported operators are refused at the edge — up front where the query service can read the backend's declared filter capabilities (`SearchCapabilities`, [ADR-013](../adr/013-storage-capability-declaration.md)), at query time otherwise. *Exit:* a conjunctive level-qualified filter reaches every backend as unqualified attributes and returns today's results; `OR`/`NOT` and unsupported operators are refused; plugins ignoring `filter` are unaffected.

**Stage B — Backend routing (one PR per backend, parallelizable after M2)**

- **M3 — ClickHouse.** Route level-qualified predicates to their typed Map column ([`query_builder.go`](../../internal/storage/v2/clickhouse/tracestore/query_builder.go)) and lower the boolean tree into the SQL `WHERE` (`AND`/`OR`/`NOT`); an empty level keeps the span-or-resource expansion. *Exit:* level-qualified/boolean queries emit the corresponding SQL; unqualified queries byte-identical to today.
- **M4 — Elasticsearch/OpenSearch.** Route span/resource/event levels to their fields in `buildTagQuery` ([`core/reader.go`](../../internal/storage/v2/elasticsearch/tracestore/core/reader.go)) and lower the boolean tree into a `bool` query; the instrumentation and link levels are rejected pending schema evolution. *Exit:* span/resource/event level-qualification and `AND`/`OR`/`NOT` work; unqualified snapshots byte-identical.
- **M5 — Cassandra + Badger (capability boundary).** Accept the conjunctive subset over indexed levels (span/resource/event); **declare** the honored level set, operator set, and conjunction-only boolean support as `SearchCapabilities` fields ([ADR-013](../adr/013-storage-capability-declaration.md)) so the query service refuses `OR`/`NOT`, unsupported operators, and predicates naming an unindexed level (link, instrumentation) up front — never silently widen (§1.6). *Exit:* supported predicates return correct supersets; unsupported ones are refused cleanly; a cross-backend conformance test asserts both.

**Stage C — Ergonomics and UI**

- **M6 — Prefix/shorthand parser** (§7) — normalize `resource.k:v`, `duration>2s`, etc. into the AST in the api_v3 HTTP parser ([`query_parser.go`](../../cmd/jaeger/internal/extension/jaegerquery/internal/apiv3/query_parser.go)). *Exit:* shorthand reaches storage as the structured predicate; unprefixed keys unchanged.
- **M7 — UI builder** — a filter builder emitting `filter`, starting with the conjunctive subset (chips with a level/property selector) and adding nested groups later; the legacy text box keeps populating `attributes`. *Exit:* existing search unaffected; qualified predicates emit `filter`.

**Out of scope (future, this model enables):**
- Properties beyond the initial set (trace-level `traceDuration`/`rootName`/`rootService`, IDs) — §5.2.
- Levels beyond the OTLP five (e.g. `parent.`, the parent span's attributes) — §5.1.
- ES/OS schema evolution to index instrumentation and link attributes distinctly (§1.6) — unblocks those levels in M4.
- Option D — ClickHouse metadata level-skipping (§8); backend-local, no coordination.
- A discovery API returning keys, their type(s), and sample values per level — the piece that feeds typed predicates and autocomplete (§5.4, §10 Q2); ClickHouse-first.
- Tiers L3–L5 (§4): result shaping, aggregation/metrics (metrics subsystem), and structural/trace-tree queries (post-fetch only — not push-down-able, so inefficient at scale).

---

## 10. Open questions

1. **Top-level shape (decided).** A single `Expression filter` (§6.1), not a top-level implicit-AND list. A single expression gives one canonical way to express a conjunction — an `and` call — consistent with the uniform recursive model and the prior-art query languages (§4); a list would add a second, implicit encoding of AND. The list's only edge, flat raw-JSON conjunctions, is absorbed by the builder (§6.3) and shorthand (§7).
2. **Attribute discovery (keys, types, values).** Add a discovery API so the UI can autocomplete valid keys per level *and their type(s)* — a key may have several (§5.4) — plus sample values, so the builder emits correctly-typed predicates. This is the load-bearing piece for typed UX (§5.4). ClickHouse's `attribute_metadata` supports it directly; ES/OS only partially and the flat backends not at all — so typed authoring assistance is ClickHouse-first, and other backends default to untyped.
3. **Conjunction semantics across spans.** Must `resource.service.name=foo AND span.http.status_code=500` match the *same* span, or may they match different spans of the same trace? (The internal `TraceReader.FindTraces` contract currently leaves this implementation-dependent.)
4. **Property phasing.** Which properties are required in the first implementation (`duration`/`name`/`service`/`status`/`kind`) vs deferred (trace-level, IDs)?
5. **Remote-storage capability rollout (§7).** Confirm that filter support and its granularity (which levels/operators/boolean) are declared through the [ADR-013](../adr/013-storage-capability-declaration.md) `SearchCapabilities` mechanism and its `jaeger.storage.v2.Capabilities` service ([jaeger-idl#211](https://github.com/jaegertracing/jaeger-idl/pull/211)) — sibling fields alongside `WithoutServiceName` — rather than a bespoke filter-aware `FindTraces` RPC. `filter` then rides the existing RPC as a plain additive field, gated by the declaration: a backend that declares no support (or predates the service) is sent only legacy scalars, so there is no silent under-filtering. The heavier fallbacks (mirror the legacy scalars alongside `filter` indefinitely, or a whole-protocol major bump) apply only if the capability-declaration route is rejected. Either way the internal `TraceReader` cleanup is not blocked.
6. **Prefix collision escape hatch.** Does the shorthand (§7) need an escape for user keys that literally begin with a level name, or is the structured JSON form the sufficient unambiguous alternative?
7. **Units of numeric values (§5.3).** Confirm that `duration` (and any future unit-bearing property) carries an explicit unit via Go duration syntax (`2s`), while bare numeric values stay unit-less and are compared as-is. Do any other properties need an explicit unit or format convention (e.g. timestamps)?

---

## 11. References

**Jaeger code**
- [Internal storage API `TraceQueryParams`](../../internal/storage/v2/api/tracestore/reader.go) — current unqualified `Attributes` field
- [ClickHouse query builder](../../internal/storage/v2/clickhouse/tracestore/query_builder.go) — 5-level OR expansion
- [ClickHouse attribute metadata](../../internal/storage/v2/clickhouse/tracestore/attribute_metadata.go) — type/level metadata view (Option D)
- [Elasticsearch tag query](../../internal/storage/v2/elasticsearch/tracestore/core/reader.go) — multi-field OR expansion
- [api_v3 HTTP query parser](../../cmd/jaeger/internal/extension/jaegerquery/internal/apiv3/query_parser.go) — `query.attributes` parsing
- [jaeger-idl#206](https://github.com/jaegertracing/jaeger-idl/pull/206) — proto foundation (M1)
- [ADR-013](../adr/013-storage-capability-declaration.md) — storage capability declaration (`SearchCapabilities`), the mechanism RFC 0005's filter capabilities plug into (§7)
- [jaeger-idl#211](https://github.com/jaegertracing/jaeger-idl/pull/211) — the `jaeger.storage.v2.Capabilities` service (§7)
- [#9067](https://github.com/jaegertracing/jaeger/pull/9067) — merged `FindTraceSummaries` into the main `tracestore.Reader`; corroborates declaring the capability on the main interface, not a side one (§7)

**External**
- [OpenTelemetry trace data model](https://opentelemetry.io/docs/specs/otel/trace/api/) — the five attribute levels
- [Grafana TraceQL documentation](https://grafana.com/docs/tempo/latest/traceql/) and [TraceQL overview (Giant Swarm)](https://docs.giantswarm.io/overview/observability/data-management/data-exploration/traceql/) — scopes, intrinsics, and structural/metrics tiers
- [Braintrust BTQL](https://www.braintrust.dev/docs/reference/btql) — structured SQL-like query language (prior art)
