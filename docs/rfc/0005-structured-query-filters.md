# RFC 0005: Structured Query Filters for Trace Search

- **Status:** Draft
- **Author:** Yuri Shkuro
- **Created:** 2026-06-19
- **Last Updated:** 2026-07-09

---

## Abstract

Jaeger's trace-search API filters spans by unqualified key-value tag pairs, implicitly ANDed, matched against every attribute location in the span. This RFC defines a **structured query-filter model** for trace search that (1) lets a predicate reference a specific attribute *level* (span / resource / instrumentation / event / link) or a built-in *property* (duration, name, status, …), (2) composes predicates with **boolean operators** (`AND`/`OR`/`NOT`), and (3) keeps the existing unqualified tag filter working unchanged.

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

When a user queries `http.status_code=500`, an unqualified backend must search all five levels with OR logic. In ClickHouse this expands to five separate `arrayExists()` calls (three top-level columns, two nested within event/link arrays), each scanning a typed Map column. In Elasticsearch each unqualified tag expands to a `bool.should` across the field locations, increasing sub-query count and reducing cache effectiveness. The cost is paid on every attribute of every search, even though the user almost always knows which level they mean.

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

- **G1 — Level-qualified attributes.** A predicate may target a single OTLP attribute level (span/resource/instrumentation/event/link) or leave it unqualified (search the default).
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
- **Predicate anatomy (§5)** — *a single predicate's subject (a level-qualified attribute or a property), operator, and value type.*

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

¹ ES `bool` query (`must`/`should`/`must_not`) nests arbitrarily. ² ES aggregations exist but overlap Jaeger's metrics/SPM path. ³ structural operators are evaluated *post-fetch* — the query service assembles each candidate trace and checks ancestor/descendant/sibling in memory — so they work on any backend; but no Jaeger schema encodes the trace tree, so they cannot be pushed into storage to prune candidates, which makes them **inefficient at scale, not infeasible**. ⁴ ClickHouse could additionally push some structural checks into a self-join within a trace partition, though not with today's schema/query builder; otherwise it is post-fetch as in ³. ⁵ superset-safe only for the levels the flat index actually contains — span/resource/event; link is unrepresentable and instrumentation indistinguishable (§1.6). ⁶ a flat inverted index has no `OR`/`NOT`. ⁷ L2 is not a delta in message types at all — boolean `and`/`or`/`not` are just more `op` values on the same `Operation` node the conjunctive subset already uses; see §6. ⁸ the API need not wait for the UI: a builder can render the conjunctive subset first and add nesting later. ⁹ capable backends evaluate the full tree; flat backends evaluate the conjunctive subset and reject `OR`/`NOT` — the same posture they already take for unsupported operators.

**Decision — target L2 (the full boolean tree); conjunction is the subset every backend supports.**

- **L1 is not a coherent stopping point.** In SELECT/FILTER/GROUP-BY terms, L3 adds SELECT and L4 adds GROUP BY — separate clauses, principled to defer. But L1 stops *inside* the FILTER clause: it has conjunction and lacks disjunction/negation, which is no natural boundary. The complete FILTER is the boolean expression — L2.
- **The backend-uniformity concern does not favor L1.** A flat-index backend handles the conjunctive *subset* of an L2 tree exactly as it would handle L1 — walk the ANDs, reject anything containing `OR`/`NOT`. So L1 buys the weak backends no simplicity; it only removes power from ClickHouse and ES/OS, the backends that motivate this RFC. L1 is L2 with the other node types deleted from the schema.
- **API expressiveness is decoupled from UI expressiveness.** The API can be L2 while the UI ships only a conjunctive-subset builder and adds nested groups later.
- **Stopping at L1 would cost two API changes** to the same surface and leave a flat predicate-list field as legacy baggage beside the legacy `attributes` map.

So the committed filter API is the **L2 boolean expression tree** (§6). "L1" is retained only as a *capability tier* — the conjunctive subset that every backend, including the flat ones, supports. **L3 is deferred** (awkward against Jaeger's whole-trace result model, and inert until L4 exists). **L4 is excluded** (belongs to the metrics/SPM subsystem; a separate RFC). **L5 is excluded** — not for infeasibility (structural predicates can be evaluated post-fetch on any backend, assembling each candidate trace) but because it is a distinct fetch-then-filter execution model that cannot prune in storage, is inefficient at scale, and is a large surface; deferred as a separate effort. The one honest internal nuance is that a pure conjunction admits a fast all-predicates-pushdown path while a tree with `OR` needs fuller evaluation — an optimization inside the capable backends, not an API concern.

**Relation to TraceQL — why the exclusions are bounded, not dead ends.** TraceQL's AST is a *pipeline of spanset operations*: a chain (`|`) of spanset filters, structural operators (`>>`/`<<`/`>`/`<`/`~`), `select()`, `by()`, and aggregates, over a per-span *field expression* (attribute/intrinsic/static combined by boolean, comparison, and arithmetic operators). This RFC's `Expression` corresponds to exactly one TraceQL construct — the field expression inside a single spanset filter `{ … }` — and shares its shape: TraceQL builds that field expression from one recursive `BinaryOperation`/`UnaryOperation` type carrying arithmetic, comparison, and boolean operators alike, exactly as our `Operation` does. That correspondence is the reassurance behind the deferrals: the excluded tiers extend this AST rather than replace it. L3/L4 add sibling clauses over the same `Expression` (a `select` is a list of expressions, a `by` is an expression, an aggregate is an `Operation`). L5 adds an *outer* layer — a pipeline whose per-spanset filter is an `Expression` — so structural queries would wrap this AST, not force a redesign of it. What this AST cannot yet express and would need shape work to add later is narrower: **set membership over a list** (addressed here via `in`/`not_in` + `Array`, §6.1) and TraceQL's `parent.` modifier, which is orthogonal to `level` (a parent-scope flag over a scope, not a scope value) and belongs with the deferred structural tier. Arithmetic and richer operators (`>=`/`<=`, `!~`) and semantic literal types (duration/status/kind) are pure additions to the open `op`/`type` vocabularies — no new node types.

---

## 5. Predicate anatomy — subject, operator, and value type

A predicate has three parts. Its **subject** — what it filters on — is exactly one of a _level-qualified attribute_ (§5.1) or a _property_ (§5.2). Its **operator** (§5.3) compares that subject against a **value**, usually a constant (a scalar, or an array for `in`/`not_in`) but — because both sides are operands of the same kind — optionally another subject (`span.a > span.b`; §6.1). A constant also carries an optional **type** (§5.3–§5.4) telling the backend how to interpret it.

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

The empty default means span-or-resource rather than "all five" because that is what Jaeger indexes and searches by default today: span attributes and resource (process) attributes are the tags reliably indexed for search, whereas event (span-log) attributes generally are not. So span-or-resource is at once the high-coverage common case *and* the behavior existing unqualified queries already get — making it the default preserves today's semantics rather than silently widening the search or paying to scan levels that are not indexed. A backend that does index more, or that expands to all levels (as ClickHouse can, at the §1.3 cost), simply returns a superset (§1.6). A further level such as `parent.` (the parent span's attributes) could be added later — the vocabulary is an open string set (§6).

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

The value of the property model is that it is *uniform*: `duration > 2s`, `status = error`, and `span.http.method = GET` are all the same shape (a predicate with an operator), instead of three unrelated mechanisms (a dedicated duration field, a magic `error` tag, and a tag map). It also makes queries expressible that are impossible today (`traceDuration`, `rootName`). The dedicated top-level query fields (`service_name`, `operation_name`, the paired `duration_min`/`duration_max`) remain supported for backward compatibility, but the query service **normalizes them into property predicates in `filters`** (`duration_min`/`duration_max` become a pair of `duration` range predicates) so that storage backends implement one filtering model rather than a growing mix of scalar fields *plus* `attributes` *plus* `filters`. That normalization is an architectural choice with a compatibility wrinkle at the remote-storage boundary — see §7.

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

The two axes combine into one structured AST: a single, uniformly recursive **`Expression`**. An expression is either an *atom* — a reference (a level-qualified attribute or a property, §5) or a constant (a scalar, or a homogeneous array for `in`/`not_in`) — or an *operation* applying an operator to operand expressions. Boolean combination (`and`/`or`/`not`), comparison (`eq`/`gt`/…), set membership, and future arithmetic/aggregation are all the same `Operation` node, so `a AND b`, `span.a > span.b`, and `(a + b) > c` compose uniformly, and the expression is the one reusable term a future projection, grouping, or call (§4 L3/L4) would operate on. The AST deliberately does **not** encode value types: a filter is an expression that *type-checks* to boolean, and `duration > "x"` is a type error but a valid graph — validated separately, as expression ASTs conventionally are (§6.1). `level`, `op`, and the optional `type` (§5.4) are **typed string enumerations** (documented closed value sets) rather than proto enums — see §6.2 for why; `property` is an open documented vocabulary.

### 6.1 Proto

```protobuf
// Expression is a node in the filter AST: either an atom — a reference
// (attribute or property) or a constant (scalar or array) — or an Operation
// applied to operand Expressions. The tree is uniformly recursive: an
// operation's operands are themselves Expressions, so boolean combination,
// comparison, set membership, and (later) arithmetic and aggregation are all
// the same shape, and `(a + b) > c` composes as naturally as `a AND b`.
//
// The AST does not encode value types. A well-formed filter is an Expression
// that evaluates to a boolean; `duration > "x"` and `"a" + 3` are type errors
// but valid AST graphs — rejected by a separate validation pass, not by the
// grammar. This keeps the node set minimal and matches how expression ASTs are
// conventionally typed (see TraceQL's single `BinaryOperation`, §4).
message Expression {
  oneof term {
    Attribute attribute = 1;  // reference: level-qualified attribute
    string    property  = 2;  // reference: built-in value — duration|name|service|status|kind|…
    Scalar    scalar    = 3;  // constant: single typed value
    Array     array     = 4;  // constant: homogeneous list (right operand of in / not_in)
    Operation operation = 5;  // operator applied to operand Expressions
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

// Operation applies `op` to operand Expressions. Arity is implied by the
// operator: `not`/`exists` are unary; `and`/`or` take two or more operands; the
// comparisons and `in`/`not_in` are binary ([left, right]). Because operands
// are Expressions, `span.a > span.b` and `(a + b) > c` are expressible, not
// only `attribute op scalar`; `in`/`not_in` take an Array as the right operand.
message Operation {
  string op = 1;                  // and|or|not | eq|ne|gt|lt|regex|exists|in|not_in | (future: gte|lte|not_regex|add|sub|avg|count|…); empty = eq
  repeated Expression operands = 2;
}

message TraceQueryParameters {
  // Legacy: unqualified AND-equality over the tag map. Retained unchanged.
  map<string, string> attributes = 3;

  // Structured filter: each element is a boolean-valued Expression. The
  // top-level list is implicitly ANDed (and ANDed with `attributes`), so the
  // common conjunction reads as a flat array while any element may nest via an
  // `and`/`or`/`not` Operation.
  repeated Expression filters = 10;
}
```

The top-level `repeated Expression` is implicitly ANDed. This keeps the dominant conjunction case as ergonomic as a flat list while still allowing full boolean nesting inside any element. (A single-root `Expression filter` is the alternative shape — marginally more uniform but forcing an explicit `and` operation for the common multi-predicate case; see §10.)

### 6.2 REST/JSON encoding, and why string enumerations

Jaeger's api_v3 HTTP endpoint serializes with gogo/protobuf `jsonpb` at its defaults, so a proto *enum* would cross the wire as its full `CONSTANT_CASE` name (`"level":"ATTRIBUTE_LEVEL_SPAN"`) with no short-alias option, and proto3 enums are *open* (an unknown number is accepted, not rejected). Plain `string` fields avoid the verbosity, and the closed value set is still declared and validated in the generated OpenAPI schema via the gnostic `enum` annotation — a **closed** set (unknown values rejected, stricter than an open proto enum):

```yaml
level: { type: string, enum: [span, resource, instrumentation, event, link] }  # Attribute.level
op:    { type: string, enum: [and, or, not, eq, ne, gt, lt, regex, exists, in, not_in] }  # Operation.op
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

The recursive `Operation` shape makes the raw JSON more verbose than a fixed `subject op value` triple would — each operation carries an `operands` array whose entries name their kind (`attribute`/`property`/`scalar`/`array`/`operation`). This is the deliberate cost of one uniform node that keeps `attr op attr` and future L3/L4 expressible; humans are not expected to author it by hand — the §7 prefix shorthand does that. Spelled out, `span.http.status_code = 500` and `duration > 2s AND http.status_code in [500,503]` are:

```
GET /api/v3/traces?query.filters=[{"operation":{"op":"eq","operands":[{"attribute":{"key":"http.status_code","level":"span"}},{"scalar":{"value":"500"}}]}}]
```
```json
{ "query": { "filters": [
  { "operation": { "op": "gt", "operands": [
      { "property": "duration" },
      { "scalar": { "value": "2s" } } ] } },
  { "operation": { "op": "in", "operands": [
      { "attribute": { "key": "http.status_code", "level": "span" } },
      { "array": { "values": ["500", "503"], "type": "int" } } ] } } ] } }
```

The second filter reads as a single `in` operation instead of an `or` of two `eq`s. Genuine disjunction nests via an `or`/`not` operation whose operands are themselves expressions.

The subject-vs-subject case that the fixed-triple shape could not express — "spans whose end-user id differs between the span and its resource" — is just another operation with two `attribute` operands:

```json
{ "operation": { "op": "ne", "operands": [
  { "attribute": { "key": "enduser.id", "level": "span" } },
  { "attribute": { "key": "enduser.id", "level": "resource" } } ] } }
```

---

## 7. Backward compatibility and degradation

**Coexistence.** The legacy `attributes` map is untouched and keeps its exact semantics (unqualified AND-equality). `filters` is a new additive field that defaults to empty; old clients are byte-for-byte unaffected, and the two may be combined (all ANDed). This holds at all layers — public api_v3, internal storage API, and the remote-storage gRPC protocol.

**Normalizing legacy query parameters into `filters` (proposed architectural decision).** Most of today's top-level `TraceQueryParameters` fields are already properties (§5.2) — `service_name` → `service`, `operation_name` → `name`, `duration_min`/`duration_max` → a pair of `duration` range predicates — and `attributes` is a set of unqualified equality predicates. The query service should **normalize all of them into the single `filters` expression** before dispatching to a storage backend, so each backend implements exactly one filtering model (the AST) instead of the growing mix of scalar fields *plus* `attributes` *plus* `filters`. (`start_time_min`/`start_time_max` and `search_depth` stay as envelope parameters: they bound the scan window and the result count, they are not span predicates. Inclusive duration bounds imply `gte`/`lte`, which the extensible operator set can add — §5.3.)

This is clean for the **internal `TraceReader`** API, which is versioned with the binary and can simply drop the redundant scalar fields once the query service populates `filters`. It is harder at the **Remote Storage gRPC API**: those scalar fields are part of the published `storage.v2` contract and existing third-party plugins read them.

Crucially, **a bare additive `filters` field on the existing `FindTraces`/`FindTraceIDs` RPCs is not enough** for the remote boundary: a plugin that predates `filters` silently ignores the unknown field and answers from the scalar fields alone — under-filtering with *no signal* to the query service. A new *field* on an existing RPC yields no such signal; only a *method an old binary does not have* does. So `filters` must ride a **new, filter-aware RPC**: a plugin built without it returns gRPC `UNIMPLEMENTED` (the generated `UnimplementedTraceReaderServer` provides this for free), which the remote client normalizes to `errors.ErrUnsupported`. The query service routes the rich `filters` query to backends that implement the new RPC and **down-converts** to the existing V2 call (legacy scalar fields + `attributes`, rejecting what V2 cannot express — e.g. `OR`/`NOT`) for those that don't. This turns a silent gap into an explicit capability check, moves capable backends to the single filter model with no redundant mirroring and no forced protocol-wide break, and leaves old plugins untouched.

This is deliberately **standard gRPC method-presence signaling, not a bespoke optional Go interface.** Jaeger tried the interface route for `FindTraceSummaries` — an optional `tracestore.SummaryReader` discovered at runtime by type-assertion — and [#9067](https://github.com/jaegertracing/jaeger/pull/9067) removed it, folding the method into the main `tracestore.Reader`: the optional interface imposed a composition tax (every decorator wrapping a reader had to re-detect and re-expose the capability or silently drop it — it regressed twice), and it never protected the remote boundary anyway, which was always the `UNIMPLEMENTED`→`ErrUnsupported` path. So `filters` follows that corrected model rather than the withdrawn one: it is a field on the *main* reader's query params (an in-tree backend that cannot honor a predicate returns `ErrUnsupported`, which the gRPC server translates to `UNIMPLEMENTED`), and the remote surface gains just the one new RPC — no side interface to re-plumb through every decorator. The internal `TraceReader` cleanup can proceed as soon as the in-tree backends read predicates from `filters` (Stage B), independent of this remote-API work. (Alternatives considered — mirroring the scalars alongside `filters` forever, or a whole-protocol major-version bump — are heavier and are the fallback if the new-RPC route is rejected; §10 Q5.)

**Capability-based degradation.** A backend advertises what it can honor and *rejects* what it cannot, rather than silently returning wrong results:

- **Levels** — ClickHouse honors all five. ES/OS honor span/resource/event today; instrumentation and link await schema evolution. The flat backends honor only the levels their write path indexes — span/resource/event — because instrumentation-scope attributes are merged into span tags and **link attributes are not stored at all** (§1.6). A predicate naming an unsupported level is rejected (`Unimplemented`), not widened — widening would be a superset only for indexed levels and plain wrong for link.
- **Operators** — a backend that has not implemented `regex`/`gt`/… rejects that predicate; it never approximates.
- **Boolean structure** — ClickHouse and ES/OS evaluate the full L2 tree. Flat backends evaluate the conjunctive subset and reject any `or`/`not` operation.
- **Remote-storage plugins** — a plugin that ignores the new `filters` field still receives the legacy `attributes` and behaves exactly as today; the query service can populate `attributes` from a purely-conjunctive, unqualified `filters` for such plugins.

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

- **M1 — Proto types in jaeger-idl.** Add `Expression`, `Attribute`, `Scalar`, `Array`, and `Operation` (with `level`/`op`/`type` as string enumerations whose closed sets are declared in the OpenAPI schema, and `property` as an open documented string — §6.2) and the `filters` field on `TraceQueryParameters`, in both the public api_v3 and the storage/v2 protos. Legacy `attributes` untouched. *Initial delivery may ship the attribute and scalar terms and add the `property` and `array` terms in a follow-up, since the oneof is additive.* **In flight — [jaeger-idl#206](https://github.com/jaegertracing/jaeger-idl/pull/206), which encodes the recursive `Expression` AST (`Expression` + `Operation` over `Attribute`/`Scalar`/`Array` atoms + the `filters` field) with the `level`/`op`/`type` string enumerations per §6.1–§6.2.** *Exit:* generated types compile and vendor cleanly; existing api_v3 callers byte-for-byte unaffected.
- **M2 — Plumb the filter through the query service to the storage interface.** Extend the internal `TraceQueryParams` ([`reader.go`](../../internal/storage/v2/api/tracestore/reader.go)) to carry the expression tree alongside the legacy `Attributes` map, and translate the proto field in the api_v3 handler. With no backend routing yet, a purely-conjunctive tree is treated as unqualified search-all (today's results); non-conjunctive trees and unsupported operators are rejected at the edge. *Exit:* a conjunctive level-qualified filter reaches every backend as unqualified attributes and returns today's results; `OR`/`NOT` and unsupported operators return `Unimplemented`; plugins ignoring `filters` are unaffected.

**Stage B — Backend routing (one PR per backend, parallelizable after M2)**

- **M3 — ClickHouse.** Route level-qualified predicates to their typed Map column ([`query_builder.go`](../../internal/storage/v2/clickhouse/tracestore/query_builder.go)) and lower the boolean tree into the SQL `WHERE` (`AND`/`OR`/`NOT`); an empty level keeps the span-or-resource expansion. *Exit:* level-qualified/boolean queries emit the corresponding SQL; unqualified queries byte-identical to today.
- **M4 — Elasticsearch/OpenSearch.** Route span/resource/event levels to their fields in `buildTagQuery` ([`core/reader.go`](../../internal/storage/v2/elasticsearch/tracestore/core/reader.go)) and lower the boolean tree into a `bool` query; the instrumentation and link levels are rejected pending schema evolution. *Exit:* span/resource/event level-qualification and `AND`/`OR`/`NOT` work; unqualified snapshots byte-identical.
- **M5 — Cassandra + Badger (capability boundary).** Accept the conjunctive subset over indexed levels (span/resource/event); **reject** `OR`/`NOT`, unsupported operators, and predicates naming an unindexed level (link, instrumentation) with `Unimplemented` — never silently widen (§1.6). *Exit:* supported predicates return correct supersets; unsupported ones error cleanly; a cross-backend conformance test asserts both.

**Stage C — Ergonomics and UI**

- **M6 — Prefix/shorthand parser** (§7) — normalize `resource.k:v`, `duration>2s`, etc. into the AST in the api_v3 HTTP parser ([`query_parser.go`](../../cmd/jaeger/internal/extension/jaegerquery/internal/apiv3/query_parser.go)). *Exit:* shorthand reaches storage as the structured predicate; unprefixed keys unchanged.
- **M7 — UI builder** — a filter builder emitting `filters`, starting with the conjunctive subset (chips with a level/property selector) and adding nested groups later; the legacy text box keeps populating `attributes`. *Exit:* existing search unaffected; qualified predicates emit `filters`.

**Out of scope (future, this model enables):**
- Properties beyond the initial set (trace-level `traceDuration`/`rootName`/`rootService`, IDs) — §5.2.
- Levels beyond the OTLP five (e.g. `parent.`, the parent span's attributes) — §5.1.
- ES/OS schema evolution to index instrumentation and link attributes distinctly (§1.6) — unblocks those levels in M4.
- Option D — ClickHouse metadata level-skipping (§8); backend-local, no coordination.
- A discovery API returning keys, their type(s), and sample values per level — the piece that feeds typed predicates and autocomplete (§5.4, §10 Q2); ClickHouse-first.
- Tiers L3–L5 (§4): result shaping, aggregation/metrics (metrics subsystem), and structural/trace-tree queries (post-fetch only — not push-down-able, so inefficient at scale).

---

## 10. Open questions

1. **Top-level shape.** `repeated Expression filters` (implicit-AND list, best conjunction ergonomics) vs a single-root `Expression filter` (marginally more uniform, but forces an explicit `and` operation for multi-predicate queries)? §6.1 recommends the former.
2. **Attribute discovery (keys, types, values).** Add a discovery API so the UI can autocomplete valid keys per level *and their type(s)* — a key may have several (§5.4) — plus sample values, so the builder emits correctly-typed predicates. This is the load-bearing piece for typed UX (§5.4). ClickHouse's `attribute_metadata` supports it directly; ES/OS only partially and the flat backends not at all — so typed authoring assistance is ClickHouse-first, and other backends default to untyped.
3. **Conjunction semantics across spans.** Must `resource.service.name=foo AND span.http.status_code=500` match the *same* span, or may they match different spans of the same trace? (The internal `TraceReader.FindTraces` contract currently leaves this implementation-dependent.)
4. **Property phasing.** Which properties are required in the first implementation (`duration`/`name`/`service`/`status`/`kind`) vs deferred (trace-level, IDs)?
5. **Remote-storage capability rollout (§7).** Confirm the **new filter-aware RPC**, with capability detected via standard gRPC `UNIMPLEMENTED`→`ErrUnsupported`, as the way to expose `filters` over remote storage — the query service down-converts to the existing V2 call for plugins that don't implement it. This is method-presence signaling on the main reader, *not* a resurrected optional interface: [#9067](https://github.com/jaegertracing/jaeger/pull/9067) removed that shape for `FindTraceSummaries` (composition tax; the boundary was always the `UNIMPLEMENTED` path). The heavier fallbacks (mirror the legacy scalars alongside `filters` indefinitely, or a whole-protocol major bump) apply only if the new-RPC route is rejected. Either way the internal `TraceReader` cleanup is not blocked.
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
- [#9067](https://github.com/jaegertracing/jaeger/pull/9067) — merged `FindTraceSummaries` into the main `tracestore.Reader`, the capability-signaling precedent for the remote `filters` RPC (§7)

**External**
- [OpenTelemetry trace data model](https://opentelemetry.io/docs/specs/otel/trace/api/) — the five attribute levels
- [Grafana TraceQL documentation](https://grafana.com/docs/tempo/latest/traceql/) and [TraceQL overview (Giant Swarm)](https://docs.giantswarm.io/overview/observability/data-management/data-exploration/traceql/) — scopes, intrinsics, and structural/metrics tiers
- [Braintrust BTQL](https://www.braintrust.dev/docs/reference/btql) — structured SQL-like query language (prior art)
