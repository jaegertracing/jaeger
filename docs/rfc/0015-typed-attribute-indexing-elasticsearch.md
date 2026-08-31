# RFC 0015: Typed Attribute Indexing for Elasticsearch/OpenSearch

- **Status:** Draft — Option A is implemented behind an alpha feature gate
- **Author:** Yuri Shkuro
- **Created:** 2026-08-12
- **Last Updated:** 2026-08-24
- **Related:** [RFC 0005](./0005-structured-query-filters.md) (structured query filters — the caller that needs this), [ADR-012](../adr/012-unified-elasticsearch-client.md) (unified Elasticsearch client), [ADR-013](../adr/013-storage-capability-declaration.md) (storage capability declaration)

---

## Delivery

- ✅ **The mapping and the query path.** Delivered in [#9321](https://github.com/jaegertracing/jaeger/pull/9321) behind the alpha feature gate `jaeger.es.typedAttributeIndexing`, off by default. The gate adds the `number` sub-field to the span, resource and event attribute values in [`jaeger-span.json`](../../internal/storage/elasticsearch/esclient/index_templates/jaeger-span.json), in both the nested and the elevated representation, and lowers `gt`/`lt`/`gte`/`lte` on an attribute onto it in [`core/filter.go`](../../internal/storage/v2/elasticsearch/tracestore/core/filter.go). With the gate off both halves behave exactly as before, and the ordering predicate is refused rather than answered. §7 records what the implementation measured, including the two answers that changed the design.

Two things named in this RFC are deliberately not delivered. `long` is not mapped separately from `double` (§7, question 2), and the boolean sub-field §4 proposed is not mapped at all, because it cannot be installed on both engines (§7, question 7). Typed constants on attributes — RFC 0005 §5.4's authoritative typed matching, which §1.1 gives as the second of the two refusals — remain refused; the sub-field makes them answerable, but routing a declared type to it is a separate change.

---

## Abstract

Jaeger's Elasticsearch/OpenSearch schema indexes every span attribute value as a `keyword`, whatever type the attribute actually had. A user can therefore ask whether an attribute *equals* something, but not whether it is *greater than* something: a range query over a keyword compares lexicographically, which makes `"9" > "10"` true. The same mapping is why a caller cannot ask for the *integer* `http.status_code` as distinct from a string that happens to look like one.

This RFC proposes to index attribute values at their own type in addition to as keywords, so that ordered and type-qualified predicates become answerable. The input is OTLP and OTLP values are typed, and the default representation records that type beside each value — so the gap is in the **index mapping**, not in the data. The proposal is therefore a mapping change, applied to both places Jaeger puts attributes, with no change to the write path and no change to the documents Jaeger stores.

---

## 1. Motivation

### 1.1 The concrete gap

[RFC 0005](./0005-structured-query-filters.md) defines a structured query-filter model whose operator set includes `gt`, `lt`, `gte`, and `lte`, and whose constants may declare a `type` that the backend is required to treat as authoritative (RFC 0005 §5.3–§5.4). Its milestone M4 implemented that model for Elasticsearch/OpenSearch and had to refuse two parts of it:

- **Ordering reaches only the span duration.** `duration` is mapped `long`, so `span.duration > 2s` lowers to a range query. Every attribute is mapped `keyword`, so `http.response.size > 500` is refused rather than answered lexicographically.
- **A constant that declares its type is refused outright.** RFC 0005 §5.4 makes a declared type authoritative — the backend must match only values stored at that type — and there is no typed storage to route such a predicate to.

**The untyped predicate is the common case, not the exception.** RFC 0005 puts a constant on the wire as a *string* with an optional type, where omitting the type means *any type*, and its own builder sets a type only where a numeric reading is required and leaves equality and membership untyped (§5.3–§5.4, §6.3). So the backend is usually handed `"500"` with no declaration and has to decide what it means. The interpretation rule already exists — a numeric operator implies a numeric reading whatever the constant looks like — and the reason it cannot be applied is that there is nowhere numeric to apply it. This is why the fix is an indexing change and not an API change, and why the recorded type discriminator matters even when the caller declares nothing: it is what separates a stored number from a stored string that looks like one.

Both refusals are correct under the current schema: RFC 0005's degradation contract (§7) requires a backend to reject a predicate it cannot honor rather than approximate it, precisely so that a caller never reads a narrower or wrong answer as the whole one. They are also both the *same* limitation, which is why one RFC addresses them together.

### 1.2 Why this is worth fixing rather than living with

Numeric comparison is the most-requested predicate that Jaeger's tag search has never supported. Latency is served by the dedicated duration field, but every other magnitude a user cares about is an attribute: response sizes, queue depths, retry counts, row counts, token counts, and status codes. `http.status_code >= 400` is the canonical example — today it must be spelled as an enumeration of every code the user can think of, which is both awkward and quietly incomplete.

The limitation is also *asymmetric across backends*, which is the more corrosive problem. ClickHouse stores attributes in typed Map columns and answers ordered predicates natively, so the same structured filter succeeds on one Jaeger deployment and is refused on another. RFC 0005 anticipated that asymmetry and built the capability declaration to make it honest rather than silent, but honesty is the floor, not the goal.

### 1.3 What this RFC is not

This is not a proposal to redesign the span schema, to migrate existing indices, or to change what Jaeger writes. It is deliberately scoped to *how the values Jaeger already writes are indexed*, because that scope is what makes the change cheap and reversible.

---

## 2. How attributes are stored today

**The type is never in question on the write path.** The input is OTLP, whose attribute value is a typed union, so every attribute arrives already carrying its type and nothing has to infer or recover it. The only questions are which representation carries that type into the stored document, and whether the index can use it. Both answers differ per representation, which is what the options in §4 turn on.

Four facts follow, three of them easy to assume wrongly.

**The stored value keeps its native type.** A numeric attribute is a JSON *number* in the document and a boolean is a JSON *boolean*; only bytes, arrays, and key-value lists are stringified. So nothing on the write path flattens a number into a string. The flattening is the mapping's doing, at index time, because the mapping says `keyword`.

**The default representation also records the type explicitly.** Attributes are stored as an array of nested objects with `key`, `value`, and `type`, where `type` names the OTLP variant (`string`, `bool`, `int64`, `float64`, `binary`). So everything a type-qualified predicate needs in order to route is already in every document; it is simply not indexed in a form a range query can use, and `value` is mapped `keyword` alongside it.

**A second representation is chosen per attribute, and it drops the type.** The `tags_as_fields` setting *elevates* selected attributes out of the nested array into a flat object (`tag.<key>`, with dots replaced), for better Kibana support. Selection is all attributes, an explicit `include` list, or a list from a file, and a binary attribute is never elevated. Elevation *moves* rather than copies, so a given attribute is in one representation or the other, never both, and a single span routinely carries attributes in both at once.

The flat object has nowhere to put a discriminator, and what that costs is not precision but *what gets typed*. A discriminator beside each value makes the type a property of **that value**. A mapped field's type is a property of the **key**, for the whole index. The two are not interchangeable, because the same attribute key legitimately arrives with different types from different instrumentation — RFC 0005 §5.4 treats that as the normal case, which is exactly why its constant type is optional and means *any type* when omitted. A per-value type represents a multi-typed key faithfully. A per-key type has to pick a winner, and §4's Option C is where that bites.

(An elevated numeric value does lose the integer-versus-double distinction, since JSON has one number type. That costs nothing for the operators in question: a range query behaves the same either way, and string-versus-number-versus-boolean, the distinction that decides whether a predicate is answerable at all, is preserved.)

**The object representation is forced to `keyword` by a dynamic template.** The index template maps `tag.*`, `process.tag.*`, and `scopeTag.*` to `keyword` with `ignore_above: 256`. Without it, Elasticsearch's default dynamic mapping would infer `long`, `double`, or `boolean` from the value it receives. So the template is what discards the type — but read the other way, it is also what makes a multi-typed key *safe*: every value of every key is a keyword, so no key can conflict with itself and no document can be rejected for one. Today's schema trades typed predicates for conflict immunity, and Option C proposes the reverse trade.

One further property matters for the risk analysis: **a mapping change does not alter `_source`.** Elasticsearch stores the document as submitted and applies the mapping only to build the index, so changing how a value is indexed cannot change what the read path deserializes. Every option below is therefore invisible to span reconstruction and to round-tripping.

---

## 3. Requirements

The criteria used to score the options, in the order they matter:

- **R1 — Ordering on arbitrary attributes.** `gt`/`lt`/`gte`/`lte` must work on an attribute the operator has never heard of, not only on a curated list of well-known keys.
- **R2 — Authoritative typed matching.** A predicate declaring `type: "int"` must match only integer-typed values, as RFC 0005 §5.4 requires. Partial honoring is worse than refusal, because it returns a superset the caller believes is exact.
- **R3 — Query cost.** The predicate must prune at the index, not scan candidate documents. A correct answer that scales linearly with the time range is not usable on the search path.
- **R4 — Mapping stability.** The change must not risk rejected documents or unbounded growth in the number of mapped fields. Dropping spans to serve a query is not a trade Jaeger can make.
- **R5 — No write-path change.** Anything that changes what Jaeger writes needs a compatibility story for readers and a migration for existing data. Avoiding that is most of why this change can be cheap.
- **R6 — One implementation for both engines.** Elasticsearch and OpenSearch have diverged; a solution needing two query builders costs roughly twice as much to build and maintain.
- **R7 — Covers both representations.** The nested and object representations are both in production use, and elevation is chosen per attribute (§2), so a solution addressing only one leaves not just a configuration behind but part of a single span's attributes.
- **R8 — Works on already-written indices.** Desirable, not required — Jaeger rotates span indices daily by default, so a mapping change reaches most deployments within a day without any migration.

R8 is listed last deliberately. Daily rotation makes it far less important here than it would be for a single monolithic index, and treating it as a hard requirement is what pushes a design toward query-time evaluation and away from indexing.

---

## 4. Options

**Which representation each option addresses.** Attributes live in two places (§2), so every option has to be read against both, and they do not all reach both. Option A is written below against the nested `value` and then extended to the elevated fields, where it turns out to work. Option B replaces the nested arrays and by construction says nothing about elevated fields. Option C is only about elevated fields. Option D reads the stored document, so it is indifferent to representation. Option E maps chosen keys by name and applies to either. R7 is what scores this, and it is the axis on which the recommendation turns out to hinge.

### Option A — Typed sub-fields on the nested value

Today the nested attribute array maps all three of its fields as `keyword`:

```json
"tags": {
  "type": "nested",
  "dynamic": false,
  "properties": {
    "key":   { "type": "keyword", "ignore_above": 256 },
    "value": { "type": "keyword", "ignore_above": 256 },
    "type":  { "type": "keyword", "ignore_above": 256 }
  }
}
```

Two settings there are worth spelling out, because the proposal keeps both and neither is its idea. **`keyword`** indexes the value as a single exact, un-analyzed term — no tokenizing, no case folding — which is what makes `eq` work and is precisely why a range over it compares lexicographically (§1). **`ignore_above: 256`** means a value longer than 256 characters is not *indexed*: it is still stored, still returned with the span, and simply cannot be matched. It bounds index growth from long attribute values, and it appears on every keyword field in Jaeger's template today.

**The proposal** is to extend the mapping of the `value` property in this nested array with additional indexed fields (no change to `_source`):

```json
"value": {
  "type": "keyword", "ignore_above": 256,
  "fields": {
    "number": { "type": "double", "coerce": false, "ignore_malformed": true }
  }
}
```

`fields` declares a **multi-field**: Elasticsearch indexes the *same* source value again under it, with a different mapping. So `tags.value` keeps its exact-term behavior unchanged and gains `tags.value.number` beside it. `ignore_malformed: true` is what makes that safe — a value that will not fit the sub-field's type is skipped *for that sub-field* rather than rejecting the document. The sub-field is named `number`, not `double`, because integer and floating-point attributes both land in it; `double` is merely the Elasticsearch type wide enough to hold them (§7, question 2).

**`coerce: false` is what makes the sub-field authoritative.** Coercion defaults to on, and it converts a numeric *string* into a numeric field, so by default a string-typed attribute whose value looks like a number is indexed as one and a range query cannot tell it from a real number. With coercion off, only a value that arrived as a JSON number enters the sub-field, so being indexed there *is* the statement that the value is numeric. That is what delivers R2 without consulting a discriminator, and it is why there is a sub-field for numbers and none for booleans: `coerce` is not a boolean parameter, so a boolean sub-field could not have been made authoritative the same way, and OpenSearch rejects `ignore_malformed` on a boolean mapper outright (§7, question 7).

**What it produces.** Take a span carrying five attributes:

```json
{
  "tags": [
    { "key": "http.response.size", "value": 4096,  "type": "int64" },
    { "key": "sampler.param",      "value": 0.001, "type": "float64" },
    { "key": "error",              "value": true,  "type": "bool" },
    { "key": "http.method",        "value": "GET", "type": "string" },
    { "key": "retry.count",        "value": "3",   "type": "string" }
  ]
}
```

Nothing about this document changes — the mapping only changes what is built from it:

| attribute | `value` (keyword) | `value.number` |
|---|---|---|
| `http.response.size` = `4096` (int64) | `"4096"` | `4096.0` |
| `sampler.param` = `0.001` (float64) | `"0.001"` | `0.001` |
| `error` = `true` (bool) | `"true"` | — |
| `http.method` = `"GET"` (string) | `"GET"` | — |
| `retry.count` = `"3"` (string) | `"3"` | — |

**The last two rows are what `coerce: false` buys.** A string that looks like a number, and a boolean, are both absent from the numeric index, so a range over it matches neither. `span.retry.count > 2` therefore does not match this span while `span.http.response.size > 500` does, and the range needs no second clause to say so:

```json
{ "nested": { "path": "tags", "query": { "bool": { "must": [
  { "term":  { "tags.key":  "http.response.size" } },
  { "range": { "tags.value.number": { "gt": 500 } } }
] } } } }
```

The recorded `type` discriminator is still there and could be paired with the range, but it has nothing left to decide: the sub-field already holds only numbers. That is what makes the same lowering work on the elevated representation, which carries no discriminator at all. A single `double` sub-field serves the whole numeric space, so separate `long` and `double` sub-fields would add a field for no reach (§7, question 2).

Note that the `nested` query is what keeps the two clauses correlated: they must match the same array element, not two different attributes on the same span. That is the existing tag-search mechanism, unchanged.

**Why multi-fields rather than sibling fields.** The natural-looking alternative is to leave `value` as it is and add `number` and `boolean` as *siblings* of it inside the nested object, giving a flatter query path (`tags.number`) and no apparent duplication. It does not work as a mapping-only change, and the reason is the crux: a mapping declares how to index a field the document **contains**. Adding `tags.number` to `properties` does not create it — the writer has to emit it. That puts the sibling shape on the far side of R5, and from there it splits two ways, neither cheap:

- **Emit the value into both `value` and `number`.** Now the *stored document* carries it twice, which the multi-field version never does: a multi-field builds a second index entry from one stored value and adds nothing to `_source`. So this is the same index cost, strictly larger documents, and a write-path change.
- **Emit a numeric value only into `number`.** This does save the keyword entry — and it breaks `eq` on `tags.value` for every numeric attribute, which is today's behavior and what every existing query and reader depends on. It also stops being a variant of this option at all: making the type intrinsic to *where the value lives* is Option B's data model, and it carries Option B's costs.

So the duplication is not something multi-fields introduce. Serving exact match *and* ordering on one value requires two index structures whichever shape is chosen; multi-fields are the shape that needs no write-path change. They are also the idiomatic Elasticsearch spelling for precisely this — the standard `text` + `keyword` pairing is the same construct.

One variant does reach a flat name without touching the write path: map `value` with `copy_to` pointing at a sibling `number`. It is not obviously better, because `copy_to` values do not appear in `_source` either, so it buys only the shorter field path — and `copy_to` has documented restrictions around `nested` scopes that would need verifying on both engines. Not adopted, but recorded so the option is not mistaken for unexplored.

**Multi-typed keys index correctly.** Because the sub-field hangs off one `value` field shared by every key, and `ignore_malformed` skips what will not fit, a key emitted as an int by one service and a string by another produces two documents that are each indexed correctly. Nothing conflicts, nothing is rejected, and no key has to be assigned a single type. That is a direct consequence of typing per value rather than per key (§2).

All of the above is a mapping-only change.

**Option A works for the elevated representation the same way.** The elevated fields are mapped by a dynamic template, and a dynamic template's `mapping` may declare multi-fields exactly as an explicit mapping can. So the same shape applies to `tag.*`:

```json
{
  "span_tags_map": {
    "path_match": "tag.*",
    "mapping": {
      "type": "keyword", "ignore_above": 256,
      "fields": {
        "number": { "type": "double", "coerce": false, "ignore_malformed": true }
      }
    }
  }
}
```

Every elevated key then gets `tag.<key>` and `tag.<key>.number`, and `span.http.response.size > 500` lowers to a plain range on `tag.http_response_size.number` with no `nested` wrapper, because an elevated field is not in an array.

**The template declares the mappings rather than inferring them, so no key is ever assigned a single type.** Both mappings are in place for every key before any value arrives, so nothing depends on which value is seen first. A key that one service emits as an int and another as a string indexes the int into `.number` and the string into the keyword, and `ignore_malformed` skips only what cannot be indexed. The multi-typed key of §2 is therefore represented on the elevated side too.

**And `coerce: false` closes the gap the elevated side would otherwise have.** With coercion on, `tag.foo.number` would hold a stored number and a stored numeric *string* indistinguishably, and the elevated representation has no discriminator to separate them — so it would deliver ordering (R1) and not authoritative typed matching (R2). With coercion off the sub-field holds only values that arrived as numbers, which is the same statement a discriminator would have made, so both representations satisfy R2 on the same lowering.

One limit remains on the elevated side: **field count**, two mapped fields per key instead of one, against Elasticsearch's total-fields limit. That matters for a `tags_as_fields: all` deployment and is negligible for an include list.

### Option B — Type-partitioned nested arrays

Write attributes into separate nested arrays by type — `stringTags`, `doubleTags`, `boolTags` — each with a `value` mapped at that type. This is ClickHouse's typed-column model transplanted to Elasticsearch, and it makes the type intrinsic to *where* a value lives rather than a discriminator beside it.

It is the cleanest data model of the five and the most expensive to adopt: it changes what Jaeger writes, so it needs a schema version, a read path that understands both layouts, and a migration story for every existing index. It also multiplies the nested-array count per span, against an index setting that currently caps nested fields at 50.

### Option C — Let Elasticsearch infer types in the object representation

Narrow the `tag.*` dynamic template so it no longer forces `keyword`, and instead branch on `match_mapping_type` — mapping a JSON number to `double`, a boolean to `boolean`, and a string to `keyword`. Because the stored value already keeps its native type (§2), this needs no write-path change either, and the inferred mapping is the right one: it separates numbers from strings from booleans, which is the distinction an ordered or type-qualified predicate actually turns on.

**The risk is the reason the template exists, and it is a multi-typed-key problem.** Elasticsearch infers the mapping for `tag.foo` from the first value it sees in an index and applies it to every later one. So a key that one service emits as an int and another as a string gets whichever the race decided. `ignore_malformed` on the numeric branches downgrades the loser from *document rejected* to *this field not indexed for this document*, which is essential — dropping spans to serve a query is not a trade Jaeger can make — but it converts a hard failure into a silent one. The same query then returns different subsets on different days, depending on which service wrote first into each index, and nothing in the result says so.

This is the per-key-versus-per-value distinction of §2 in its concrete form. What C delivers is ordering on an elevated attribute (R1) for a consistently-typed key. It cannot deliver authoritative typed matching (R2), because the elevated form carries no discriminator to pair with and the inferred mapping conflates a stored number with a stored numeric string. Field count is its one economy: growth is unchanged from today, since the elevated form already maps exactly one field per distinct key.

### Option D — Runtime fields (Elasticsearch) / derived fields (OpenSearch)

Define a field computed at query time that casts the keyword to a number, and range-query that. Nothing is indexed, so this works on **existing** indices with no mapping change at all.

It fails R3: the cast runs per candidate document, with no index structure to prune with, so cost grows with the number of documents in the time range rather than with the number of matches. Both vendors document it as a storage-for-speed trade and advise against querying a non-indexed field except infrequently. It also fails R6, since Elasticsearch runtime fields and OpenSearch derived fields have different syntax and different minimum versions.

### Option E — Curated typed mappings for well-known keys

Map a fixed set of semantic-convention attributes at their real types by name — `http.response.status_code` as `long`, and so on — as Elastic's own OpenTelemetry integration does for the fields it knows.

This is precise and cheap to reason about, and it fails R1 by construction: it serves the keys someone thought of, and every user's most important attribute is the one specific to their system. Its reach is exactly the curated list and grows only when someone extends it.

### Evaluation

Legend: 🟢 good · 🟡 partial · 🔴 poor

| Criterion | A: typed sub-fields | B: typed arrays | C: inferred object types | D: runtime fields | E: curated keys |
|---|:-:|:-:|:-:|:-:|:-:|
| R1 — ordering on arbitrary attributes | 🟢 | 🟢 | 🟢 | 🟢 | 🔴¹ |
| R2 — authoritative typed matching | 🟢² | 🟢 | 🟡³ | 🟡 | 🟢¹ |
| R3 — query cost | 🟢 | 🟢 | 🟢 | 🔴 | 🟢 |
| R4 — mapping stability | 🟡⁴ | 🟢 | 🟡⁵ | 🟢 | 🟢 |
| R5 — no write-path change | 🟢 | 🔴 | 🟢 | 🟢 | 🟡 |
| R6 — one implementation for both engines | 🟢 | 🟢 | 🟢 | 🔴⁶ | 🟢 |
| R7 — covers both representations | 🟢⁷ | 🔴⁸ | 🟡⁹ | 🟢 | 🟡 |
| R8 — works on existing indices | 🔴 | 🔴 | 🔴 | 🟢 | 🔴 |

¹ only for the curated key set. ² in both representations, because `coerce: false` keeps a numeric string out of the numeric sub-field, so being indexed there is itself the statement that the value arrived as a number. The nested side's recorded discriminator agrees with it and is not consulted. ³ the inferred mapping honors string versus number versus boolean, which is the distinction predicates turn on, but it is per key rather than per value, so it holds only for a key that is consistently typed — and it conflates integer with double, which costs nothing for the operators involved. ⁴ no effect on the nested side, where the sub-field hangs off one shared `value`; on the elevated side it maps two fields per key instead of one, which is a bounded multiplier against the total-fields limit and only bites a `tags_as_fields: all` deployment. ⁵ `ignore_malformed` prevents rejected documents, but an attribute seen at two types becomes silently unsearchable for whichever type lost the race, and the result set says nothing about it. ⁶ Elasticsearch runtime fields and OpenSearch derived fields differ in syntax and minimum version. ⁷ the same multi-field shape applies to the nested `value` and, through the dynamic template, to the elevated fields. ⁸ replaces the nested representation and leaves the elevated one untouched. ⁹ elevated representation only.

---

## 5. Recommendation

**Adopt Option A, applied to both representations: a numeric multi-field on the nested `value`, and the same multi-field in the `tag.*` dynamic template, both with `coerce: false`. Reject B, C, D, and E as the primary mechanism.**

Option A satisfies R1 through R7 together, and it does so as a change to two mappings in one index template. It needs no write-path change, because the data it indexes is already being written. It prunes at the index, because the values are genuinely indexed. And it honors a type whether or not the caller declared one, because with coercion off the sub-field holds only values that arrived as numbers.

The property that matters most is the one easiest to overlook: **A never assigns a type to a key.** On the nested side the sub-field hangs off one shared `value`; on the elevated side the template gives every key both mappings. Either way a key that different services emit at different types is indexed correctly for every span, with no winner to pick and nothing silently dropped (§2). A is also the most reversible of the five — reverting the template stops populating the sub-field, and no document ever became invalid.

Its one real cost is field count on the elevated side, two mapped fields per key instead of one. That is a bounded multiplier, it is visible in advance, and it only presses on a `tags_as_fields: all` deployment, which is already the configuration that trades mapping size for Kibana ergonomics.

**One consequence of `coerce: false` deserves stating plainly, because it is a behavior change and not only an improvement.** An attribute a service wrote as *text* is absent from the numeric index, so an ordering predicate on it matches nothing rather than being refused. `http.status_code >= 400` is the canonical example from §1.2 and also the canonical instance of the problem: the semantic convention is an integer, but instrumentation that writes it as a string leaves the numeric sub-field empty, and the caller reads an empty answer as a complete one. That is a narrower answer than a refusal, which is the trade RFC 0005 §7 exists to prevent, and it is the price of not consulting a discriminator. What makes it acceptable is that the alternative is worse in the same direction: with coercion on, the same query answers with the *string* values ordered lexicographically, which is wrong rather than empty.

It fails only R8, and R8 is the requirement daily index rotation makes cheap. A deployment that adopts the new template gets ordered predicates on tomorrow's data, on all data once retention has turned over, and a clean `InvalidArgument` in the meantime rather than a wrong answer. That is a materially better trade than Option D's correct-but-unscalable scan, which would make the feature's cost unpredictable exactly when a time range is wide.

Option C is rejected because A reaches the same predicates on the same fields without C's failure mode. Both give ordering on an elevated attribute, and with `coerce: false` A gives authoritative typing there as well, which C cannot. C's inferred mapping is per key, so it picks one type and silently stops indexing the others — very hard for an operator to diagnose, and exactly what RFC 0005's refuse-rather-than-approximate posture exists to avoid. Today's forced-`keyword` template is what currently buys immunity from that (§2), and A keeps the immunity while adding the reach. What C saves is field count, and paying for that in silently wrong results is not a trade worth making.

Option B is the better data model and is rejected on cost: it buys nothing over A that a user can observe, while requiring a schema migration. It is worth revisiting only if a future schema change is being made for other reasons. Option E is rejected as a general solution but should be adopted opportunistically for a small set of hot semantic-convention keys, where an exact typed mapping beats a coerced sub-field.

---

## 6. Consequences

**The query path gains one branch, and the API does not change at all.** RFC 0005's structured filter already carries `gt`/`lt`/`gte`/`lte`, and the Elasticsearch reader already refuses them on an attribute. Lowering them means a range over the value's `.number` sub-field, applied to each field the attribute lives in: a `nested` query over `tags.value.number` paired with the key term, *or* a plain range over `tag.<key>.number` for an elevated key — which is the same OR across representations the reader already builds for every tag search today (§2). No proto change, no client change, no UI change.

`FilterCapabilities` needs no change either, which is not obvious. `gt`/`lt`/`gte`/`lte` are already declared, because `span.duration` needs them, and a capability declaration names levels and operators rather than the pairing of an operator with a particular attribute. So this refusal was never at the capability edge: it came from the reader, and lifting it is a reader change alone.

**Refusal stays the behavior for an index without the sub-field**, which is what makes the rollout safe. Because a range query against a missing sub-field silently matches nothing rather than erroring, the reader must *not* discover support by trying; it has to know. So the reader needs a way to tell whether the index it is about to query carries the sub-field.

**A feature gate is how it is told.** `jaeger.es.typedAttributeIndexing` decides both halves at once: with it off the template maps no sub-field and the reader refuses an ordering predicate on an attribute, and with it on the template maps the sub-field and the reader lowers the predicate onto it. One switch, so the write side and the read side cannot disagree — which is the failure a per-index-options configuration field would invite. What the gate does *not* solve is a deployment whose retention still holds indices written before it was turned on: those carry no sub-field, and a range over them matches nothing. §7 question 1 is that gap, and an alpha gate is where it is acceptable to have it.

**Nothing about span reading changes.** Mappings do not affect `_source` (§2), so reconstruction, round-tripping, and every existing snapshot test are unaffected. The change is additive to the index and invisible to the documents.

**Two adjacent gaps stay open and are explicitly out of scope.** The instrumentation and link levels of RFC 0005 remain unserviceable on this schema, for an unrelated reason — the write path folds scope attributes into span tags and does not store link attributes at all — and the index template's unused `scopeTag`/`scopeTags` fields are a remnant rather than a foundation. Those are level questions, not type questions, and belong to their own change.

---

## 7. Open questions

Questions 2 and 4 through 7 have been measured, on Elasticsearch 7.17 and 9.4 and OpenSearch 1.3 and 3.7. Two of them came back against what this RFC assumed, and both changed the design: `coerce: false` is adopted, and there is no boolean sub-field. The measurements run in CI as part of the storage integration suite, so they are re-checked against every backend version Jaeger supports rather than resting on one probe.

1. **How does the reader know an index has the typed sub-field?** *Answered for now, and not finished.* The `jaeger.es.typedAttributeIndexing` gate decides it, for the template and for the reader together (§6). What the gate cannot describe is a retention window holding indices written on both sides of the change: those older indices carry no sub-field, so an ordering predicate over a time range that reaches them matches nothing where it should refuse. Startup mapping introspection or a version recorded in the template would answer per index; "refuse until the oldest in-range index has the sub-field" is the cheaper approximation. Still open, and the reason the gate is alpha.
2. **Should `long` be mapped separately from `double`?** *Measured: the rounding is real, and one `double` stays.* `9007199254740993` indexed into the `double` sub-field reads back as `9007199254740992`, off by one as predicted, identically on both engines. Two things keep this from mattering. The keyword still holds the value exactly, so `eq` is unaffected and only the ordered comparison is approximate above 2^53. And splitting `number` into `int` and `double` would need `coerce: false` on the integer sub-field anyway — because `coerce` truncates fractions for an integer field, which would index `sampler.param = 0.001` as `0`, not malformed and so quietly wrong. Since the design now sets `coerce: false` regardless, the split is available at the cost of one more field per key if a real attribute ever needs an exact comparison above 2^53. Nothing has asked.
3. **Should the sub-field be on by default?** *Open.* It is off, behind an alpha gate. The field-count cost falls on `tags_as_fields: all`, which is the configuration least able to afford it, and question 1's gap is the other reason not to default it on yet.
4. **Is `ignore_above: 256` a problem for the sub-field?** *Measured, and the assumption behind the question was wrong.* The first half held: `ignore_above` is a keyword parameter and does not reach `value.number`. The second half — "no number is 256 characters long anyway" — does not. Under default coercion a 300-character numeric *string* is indexed into `.number` as `1e+300` and matches a range, while the parent keyword skips it for exceeding `ignore_above`. The same value is then invisible to equality and visible to ordering. `coerce: false` removes it, which is a second and independent reason to adopt it.
5. **Can a skipped sub-field be detected?** *Measured: yes, on both engines.* `ignore_malformed` records the skipped field in the `_ignored` metadata field, which is queryable, and it names the specific sub-field rather than only the document. It is useful on the elevated side, where the field is per key and so the answer names the attribute. On the nested side every attribute shares one `tags.value`, so an entry says only that some attribute on the span did not fit — which does not identify which one.
6. **Could `coerce: false` make the sub-field its own discriminator?** *Measured: yes. Adopted.* With coercion off a JSON string does not enter `value.number`, so the sub-field holds only values that arrived as numbers. The `type` pairing becomes an optimization rather than a correctness requirement on the nested side, and R2 is delivered on the elevated side, where there is no discriminator to pair with. The one unverified assumption this rested on holds: **every document was accepted**, on every engine and version tried, so `ignore_malformed` absorbs the refused coercion and the old Elasticsearch bugs ([#10070](https://github.com/elastic/elasticsearch/issues/10070), [#11498](https://github.com/elastic/elasticsearch/issues/11498), [#25289](https://github.com/elastic/elasticsearch/issues/25289)) do not reproduce. What it costs is stated in §5: an attribute written as text answers an ordering predicate with nothing rather than with a refusal.
7. **Is the `boolean` sub-field worth its field?** *Measured: it cannot do the job, and it cannot be mapped on both engines. Dropped.* The question assumed that landing in `tag.foo.boolean` would be the signal that `"true"` was a boolean rather than the string. It is not: parsing `"true"` into a boolean field is not coercion, so `coerce` does not separate them and the string is indexed there too. The keyword has the same ambiguity — it renders a JSON boolean as the term `"true"` — so there was never a distinction for the sub-field to add. Portability then settles it outright: **OpenSearch rejects `ignore_malformed` on a boolean mapper**, so the template fails to install with a 400, and without that parameter a value that is not a boolean costs the whole document, which is the one thing R4 forbids. Neither spelling can be installed on both engines. Dropping it takes the elevated multiplier from three fields per key to two (§5), and loses nothing, because equality is the only operator a boolean has and the keyword answers it.

---

## 8. References

**Jaeger code**
- [Elasticsearch span index template](../../internal/storage/elasticsearch/esclient/index_templates/jaeger-span.json) — the `keyword` mapping for `tags.value` and the `tag.*` dynamic template
- [OTLP-to-document conversion](../../internal/storage/v2/elasticsearch/tracestore/to_dbmodel.go) — `attributeToDbTag`, which preserves JSON types and records the type discriminator
- [Elasticsearch span writer](../../internal/storage/v2/elasticsearch/tracestore/core/writer.go) — `splitElevatedTags`, the per-attribute choice between representations
- [Elasticsearch filter lowering](../../internal/storage/v2/elasticsearch/tracestore/core/filter.go) — where ordered and typed predicates are refused today
- [RFC 0005 §5.4](./0005-structured-query-filters.md) — typed values, and the per-backend assessment that scores Elasticsearch 🟡 on typed predicates

**External**
- [Elasticsearch keyword type family](https://www.elastic.co/docs/reference/elasticsearch/mapping-reference/keyword) — multi-fields on a keyword, and `ignore_above`
- [Elasticsearch multi-fields](https://www.elastic.co/docs/reference/elasticsearch/mapping-reference/multi-fields) — indexing one source value several ways; the `text` + `keyword` idiom Option A follows
- [Elasticsearch `copy_to`](https://www.elastic.co/docs/reference/elasticsearch/mapping-reference/copy-to) — the sibling-field variant considered and not adopted (§4, Option A)
- [Elasticsearch `ignore_malformed`](https://www.elastic.co/docs/reference/elasticsearch/mapping-reference/ignore-malformed) — skipping a value that does not fit a field instead of rejecting the document
- [Elasticsearch numeric field types](https://www.elastic.co/docs/reference/elasticsearch/mapping-reference/number) — the ranges (`long` is full signed 64-bit; `double` is IEEE-754) and why ranges want a numeric mapping
- [Elasticsearch `coerce`](https://www.elastic.co/docs/reference/elasticsearch/mapping-reference/coerce) — string-to-number conversion and fraction truncation for integer fields (§7, questions 2 and 6)
- [Elasticsearch runtime fields](https://www.elastic.co/docs/manage-data/data-store/mapping/runtime-fields) and [OpenSearch derived fields](https://docs.opensearch.org/latest/mappings/supported-field-types/derived/) — Option D on each engine
- [Expensive queries in Elasticsearch and OpenSearch](https://bigdataboutique.com/blog/expensive-queries-in-elasticsearch-and-opensearch-a83194) — the cost of querying a field that is not indexed
