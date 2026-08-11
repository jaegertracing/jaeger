# RFC 0005: Structured Query Filters for Trace Search

- **Status:** Draft
- **Author:** Yuri Shkuro
- **Created:** 2026-06-19
- **Last Updated:** 2026-08-10

---

## Abstract

Jaeger's trace-search API filters spans by unqualified key-value tag pairs, implicitly ANDed. Each pair is matched against the span's attribute locations without distinguishing among them, and how many locations — and which — is backend-dependent. This RFC defines a **structured query-filter model** for trace search that (1) lets a predicate reference a specific attribute *level* (span / resource / instrumentation / event / link) or a built-in *field* (duration, name, status, …), (2) composes predicates with **boolean operators** (`AND`/`OR`/`NOT`), and (3) keeps the existing unqualified tag filter working unchanged.

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

When a user queries `http.status_code=500`, an unqualified backend must fan the query out across every attribute level it indexes with OR logic. In ClickHouse a key with no recorded metadata expands to five separate `arrayExists()` calls (three top-level columns, two nested within event/link arrays), each scanning a typed Map column; when the `attribute_metadata` view already knows the key, ClickHouse restricts the fan-out to the levels and types it was actually seen at (§8, Option D). In Elasticsearch each unqualified tag expands to a `bool.should` across the field locations, increasing sub-query count and reducing cache effectiveness. The cost is paid on every attribute of every search, even though the user almost always knows which level they mean.

### 1.4 The semantic problem

The unqualified API cannot express intent. "Find spans where the *span* has `deployment.environment=staging`" and "find spans whose *resource* has `deployment.environment=staging`" are different questions — the first finds spans explicitly tagged, the second finds spans emitted by services in staging — but today they are the same query. Nor can the API express `duration > 2s`, `http.status_code >= 400`, or `A OR B`: it supports only string equality, ANDed.

### 1.5 Two axes, not one

Level qualification alone is too narrow: attaching a level to each attribute leaves an API that still cannot express `OR` or `duration > 2s`. A complete answer must settle two independent axes:

- **What a predicate can reference** (the *leaf*): a level-qualified attribute, but also built-in span/trace *fields* (`duration`, `name`, `status`, …) that are not attributes at all, and an *operator* richer than equality.
- **How predicates combine** (the *composition*): equality-only conjunction is the floor; a boolean expression is the natural ceiling; aggregation and trace-tree navigation lie beyond.

This RFC designs both axes together (§3–§5) rather than adding the level qualifier alone.

### 1.6 The storage-backend landscape

Feasibility is dominated by how each backend physically stores and indexes attributes.

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
- **G2 — Built-in fields.** A predicate may target a built-in span/trace field (`duration`, `name`, `status`, `kind`, service, trace-level values) uniformly with attributes (§5).
- **G3 — Richer operators.** Beyond equality: `ne`, `gt`, `lt`, `gte`, `lte`, `regex`, `exists`, and set membership `in`/`not_in` — extensible without a second API redesign.
- **G4 — Boolean composition.** Predicates combine with `AND`/`OR`/`NOT` and nesting, including an existential quantifier (`some`) over the multi-valued event/link collections (§4 tier L2, §5.5).
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
- **Predicate anatomy (§5)** — *a single predicate's operands (level-qualified attributes, built-in fields, or constants), operator, and value typing.*

They are independent: the composition tier could be chosen with or without built-in fields, and vice versa. §6 combines the two into one proto/AST.

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
| **L1** | Level- or field-qualified predicates (a reference, operator, and value), still all-ANDed | — |
| **L2** | Boolean expression tree: `AND`/`OR`/`NOT` + nesting over L1 leaves, plus existential quantification (`some`) over the event/link collections | SQL `WHERE`, BTQL filter |
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

So the proposed filter API is the **L2 boolean expression tree** (§6). "L1" is retained only as a *capability tier* — the conjunctive subset that every backend, including the flat ones, supports. **L3 is deferred** (awkward against Jaeger's whole-trace result model, and inert until L4 exists). **L4 is excluded** (belongs to the metrics/SPM subsystem; a separate RFC). **L5 is excluded** — not for infeasibility (structural predicates can be evaluated post-fetch on any backend, assembling each candidate trace) but because it is a distinct fetch-then-filter execution model that cannot prune in storage, is inefficient at scale, and is a large surface; deferred as a separate effort. One nuance sits inside the backends, not the API: a pure conjunction admits a fast all-predicates-pushdown path while a tree with `OR` needs fuller evaluation.

**Why the excluded tiers are bounded, not dead ends.** This RFC's `Expression` is the *filter* layer — the per-span predicate. The deferred tiers (§4) extend it rather than replace it, which is why declining them now does not paint the design into a corner. L3 (projection) and L4 (aggregation/grouping) add sibling clauses over the same `Expression`: a projection is a list of expressions, a group key is an expression, an aggregate is a `Call`. L5 (structural / trace-tree queries) adds an *outer* layer — a query over relationships between sets of spans, whose per-set filter is an `Expression` — so structural queries would wrap this AST, not force a redesign of it. The only capabilities the current shape genuinely could not grow into are narrow: **set membership over a list** (already addressed via `in`/`not_in` + `List`, §6.1) and a parent-scope modifier (a flag over a level, orthogonal to `level` itself; it belongs with the deferred structural tier). Everything else is a pure addition to the open `op`/`type` vocabularies or the `Call` node, with no new message types: further operators (`!~`, arithmetic), aggregates, and semantic literal types (duration/status/kind). The fuller trace query languages surveyed in §4 occupy exactly these upper tiers.

---

## 5. Predicate anatomy — operands, operators, and value types

A predicate is a `Call` (§6.1): an **operator** (§5.3) applied to **operand** expressions. Each operand is either a *reference* — a value on the span or trace, identified by its level, name, and whether it is an attribute or a built-in field (§5.1–§5.2) — or a *constant* (a scalar, or a list for `in`/`not_in`). The operands are the same kind of thing, so neither side is privileged: the everyday `reference op constant` shape (`span.http.status_code = 500`) and a `reference op reference` shape (`span.a > span.b`) are equally expressible. A constant carries an optional **type** (§5.3–§5.4) telling the backend how to interpret it.

### 5.1 References: levels and the `attr` flag

A **reference** names a value to read off the span or trace. It has three parts: a **level** (the scope it lives in), a **name**, and an **`attr`** flag. At an explicit level, `attr: false` (the default) means a built-in field of that level (§5.2); `attr: true` means an entry in that level's attribute map, keyed by `name`. An **empty level** is always an attribute — the unqualified span-or-resource search — since a built-in field has no unqualified form. So the flag is only ever set to reach a level-qualified *attribute*; built-ins and unqualified attributes carry no flag. We call the qualifier **level**, not "scope", so it never overloads OTLP's `InstrumentationScope`.

The attribute-map levels follow the OTLP model (§1.2):

| `level` | Attribute map |
|---------|---------------|
| *(empty)* | span **or** resource attributes (default) |
| `span` | `Span.attributes` |
| `resource` | `resource.attributes` |
| `instrumentation` | `ScopeSpans.scope.attributes` (the `InstrumentationScope`) |
| `event` | `Span.events[].attributes` |
| `link` | `Span.links[].attributes` |

The `attr` flag disambiguates a built-in field from an attribute that happens to share its name: `{level:"span", name:"duration"}` is the span's duration (built-in, the default), while `{level:"span", name:"duration", attr:true}` is a span attribute *named* `duration`. A built-in field always names an explicit level; an empty level is therefore always an attribute. A reference at an explicit `event` or `link` level with an *empty* `name` is a third case: it denotes the whole collection, and is used only as the first operand of `some` (§5.5).

The empty-level default means span-or-resource attributes rather than "all five", a deliberate choice for the new `filter` model. Span and resource (process) attributes are the tags reliably indexed across every backend, so this default covers the high-value common case without paying to scan levels that are unindexed or costly. It is *not* a claim that span-or-resource matches today's unqualified behavior — that behavior is backend-dependent and generally searches *more*: ClickHouse ORs across all five levels (§1.3), and Elasticsearch across its indexed span/resource/event locations. The legacy `attributes` map keeps that existing behavior unchanged; the span-or-resource default applies only to an empty `level` in the new `filter` field. A backend that indexes or scans more simply returns a superset (§1.6). Further levels are future enhancements — a `trace` level for whole-trace fields (`traceDuration`, `rootName`), or `parent.` for the parent span's attributes. Neither is answerable today: no Jaeger backend stores a trace-level entity, so a whole-trace predicate needs the trace assembled first (§9). The level vocabulary is an open string set (§6), so they slot in later without a redesign.

### 5.2 Built-in fields

Much of what users filter on is not an attribute at all but a **built-in field** — a value the data model defines directly, not an attribute-map entry. A reference names one by giving its level and leaving `attr` unset; a built-in field is the default at an explicit level (§5.1). Built-in fields exist at every level, not just the span; the lists below are **representative, not exhaustive**:

| `level` | Built-in fields (representative) | Today in Jaeger's API |
|---------|-----------------|-----------------------|
| `span` | `duration`, `name`, `kind`, `status`, `startTime`, `spanID`, … | `duration_min`/`max`, `operation_name`, `span.kind` tag, `error=true` tag |
| `resource` | `service`, … | `service_name` field |
| `instrumentation` | `name`, `version` | not expressible |
| `event` | `name`, `timeSinceStart`, … | not expressible |
| `link` | `traceID`, `spanID` (the linked trace/span) | not expressible |

The value of folding these into references is *uniformity*: `span.duration > 2s`, `span.status = error`, and `span.http.method = GET` are all the same shape (a predicate over a reference), instead of three unrelated mechanisms (a dedicated duration field, a magic `error` tag, and a tag map). It also makes queries expressible that are impossible today (`event.name`, `link.traceID`, `span.startTime`). The dedicated top-level query fields (`service_name`, `operation_name`, the paired `duration_min`/`duration_max`) and the legacy `attributes` map remain supported for backward compatibility but are **mutually exclusive with `filter`** (§7): a legacy request uses them, and the query service normalizes them into built-in-field predicates internally, while a `filter` request expresses `service`, `name`, and `duration` as references directly. Either way a backend sees one filtering model rather than a growing mix of scalar fields *plus* `attributes` *plus* `filter`.

The built-in-field names are an **open, documented vocabulary per level** (like the levels themselves), not a closed set, and a backend advertises which it can serve. A first cut can support the span fields (`duration`, `name`, `status`, `kind`) and phase in the event/link ones (§9); whole-trace fields (`traceDuration`, `rootName`) wait on a future `trace` level (§5.1). The event- and link-level fields interact with correlated matching (§5.5), since a span has many of each.

### 5.3 Operators and value typing

The operator set is `eq`, `ne`, `gt`, `lt`, `gte`, `lte`, `regex`, `exists`, and set membership `in`/`not_in` (whose right operand is a `List`, §6.1). The negated leaf comparisons `ne` and `not_in` are kept distinct from a boolean `not` for two reasons: they map to a backend's native negated operators (`!=`, `NOT IN`) so they push down as leaf predicates, and they stay available on backends that reject boolean nesting (§7). `x not_in list` also reads predictably when the attribute is absent — it does not match, whereas `not(x in list)` matches every span that lacks the attribute entirely. A constant `value` is a string on the wire and carries an **optional `type`** (`string`, `int`, `double`, or `bool`) telling the backend how to interpret it (on the `Scalar`/`List` term, §6.1). Omit `type` and the backend resolves it as it does today, matching wherever the key actually lives (including a key recorded with more than one type). A specified `type` is *authoritative*: the backend routes to that typed storage and matches only there — it does **not** also probe the string form — so specifying narrows and skips the metadata lookup. §5.4 covers why "any type" is the default. Numeric operators (`gt`/`lt`/`gte`/`lte`) imply a numeric interpretation regardless. A backend that does not implement an operator rejects the predicate (§7) rather than guessing.

**Units of numeric values.** For a value with an implied unit — chiefly `duration` — the wire value should carry the unit *explicitly*, in Go duration syntax (`2s`, `1h30m`), matching today's `duration_min`/`duration_max` fields, rather than a bare number in an assumed unit (which is ambiguous — nanoseconds? milliseconds?). A bare-number value (e.g. a numeric attribute like `http.response.size`) is compared numerically and carries no RFC-defined unit: the caller and the stored data share whatever unit the attribute was recorded in, exactly as today.

### 5.4 Typed values

`type` is optional. Omitted, it means *any type*: the backend resolves the value wherever the key lives, across every observed type, exactly as today. Set, it is *authoritative*: the backend routes to that typed storage and matches only there, so specifying a type narrows the match and skips the metadata lookup. A query that declares `type=int` for a value stored as a string then matches nothing — the caller narrowing to the int-typed value, not a silent bug. Two facts force this "optional, authoritative when set" rule rather than a mandatory type.

**A key is legitimately multi-typed.** The same key appears with different types across services — `http.status_code` as an int from one service, a string from another — and ClickHouse's `attribute_metadata` records exactly that. Today's resolution searches all observed types and matches both. A single mandatory `type` could not express "any type" and would silently drop the others, so the forgiving any-type behavior must stay the default.

**Most backends cannot expose type metadata.** Typed authoring assistance needs a discovery API that returns each key's type(s). Only ClickHouse has one (`attribute_metadata`); ES/OS would need an expensive aggregation that does not even surface types, and the flat backends have no enumeration at all. So a mandatory type is undeliverable as good UX on most backends.

What each backend can do with `type` (🟢 native · 🟡 partial / costly · 🔴 not feasible):

| Capability | ClickHouse | Elasticsearch/OpenSearch | Cassandra / Badger |
|------------|:---:|:---:|:---:|
| typed predicate evaluation | 🟢 typed columns | 🟡 `eq` is a string term; numeric `gt`/`lt` needs the tag indexed numerically (a schema question) | 🔴 string `eq` only; no numeric range |
| typed discovery API | 🟢 `attribute_metadata` | 🟡 expensive aggregation; type not exposed | 🔴 no enumeration at all |

Three consequences follow:

- ClickHouse's `attribute_metadata` view (Option D, §8) is **not eliminated** — it resolves untyped predicates and feeds a future discovery API. A supplied type makes the lookup *avoidable*, not obsolete.
- The discovery API (§9) is realistically **ClickHouse-first**; other backends default to untyped.
- The flat backends ignore `type` (they store strings) and reject numeric operators (§7).

Typed queries therefore roll out immediately where the type is intrinsic — built-in fields (`duration`, `status`, `kind`) and string-`eq` attributes (today's default) — with typed predicates over arbitrary user attributes, and the discovery API, following ClickHouse-first.

### 5.5 Correlated matching over events and links

A span has one resource and one instrumentation scope, but *many* events and *many* links. So a filter that names two event fields — "an event whose `name` is `exception` **and** whose `timeSinceStart` is over 50us" — has two readings: the same event satisfies both (correlated), or one event is named `exception` and some *other* event is late (uncorrelated). The correlated reading is almost always what a user means, and a flat `and(event.name = "exception", event.timeSinceStart > 50us)` gives the uncorrelated one — each predicate matches *some* event independently.

Expressing the correlated reading needs a **quantifier that binds a single element** of the collection and evaluates a predicate against it. This is a standard construct — SQL `EXISTS (… WHERE …)`, MongoDB `$elemMatch`, Elasticsearch's `nested` query, ClickHouse's `arrayExists(e -> …, events)` — and it is the `some` operator:

```
some( <collection>, <predicate> )
```

- The first operand names the collection: a reference at `event` or `link` level with no `name` (the collection-reference case of §5.1).
- The second is a boolean predicate. **Inside it, references at the quantified level bind to the currently-bound element**; references at other levels (`span`, `resource`) bind to the enclosing span as usual. So `some(event, and(event.name = "exception", event.timeSinceStart > 50us))` reads "there is an event on this span whose name is `exception` and which fired more than 50us in" — one event satisfying both.

`some` yields a boolean, so it composes like any other predicate — AND it with span predicates, negate it, nest it. Outside a `some`, an event/link reference keeps its uncorrelated "any element" meaning, which is all a bare `exists(event.name)` needs. The universal quantifier (*every* element matches) is not a separate operator: `every` is `not(some(c, not(p)))` by De Morgan — including the correct vacuous truth on an empty collection — so `some` is the primitive and an `every`/`all` sugar can be added later if demand appears.

Correlated matching is a **declared capability** (ADR-013, §7): ClickHouse and Elasticsearch can evaluate `some` (via `arrayExists` and `nested` respectively); a backend that cannot declares it unsupported, and the query service refuses a filter containing `some` up front rather than silently returning the uncorrelated answer.

---

## 6. Proposed API

The two axes combine into one structured AST: a single, uniformly recursive **`Expression`**. An expression is either an *atom* — a reference (a level-qualified attribute or a built-in field, §5) or a constant (a scalar, or a homogeneous list for `in`/`not_in`) — or a *call* applying an operator or function to argument expressions. Boolean combination (`and`/`or`/`not`), comparison (`eq`/`gt`/…), set membership, and future arithmetic/aggregation are all the same `Call` node, so `a AND b`, `span.a > span.b`, and `(a + b) > c` compose uniformly, and the expression is the one reusable term a future projection, grouping, or named function (§4 L3/L4) would operate on. The AST deliberately does **not** encode value types: a filter is an expression that *type-checks* to boolean, and `duration > "x"` is a type error but a valid graph — validated separately, as expression ASTs conventionally are (§6.1). `level`, `op`, and the optional `type` (§5.4) are **typed string enumerations** (documented closed value sets) rather than proto enums — see §6.2 for why; the built-in-field names are an open documented vocabulary (§5.2).

### 6.1 Proto

```protobuf
// Expression is a node in the filter AST: either an atom — a reference or a
// constant (scalar or list) — or a Call applying an operator or function to
// argument Expressions. The tree is uniformly recursive: a call's
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
    Reference ref    = 1;  // a value on the span/trace: attribute or built-in field
    Scalar    scalar = 2;  // constant: single typed value
    List      list   = 3;  // constant: homogeneous list (right arg of in / not_in)
    Call      call   = 4;  // an operator/function applied to argument Expressions
  }
}

message Reference {
  string name  = 1;  // built-in field name, or attribute key when attr = true
  string level = 2;  // span|resource|instrumentation|event|link; empty = span-or-resource attribute
  bool   attr  = 3;  // true = an attribute of `level`; false (default) = a built-in field of `level` (§5.2)
}

message Scalar {
  string value = 1;
  string type  = 2;  // optional hint: string(default)|int|double|bool; empty = any type (§5.4)
}

message List {
  repeated string values = 1;
  string type = 2;          // optional hint applied to every element; empty = any type
}

// Call applies operator/function `op` to argument Expressions. (Named Call, not
// Operation, because api_v3 already has an `Operation` message — the span
// operation-name metadata returned by GetOperations.) Arity is implied by `op`:
// `not`/`exists` are unary; `and`/`or` take two or more args; the comparisons
// and `in`/`not_in` are binary ([left, right]). Because args are Expressions,
// `span.a > span.b` and `(a + b) > c` are expressible, not only `ref op scalar`;
// `in`/`not_in` take a List as the right arg. `some` (§5.5) is the existential
// quantifier over the event/link collections — args = [collection Reference,
// predicate], with the predicate's same-level references bound to one element.
// Named scalar functions and aggregates (avg, count, coalesce, …) are future
// `op` values needing no new message — a function is just a call.
message Call {
  string op = 1;              // and|or|not | eq|ne|gt|lt|gte|lte|regex|exists|in|not_in | some | (future: not_regex|every|add|sub|avg|count|…)
  repeated Expression args = 2;
}

message TraceQueryParameters {
  // Legacy: unqualified AND-equality over the tag map. Retained unchanged.
  map<string, string> attributes = 3;

  // Structured filter: a single boolean-valued Call (an operator over argument
  // Expressions), mutually exclusive with the legacy predicate fields
  // (service_name, operation_name, duration_min/max, attributes; §7). A
  // multi-predicate conjunction is an explicit `and` call.
  Call filter = 10;
}
```

The filter is a single `Call`, so there is exactly one way to express a conjunction — an `and` call — rather than a second, implicit one (a top-level list). It is typed `Call` rather than the more general `Expression` because a filter always applies an operator (a `Call`): the top level then carries no `Expression` oneof envelope (`{"op":…}` on the wire, not `{"call":{"op":…}}`), so the everyday single-predicate query is that much shorter. A sub-expression composed elsewhere is lifted into a filter by wrapping it (`Expr(call)` in a typed builder), so nothing is lost. `or`/`not` at the top read directly, matching how the prior-art structured query languages (§4) carry their filter. The `and` wrapper for the common multi-predicate case is emitted by the builder (§6.3) or shorthand (§7), never hand-written. (A top-level implicit-AND list was the alternative; see §8.)

### 6.2 REST/JSON encoding, and why string enumerations

Jaeger's api_v3 HTTP endpoint serializes with gogo/protobuf `jsonpb` at its defaults, so a proto *enum* would cross the wire as its full `CONSTANT_CASE` name (`"level":"ATTRIBUTE_LEVEL_SPAN"`) with no short-alias option, and proto3 enums are *open* (an unknown number is accepted, not rejected). Plain `string` fields avoid the verbosity, and the value set is still declared in the generated OpenAPI schema via the gnostic `enum` annotation, which validates it for generated clients and request validators (stricter there than an open proto enum). The closure is a schema-layer guarantee, not a proto one: the field stays a plain `string`, so at runtime an unknown `level`/`op` is caught by the backend rejecting it as *unsupported* (§7), not by the type system. That is deliberate — it is exactly what lets a backend treat an unrecognized value as "unsupported" rather than fail a type check.

```yaml
level: { type: string, enum: [span, resource, instrumentation, event, link] }               # Reference.level
op:    { type: string, enum: [and, or, not, eq, ne, gt, lt, gte, lte, regex, exists, in, not_in, some] }  # Call.op
type:  { type: string, enum: [string, int, double, bool] }                                   # Scalar.type / List.type; optional, empty = any type
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

The recursive `Call` shape makes the raw JSON verbose — each call carries an `args` array whose entries name their kind (`ref`/`scalar`/`list`/`call`). That verbosity is the deliberate cost of one uniform node that expresses `ref op ref` and keeps future L3/L4 in reach; humans are not expected to author it by hand — the §7 prefix shorthand does that. Spelled out, `http.status_code = 500` and `span.duration > 2s AND http.status_code in [500,503]` are:

```
GET /api/v3/traces?query.filter={"op":"eq","args":[{"ref":{"name":"http.status_code"}},{"scalar":{"value":"500"}}]}
```
```json
{ "query": { "filter": {
  "op": "and", "args": [
    { "call": { "op": "gt", "args": [
        { "ref": { "name": "duration", "level": "span" } },
        { "scalar": { "value": "2s" } } ] } },
    { "call": { "op": "in", "args": [
        { "ref": { "name": "http.status_code" } },
        { "list": { "values": ["500", "503"] } } ] } } ] } } }
```

The `filter` itself is a `Call`, written bare (`{"op":…}`); its `args` are `Expression`s, so a nested call carries the `{"call":…}` envelope. A single predicate is the filter directly (the first example); a conjunction is an `and` call over its predicates (the second). Note that nothing here carries a flag: `span.duration` is a built-in field (the default at an explicit level) and `http.status_code` is an unqualified attribute (empty level). The membership test is a single `in` call over a list, and `or`/`not` nest the same way as `and`.

The `attr` flag appears only when you qualify an *attribute* by level — "spans whose end-user id differs between the span and its resource":

```json
{ "op": "ne", "args": [
  { "ref": { "name": "enduser.id", "level": "span", "attr": true } },
  { "ref": { "name": "enduser.id", "level": "resource", "attr": true } } ] }
```

And the correlated event query of §5.5 — an event named `exception` that fired more than 50us into the span — is a `some` over the `event` collection whose predicate's event-level references bind to one event:

```json
{ "op": "some", "args": [
  { "ref": { "level": "event" } },
  { "call": { "op": "and", "args": [
    { "call": { "op": "eq", "args": [
        { "ref": { "name": "name", "level": "event" } },
        { "scalar": { "value": "exception" } } ] } },
    { "call": { "op": "gt", "args": [
        { "ref": { "name": "timeSinceStart", "level": "event" } },
        { "scalar": { "value": "50us" } } ] } } ] } } ] }
```

### 6.3 Programmatic construction — a fluent builder

The verbose AST is comfortable for machines to *transport* but unpleasant to *assemble by hand*: a client SDK or automation that composes queries programmatically (as opposed to a human typing into a search box) should not be hand-building nested `call`/`args` dictionaries. The recommended ergonomics is a thin **fluent builder** in each client language that emits the §6.1 AST. It is the programmatic counterpart to the §7 prefix shorthand (the human on-ramp): a convenience layer over the same contract, not a second contract — a Go or TypeScript builder would compile to the identical AST. The builder is not a bespoke DSL: it follows the operator-overloading idiom well established across the Python ecosystem — SQLAlchemy, pandas, Django's `Q`, elasticsearch-dsl — so it reads as familiar to anyone who has composed queries in those libraries. A Python sketch:

```python
from jaeger.query import span, resource, event, link, attr, Query

# References — each level is callable for attributes and exposes its built-in
# fields as members, so one object covers both `attr` cases of a Reference
span("http.status_code")          # attribute at the span level        (attr:true)
span.duration                     # built-in field of the span          (attr:false)
resource("deployment.environment")# attribute at the resource level     (attr:true)
resource.service                  # built-in field of the resource      (attr:false)
event.name                        # built-in field of an event          (attr:false)
attr("k8s.pod.name")              # unqualified attribute (span-or-resource)

# Predicates — Python comparison operators build a `call`
span.duration > "2s"                                  # gt
span("http.status_code") == 500                       # eq
span("http.method").matches("GET|POST")               # regex
resource.service.one_of(["cart", "checkout"])         # in   (also .not_one_of / .exists)
span("a") > span("b")                                 # attribute vs attribute

# Composition — &, |, ~  (or and_(...), or_(...), not_(...))
flt = (span.duration > "2s") & span("http.status_code").one_of([500, 503])
flt = flt | ~resource.service.eq("healthcheck")

# Terminal — multiple .where() calls are ANDed into one Expression
q = (Query()
     .where(span.duration > "2s")
     .where(span("http.status_code").one_of([500, 503]))
     .build())          # -> TraceQueryParameters.filter (an `and` of the two)
```

Each fragment lowers directly to the AST — `span.duration > "2s"` produces `{"call":{"op":"gt","args":[{"ref":{"name":"duration","level":"span"}},{"scalar":{"value":"2s"}}]}}` (`span.x` emits a built-in-field `ref` — level set, no `attr` flag; `span(...)`/`resource(...)` emit level-qualified attribute `ref`s with `attr:true`; `attr(...)` emits an unqualified attribute). Two builder conveniences carry their weight:

- **Type-hint inference.** A specified `type` is authoritative (§5.4), so the builder sets it only where a numeric interpretation is required — a comparison like `attr("size") > 500` emits an `int`-typed `scalar` — and leaves equality and membership untyped, so `== 500` and `one_of([500,503])` match whatever form is stored (any-type resolution). The caller passes an explicit type to narrow an equality to one type.
- **Operator mapping.** `== != > < >= <=` map to `eq/ne/gt/lt/gte/lte`; `& | ~` to `and/or/not`; and the operators Python cannot overload get method forms — `.matches()` (regex), `.exists()`, `.one_of()`/`.not_one_of()` (in/not_in), since `x in [...]` and `and`/`or` keywords cannot be intercepted. Method aliases (`.eq()`, `.gt()`, …) exist for callers who prefer them or want to avoid overloading `==` (which, as in SQLAlchemy/pandas, returns a query fragment, not a bool).

This is illustrative, not normative: the wire contract is the AST (§6.1), and each SDK is free to shape its builder idiomatically as long as it emits that AST.

---

## 7. Backward compatibility and degradation

**Coexistence.** The legacy predicate fields keep their exact semantics, and `filter` is a new additive field that defaults to empty, so old clients are byte-for-byte unaffected. The canonical new query is **a time range plus `filter`**. The legacy predicate fields — `service_name`, `operation_name`, `duration_min`/`duration_max`, and the `attributes` map — are **mutually exclusive with `filter`**: a request is either legacy-style or filter-style, and setting a legacy predicate field alongside `filter` is rejected (`InvalidArgument`). (`start_time_min`/`start_time_max`, `search_depth`, and `raw_traces` are the envelope; they are not predicates and are always allowed.) The query service builds one effective filter per request — from `filter`, or by normalizing the legacy fields — so a backend only ever sees a single filter model. This holds at all layers: public api_v3, internal storage API, and the remote-storage gRPC protocol.

**Normalizing legacy query parameters into `filter` (proposed architectural decision).** Each legacy predicate field maps to a built-in-field reference (§5.2) — `service_name` → `service`, `operation_name` → `name`, `duration_min`/`duration_max` → a pair of `duration` range predicates — and `attributes` is a set of unqualified equality predicates. For a **legacy request** (no `filter`), the query service **normalizes those fields into a single `filter` expression** before dispatching, so every backend implements exactly one filtering model (the AST). A **filter request** expresses the same things as references itself, which is why the two are mutually exclusive rather than merged. (The inclusive duration bounds use `gte`/`lte`, part of the operator set — §5.3.)

This is clean for the **internal `TraceReader`** API, which is versioned with the binary and can simply drop the redundant scalar fields once the query service populates `filter`. It is harder at the **Remote Storage gRPC API**: those scalar fields are part of the published `storage.v2` contract and existing third-party plugins read them.

A plain additive `filter` field on the existing `FindTraces`/`FindTraceIDs` RPCs would be a *silent* gap at the remote boundary: a plugin that predates `filter` ignores the unknown field and answers from the scalar fields alone, under-filtering with no signal. What closes the gap is a **separate capability declaration that gates the field** — the mechanism in [ADR-013](../adr/013-storage-capability-declaration.md). A backend declares what it can search through `SearchCapabilities` on `tracestore.Reader`, carried across the remote boundary by the `jaeger.storage.v2.Capabilities` service ([jaeger-idl#211](https://github.com/jaegertracing/jaeger-idl/pull/211), merged). RFC 0005's filter capabilities — which levels, which operators, whether boolean `or`/`not` is honored — are declared there as sibling fields alongside the `WithoutServiceName` field ADR-013 shipped first.

So the query service asks before it dispatches: it sends the rich `filter` only to a backend that declares support, and **down-converts** to the legacy scalar fields + `attributes` (rejecting what those cannot express, e.g. `or`/`not`) for one that does not. A plugin predating the declaration reads as least capable — its `Capabilities` service is absent, which maps `UNIMPLEMENTED` → `ErrUnsupported` → least capable per ADR-013 — so the query service never sends it `filter` and the under-filtering case cannot arise. No bespoke filter-aware RPC is needed. The internal `TraceReader` cleanup — dropping the redundant scalar fields once the query service populates `filter` — can proceed independently. (Heavier fallbacks — mirroring the scalars alongside `filter`, or a whole-protocol major bump — apply only if the capability-declaration route is rejected.)

**Capability-based degradation.** The backend-wide limits are *declared* through `SearchCapabilities` (ADR-013), so the query service refuses an unserviceable filter before it dispatches and the UI builder (M7) grays out what a backend cannot serve; a per-query predicate that no declared capability covers is *rejected* at query time as the backstop. Either way a backend never silently returns wrong results:

- **Levels** — ClickHouse honors all five. ES/OS honor span/resource/event today; instrumentation and link await schema evolution. The flat backends honor only the levels their write path indexes — span/resource/event — because instrumentation-scope attributes are merged into span tags and **link attributes are not stored at all** (§1.6). The honored level set is declared, so a predicate naming an unsupported level is refused — not widened, since widening would be a superset only for indexed levels and plain wrong for link.
- **Operators** — the implemented operator set is declared; a backend that has not implemented `regex`/`gt`/… does not advertise it, and the query service refuses such a predicate rather than letting the backend approximate.
- **Boolean structure** — ClickHouse and ES/OS declare full boolean support; the flat backends declare conjunction-only, so an `or`/`not` call is refused up front while their conjunctive subset still runs.
- **Remote-storage plugins** — a plugin that declares no filter capability (or predates the `Capabilities` service, and so reads as least capable) receives only the legacy `attributes` and behaves exactly as today; the query service populates `attributes` from a purely-conjunctive, unqualified `filter` for it.

**Prefix syntax as the human on-ramp.** The verbose structured form is machine-first. For humans (the UI text box, `curl`), the query parser accepts a prefix shorthand that normalizes into the structured expression — `resource.deployment.environment:staging` → an `eq` call over `ref{name:"deployment.environment",level:"resource",attr:true}` and `scalar{"staging"}`; `duration>2s` → a `gt` call over `ref{name:"duration",level:"span"}` and `scalar{"2s"}`. This is a convenience layer over the same AST, not a second contract, and it means the UI need never emit the verbose operand JSON by hand.

---

## 8. Considered alternatives

The structured model of §4–§6 is option C. Three lettered alternatives (A, B, D) were considered and not adopted, along with a free-text surface:

- **A — change the default level of the existing `attributes` field** (a `search_all_attribute_scopes` boolean). *Rejected.* It silently changes the semantics of an existing field (a migration flag-day), offers only binary "span+resource vs all" precision, and extends to neither operators nor boolean composition. A dead end.
- **B — encode the level as a key prefix** (`resource.k8s.namespace.name`). *Not a competing data model — adopted as text sugar* (§7). As an API contract it is rejected: the convention is implicit and unvalidated, collides with user keys that happen to start with a level name, and cannot express operators or booleans.
- **D — backend metadata level-skipping** (ClickHouse consults its `attribute_metadata` view to skip levels a key was never seen at). *Orthogonal, and already implemented.* A ClickHouse-local optimization needing no API change; the typed `filter` makes its lookup avoidable when a type is supplied (§5.4) but neither depends on nor replaces it.
- **A free-text query language** (parse a TraceQL/BTQL/SQL string). *Non-goal* (§2). Jaeger commits to a structured AST; a text surface, if ever desired, can compile to this same AST without changing the contract.

**AST node-shape decisions.** Four shape choices within the structured model, each with the alternative it was chosen over:

- **One `Reference{name, level, attr}`, not separate `attribute`/`field` variants.** A built-in field and an attribute are both "a value read off the span/trace," so they are one node parameterized by level, with an `attr` flag distinguishing a level-qualified attribute from the built-in field that is the default at an explicit level. *Rejected:* separate oneof variants — the split saves no evaluator branch (a field and a map entry resolve differently regardless), and a bare field-name string cannot carry a level, which is required once built-in fields exist at non-span levels (`event.name`, `link.traceID`; §5.2). Unifying is what makes those expressible at all. The default is built-in rather than attribute so the common built-in-heavy query carries no flags; only a level-qualified attribute sets `attr:true` (§5.1).
- **`in`/`not_in` take a `List` operand, not variadic scalar args.** The set is a first-class `List` literal (one `type` for the homogeneous list), so `in`/`not_in` stay binary `[subject, set]` like every other operator. *Rejected:* `Call(op="in", args=[subject, s1, s2, …])` — a variadic form invents a "first arg is the subject, the rest are the set" convention unique to `in`, lets a `ref`/`call` slip into set positions, and carries a `type` per element (admitting a heterogeneous set validation must then reject). The concern that a first-class `List` enables nonsensical ASTs is closed by `filter: Call` (a list cannot be the top-level filter) and by validation catching a list in a scalar position, the same class as any other type error.
- **Top-level `filter` is a `Call`, not an `Expression` (nor an implicit-AND list).** A filter always applies a boolean operator, so the field is a `Call`; the top level then carries no `Expression` oneof envelope — `{"op":…}` on the wire, not `{"call":{…}}` — so the common single-predicate query is shorter, and a single node gives one canonical encoding of conjunction (an `and` call) rather than a second, implicit one (a top-level list). *Rejected:* `Expression filter` — the composability it appeared to buy (a filter being the same type as any sub-expression) is a host-language concern, met by a one-line `Expr(call)` wrap in a typed builder, not something the wire must carry, so the constant envelope on every request buys nothing. A top-level implicit-AND list was also rejected: it is a second way to spell AND, and forces a one-element list for a top-level `or`.
- **Scalars carry a string `value` + optional `type` hint, not a typed `oneof`** (§5.4). A typed `oneof {int64|double|bool|string}` cannot express the default the tracing data model needs — *match any type* — because a key is legitimately multi-typed across services (`http.status_code` int in one, string in another) and the caller often has no type metadata with which to choose a variant (§5.4). Unit-bearing values (`duration` = `"2s"`, future timestamps) have no native proto scalar and revert to strings regardless. The stringify "tax" for a known-typed caller is paid once by the builder, which infers the type from the native value (§6.3); wire packing is immaterial at query-payload sizes. *Rejected:* a typed `oneof` — its strictness is illusory here, since it cannot represent "any type."

---

## 9. Implementation roadmap

PR-sized milestones with explicit exit bars, grouped into stages. The API is L2 from the start; capable backends (ClickHouse, ES/OS) evaluate the full tree, while the flat backends support only its conjunctive subset. The cross-backend API contract is where the coordination cost lives.

**Stage A — API foundation (additive, no behavior change)**

- **M1 — Proto types in jaeger-idl.** Add `Expression`, `Reference`, `Scalar`, `List`, and `Call` (with `level`/`op`/`type` as string enumerations whose closed sets are declared in the OpenAPI schema — §6.2) and the `filter` field on `TraceQueryParameters`, in both the public api_v3 and the storage/v2 protos. Legacy `attributes` untouched. *Initial delivery may ship the `ref` and `scalar` terms with span-level attributes and built-in fields, and phase in the `list` term, the non-span levels, and the `some` quantifier (§5.5), since the oneof and the `op` vocabulary are additive.* **In flight — [jaeger-idl#206](https://github.com/jaegertracing/jaeger-idl/pull/206), which encodes the recursive `Expression` + `Call` AST (the `ref`/`scalar`/`list`/`call` terms and the `level`/`op`/`type` string enumerations) per §6.1–§6.2.** *Exit:* generated types compile and vendor cleanly; existing api_v3 callers byte-for-byte unaffected.
- **M2 — Plumb the filter through the query service to the storage interface.** Extend the internal `TraceQueryParams` ([`reader.go`](../../internal/storage/v2/api/tracestore/reader.go)) to carry the expression tree alongside the legacy `Attributes` map, and translate the proto field in the api_v3 handler. With no backend routing yet, a purely-conjunctive tree is treated as unqualified search-all (today's results); non-conjunctive trees and unsupported operators are refused at the edge — up front where the query service can read the backend's declared filter capabilities (`SearchCapabilities`, [ADR-013](../adr/013-storage-capability-declaration.md)), at query time otherwise. *Exit:* a conjunctive level-qualified filter reaches every backend as unqualified attributes and returns today's results; `OR`/`NOT` and unsupported operators are refused; plugins ignoring `filter` are unaffected.

**Stage B — Backend routing (one PR per backend, parallelizable after M2)**

- **M3 — ClickHouse.** Route level-qualified predicates to their typed Map column ([`query_builder.go`](../../internal/storage/v2/clickhouse/tracestore/query_builder.go)) and lower the boolean tree into the SQL `WHERE` (`AND`/`OR`/`NOT`); an empty level keeps the span-or-resource expansion. *Exit:* level-qualified/boolean queries emit the corresponding SQL; unqualified queries byte-identical to today.
- **M4 — Elasticsearch/OpenSearch.** Route span/resource/event levels to their fields in `buildTagQuery` ([`core/reader.go`](../../internal/storage/v2/elasticsearch/tracestore/core/reader.go)) and lower the boolean tree into a `bool` query; the instrumentation and link levels are rejected pending schema evolution. *Exit:* span/resource/event level-qualification and `AND`/`OR`/`NOT` work; unqualified snapshots byte-identical.
- **M5 — Cassandra + Badger (capability boundary).** Accept the conjunctive subset over indexed levels (span/resource/event); **declare** the honored level set, operator set, and conjunction-only boolean support as `SearchCapabilities` fields ([ADR-013](../adr/013-storage-capability-declaration.md)) so the query service refuses `OR`/`NOT`, unsupported operators, and predicates naming an unindexed level (link, instrumentation) up front — never silently widen (§1.6). *Exit:* supported predicates return correct supersets; unsupported ones are refused cleanly; a cross-backend conformance test asserts both.

**Stage C — Ergonomics and UI**

- **M6 — Prefix/shorthand parser** (§7) — normalize `resource.k:v`, `duration>2s`, etc. into the AST in the api_v3 HTTP parser ([`query_parser.go`](../../cmd/jaeger/internal/extension/jaegerquery/internal/apiv3/query_parser.go)). *Exit:* shorthand reaches storage as the structured predicate; unprefixed keys unchanged.
- **M7 — UI builder** — a filter builder emitting `filter`, starting with the conjunctive subset (chips with a level/field selector) and adding nested groups later; the legacy text box keeps populating `attributes`. **Type foundation in flight — [jaeger-ui#4371](https://github.com/jaegertracing/jaeger-ui/pull/4371) regenerates the api_v3 zod client with the filter-AST schemas (`Expression`/`Reference`/`Scalar`/`List`/`Call`) the builder emits against; the generator reproduces the recursive AST and string enumerations with no manual fixups. Draft pending M1.** *Exit:* existing search unaffected; qualified predicates emit `filter`.

**Out of scope (future, this model enables):**
- A `trace` level and its whole-trace built-in fields (`traceDuration`/`rootName`/`rootService`) — no Jaeger backend stores a trace-level entity, so these need the trace assembled and are left to a future enhancement (§5.1–§5.2). Also IDs, and built-in fields beyond the initial span set.
- Levels beyond the OTLP five (e.g. `parent.`, the parent span's attributes) — §5.1.
- ES/OS schema evolution to index instrumentation and link attributes distinctly (§1.6) — unblocks those levels in M4.
- A discovery API returning keys, their type(s), and sample values per level — the piece that feeds typed predicates and autocomplete (§5.4); ClickHouse-first.
- Tiers L3–L5 (§4): result shaping, aggregation/metrics (metrics subsystem), and structural/trace-tree queries (post-fetch only — not push-down-able, so inefficient at scale).

---

## 10. Open questions

1. **Conjunction semantics across spans.** Must `resource.service=foo AND span.http.status_code=500` match the *same* span, or may they match different spans of the same trace? (The internal `TraceReader.FindTraces` contract currently leaves this implementation-dependent.)
2. **Built-in-field phasing.** Which built-in fields are required in the first implementation (span `duration`/`name`/`status`/`kind`) versus deferred (event/link fields, IDs; whole-trace fields are out of scope, §9)? And which levels' correlated matching (§5.5) ships first?
3. **Prefix collision escape hatch.** Does the shorthand (§7) need an escape for user keys that literally begin with a level name, or is the structured JSON form the sufficient unambiguous alternative?

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
