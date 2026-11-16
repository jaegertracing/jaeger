# RFC 0017: Evolving the ClickHouse Schema

- **Status:** Partially implemented
- **Author:** Yuri Shkuro
- **Created:** 2026-09-21
- **Last Updated:** 2026-11-16
- **Related:** [ADR-008 (ClickHouse storage schema)](../adr/008-clickhouse-storage-schema.md) · [#8715 (attribute search skip indexes)](https://github.com/jaegertracing/jaeger/issues/8715) · [#8918 (search performance, Bloom filter tuning)](https://github.com/jaegertracing/jaeger/issues/8918) · [ClickStack schema reference](https://clickhouse.com/docs/clickstack/ingesting-data/schemas#traces) · [ClickStack seed schema](https://github.com/ClickHouse/ClickStack/blob/main/docker/otel-collector/schema/seed/00005_otel_traces.sql) · [OTel `clickhouseexporter` DDL templates](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/exporter/clickhouseexporter/internal/sqltemplates) · [Langfuse `events_full` migration](https://github.com/langfuse/langfuse/blob/main/packages/shared/clickhouse/migrations/canonical/0039_create_events_full.up.sql)

---

## Abstract

Jaeger's ClickHouse backend ([ADR-008](../adr/008-clickhouse-storage-schema.md)) has no index that serves attribute search, so an attribute-only search took 1.77 s on the 10M-span benchmark against 37 ms to 47 ms for searches that the sort key or a `minmax` index can prune ([BENCHMARKING.md](../../internal/storage/v2/clickhouse/BENCHMARKING.md)). Its span columns carry no codecs, no `LowCardinality` and no table settings. This RFC proposes the schema changes that address both. Several are influenced by the ClickStack schema; the measurements that decide them are Jaeger's own benchmark and the internal benchmark in Addendum A.

Jaeger's typed attribute groups stand, because a single `Map(String, String)` discards the value types that Jaeger's query API needs. This RFC changes ADR-008's schema in these ways:

- **Storage tuning:** codecs, `LowCardinality` and table settings.
- **Attribute hoisting:** the attributes a deployment searches most are copied into materialized columns that operators can index.
- **Attribute index:** a text index over `key=value` items serves equality search on the attributes that are not hoisted, with a hashed Bloom filter as the fallback where the text index costs too much storage. Ordered predicates on those attributes still scan.
- **Attribute container:** one typed `Map(String, T)` column per value type replaces each `Nested` group, reversing ADR-008's choice. A map is what OTLP attributes are, a query reads `m['k']` where `Nested` needs `value[indexOf(key, 'k')]`, and ClickHouse's recent attribute work, including a bucketed serialization for single-key reads, targets `Map` and not `Nested`. In one internal benchmark the two took storage within 0.4% of each other on the same rows (Addendum A). The backend offers the `Map` layout as an experimental opt-in, and a retest of the query shapes where it measured slower on 26.8 decides when it becomes the default.

Hoisting and the attribute index work over either container. The `JSON` type is not proposed, because of its merge behavior at high path counts; §5.4 keeps it as a measurement.

---

## 1. Motivation

The OpenTelemetry Collector writes spans to ClickHouse through its `clickhouseexporter`, whose default DDL the ClickStack documentation reproduces with a handful of HyperDX-specific additions.

Three findings motivate this RFC.

1. **Jaeger's attribute search is slow, and ClickStack's is indexed.** Jaeger's benchmark shows an attribute-only search at 1.77 s against 37 ms to 47 ms for the single-predicate searches that the sort key or a `minmax` index can prune, and [#8715](https://github.com/jaegertracing/jaeger/issues/8715) proposes Bloom filter skip indexes on the attribute columns to close the gap. The `clickhouseexporter` has shipped exactly such indexes on `mapKeys` and `mapValues` for years, ClickStack has since replaced both with text indexes, and the ClickHouse observability schema-design page now says of the separate key and value filters that "we find these to be generally ineffective and don't recommend copying them into your custom schema" ([schema design](https://clickhouse.com/docs/use-cases/observability/schema-design)). Whatever Jaeger does here should be informed by what those two choices learned.
2. **Jaeger's table carries no storage tuning.** The `spans` DDL declares no compression codecs, no `LowCardinality` wrappers, and no `SETTINGS`. Every one of those appears in the ClickStack schema, and ADR-008 does not discuss any of them.
3. **Jaeger cannot read the tables the exporter writes.** A deployment that already runs the `clickhouseexporter` has two options today: dual-write into Jaeger's schema, or not use Jaeger. Knowing exactly how far apart the schemas are is the prerequisite for deciding whether a read-only adapter is worth building.

ADR-008 records the decisions behind Jaeger's schema. This RFC keeps the ones whose grounds still hold, identifies which parts of the schema were never argued for and proposes filling those in, and reopens one decision, `Nested` over `Map`, whose stated ground was a compression difference that a measurement on the same rows did not find (§2.3, Addendum A).

---

## 2. The Two Schemas

### 2.1 Jaeger

The full DDL is in [`create_spans_table.sql`](../../internal/storage/v2/clickhouse/sql/create_spans_table.sql); the shape is:

```sql
CREATE TABLE spans (
    id String, trace_id String, trace_state String, parent_span_id String,
    name String, kind String,
    start_time DateTime64(9),
    status_code String, status_message String,
    duration Int64,
    bool_attributes    Nested (key String, value Bool),
    double_attributes  Nested (key String, value Float64),
    int_attributes     Nested (key String, value Int64),
    str_attributes     Nested (key String, value String),
    complex_attributes Nested (key String, value String),
    events Nested (name String, timestamp DateTime64(9),
                   bool_attributes Nested (...), ..., complex_attributes Nested (...)),
    links  Nested (trace_id String, span_id String, trace_state String,
                   bool_attributes Nested (...), ..., complex_attributes Nested (...)),
    service_name String,
    resource_{bool,double,int,str,complex}_attributes Nested (key String, value ...),
    scope_name String, scope_version String,
    scope_{bool,double,int,str,complex}_attributes Nested (key String, value ...),
    INDEX idx_trace_id trace_id TYPE bloom_filter GRANULARITY 1,
    INDEX idx_duration duration TYPE minmax GRANULARITY 1
) ENGINE = MergeTree
PARTITION BY toDate(start_time)
ORDER BY (service_name, name, toDateTime(start_time))
TTL start_time + INTERVAL <ttl> SECOND DELETE
```

The `attribute_layout` setting can declare every typed group as `Map(String, T)` instead (§5.4); the DDL above is the default `nested` layout. Five derived tables hang off it through materialized views: `trace_id_timestamps` (per-trace min/max start time, `AggregatingMergeTree`), `services`, `operations`, `attribute_metadata` (every `(key, type, level)` triple ever seen, fed by three views), plus a `dependencies` table that the dependency writer fills from a separate aggregation job rather than a materialized view.

### 2.2 ClickStack

The DDL below is the one the ClickStack documentation publishes. It is the `clickhouseexporter`'s [`traces_table.sql`](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/exporter/clickhouseexporter/internal/sqltemplates/traces_table.sql) with HyperDX-specific additions, marked below, and two cosmetic differences: the exporter template writes the timestamp codec as `CODEC(Delta, ZSTD(1))` and renders the TTL as `toDateTime(Timestamp) + toInterval...`, both equivalent to what is shown here. The block is abridged: the per-column `ZSTD(1)` codecs on the HyperDX columns and the `T64` codec on `SampleRate` are omitted.

```sql
CREATE TABLE otel_traces (
    Timestamp DateTime64(9) CODEC(Delta(8), ZSTD(1)),
    TraceId String CODEC(ZSTD(1)), SpanId String CODEC(ZSTD(1)),
    ParentSpanId String CODEC(ZSTD(1)), TraceState String CODEC(ZSTD(1)),
    SpanName LowCardinality(String) CODEC(ZSTD(1)),
    SpanKind LowCardinality(String) CODEC(ZSTD(1)),
    ServiceName LowCardinality(String) CODEC(ZSTD(1)),
    ResourceAttributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
    ScopeName String CODEC(ZSTD(1)), ScopeVersion String CODEC(ZSTD(1)),
    SpanAttributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
    Duration UInt64 CODEC(ZSTD(1)),
    StatusCode LowCardinality(String) CODEC(ZSTD(1)),
    StatusMessage String CODEC(ZSTD(1)),
    Events Nested (Timestamp DateTime64(9), Name LowCardinality(String),
                   Attributes Map(LowCardinality(String), String)) CODEC(ZSTD(1)),
    Links  Nested (TraceId String, SpanId String, TraceState String,
                   Attributes Map(LowCardinality(String), String)) CODEC(ZSTD(1)),
    -- HyperDX additions:
    `__hdx_materialized_rum.sessionId` String MATERIALIZED ResourceAttributes['rum.sessionId'],
    SampleRate UInt64 MATERIALIZED greatest(toUInt64OrZero(SpanAttributes['SampleRate']), 1),
    ResourceAttributeItems Array(String) ALIAS arrayMap((arr) -> concat(arr.1, '=', arr.2), ResourceAttributes::Array(Tuple(String, String))),
    SpanAttributeItems     Array(String) ALIAS arrayMap((arr) -> concat(arr.1, '=', arr.2), SpanAttributes::Array(Tuple(String, String))),
    -- indexes:
    INDEX idx_trace_id TraceId TYPE bloom_filter(0.001) GRANULARITY 1,
    INDEX idx_res_attr_key  mapKeys(ResourceAttributes) TYPE bloom_filter(0.01) GRANULARITY 1,
    INDEX idx_span_attr_key mapKeys(SpanAttributes)     TYPE bloom_filter(0.01) GRANULARITY 1,
    INDEX idx_res_attr_items  ResourceAttributeItems TYPE text(tokenizer = 'array'),   -- HyperDX; exporter uses bloom_filter on mapValues
    INDEX idx_span_attr_items SpanAttributeItems     TYPE text(tokenizer = 'array'),   -- HyperDX; exporter uses bloom_filter on mapValues
    INDEX idx_duration Duration TYPE minmax GRANULARITY 1,
    INDEX idx_lower_span_name lower(SpanName) TYPE tokenbf_v1(32768, 3, 0) GRANULARITY 8,  -- HyperDX
    INDEX idx_rum_session_id `__hdx_materialized_rum.sessionId` TYPE bloom_filter(0.001) GRANULARITY 1  -- HyperDX
) ENGINE = MergeTree
PARTITION BY toDate(Timestamp)
ORDER BY (ServiceName, SpanName, toDateTime(Timestamp))
TTL toDate(Timestamp) + <ttl>
SETTINGS index_granularity = 8192, ttl_only_drop_parts = 1;
```

One derived table accompanies it, defined by the exporter templates `traces_id_ts_lookup_table.sql` and `traces_id_ts_lookup_mv.sql` rather than by the ClickStack page: `otel_traces_trace_id_ts` (`TraceId`, `Start DateTime`, `End DateTime`), a plain `MergeTree` partitioned by day and ordered by `(TraceId, Start)`, fed by a materialized view that groups `min`/`max` of `Timestamp` per `TraceId` within each insert block. There is no services, operations, or attribute-metadata table; HyperDX answers those questions by querying the `LowCardinality` columns of the main table directly.

Since February 2026 the ClickStack collector creates its tables from its own [seed schema](https://github.com/ClickHouse/ClickStack/blob/main/docker/otel-collector/schema/seed/00005_otel_traces.sql) rather than from the exporter templates, and that schema has moved past the block above in four ways. The trace ID carries a `text(tokenizer = 'array')` index instead of the Bloom filter, and the `otel_traces_trace_id_ts` lookup table is gone ([What's new in ClickStack, April 2026](https://clickhouse.com/blog/whats-new-in-clickstack-april-2026)). The key Bloom filters on `mapKeys()` are text indexes, the span-name token filter is a `splitByNonAlpha` text index, and attribute equality is read straight from the index through the `direct_read` optimization, which needs ClickHouse 26.5 or later patch releases of 26.2 through 26.4 ([ClickStack PR 2341](https://github.com/ClickHouse/ClickStack/pull/2341)). Every codec moved from `ZSTD(1)` to `ZSTD(3)` in November 2026, and the upstream exporter made the same change in the same week ([contrib PR 52706](https://github.com/open-telemetry/opentelemetry-collector-contrib/pull/52706)). A `SummingMergeTree` rollup table feeds autocomplete. The seed schema requires ClickHouse 26.2, with a `bloom_filter` compatibility schema for older servers. §3.5 and §5 account for these changes. The block above is kept as the exporter's index and table shape; since contrib PR 52706 the exporter's codecs are `ZSTD(3)` rather than the `ZSTD(1)` shown.

The exporter also ships an experimental **JSON variant** ([`traces_json_table.sql`](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/exporter/clickhouseexporter/internal/sqltemplates/traces_json_table.sql)) in which `ResourceAttributes` and `SpanAttributes` are ClickHouse `JSON` columns accompanied by `*AttributesKeys Array(LowCardinality(String))` columns that carry the key Bloom filters, the sort key gains `Timestamp` as a fourth component, and the trace-ID Bloom filter is absent, so trace retrieval by ID falls back to the sort key and partition pruning alone. ClickStack promoted it in July 2025 as "up to 9x faster over the Map type" ([What's new in ClickStack, July 2025](https://clickhouse.com/blog/whats-new-clickstack-july-2025)) and has since walked it back: its documentation now says "For typical observability workloads we recommend keeping the default Map-based schema", marks `JSON` as "Beta feature, not production ready", and recommends it only where "a small, stable set of attribute keys", ingest throughput that is modest relative to attribute cardinality, and a need for typed access all hold ([Map vs JSON](https://clickhouse.com/docs/clickstack/ingesting-data/schema/map-vs-json)). When ClickStack runs in JSON mode its collector falls back to the exporter's JSON DDL and bypasses every seed table, so none of the ClickStack indexing or rollup work exists in that mode ([collector `entrypoint.sh`](https://github.com/ClickHouse/ClickStack/blob/main/docker/otel-collector/entrypoint.sh)). §3.2 accounts for the variant and §2.3 for the reversal.

### 2.3 What changed in ClickHouse since the Jaeger schema was designed

ADR-008 was written against ClickHouse 25.x. The releases since then changed the three types a span-attribute schema can choose from, and the text index became the attribute-search mechanism that ClickStack uses. The timeline is compiled from the [release changelogs](https://clickhouse.com/docs/resources/changelogs/oss/2026) and release posts.

| Release | Date | Change |
| --- | --- | --- |
| 25.3 LTS | 2025-03 | `JSON`, `Variant` and `Dynamic` declared production-ready |
| 25.10 | 2025-10 | Text index accepted on `mapKeys()` and `mapValues()` |
| 25.12 | 2025-12 | `advanced` shared-data format becomes the `JSON` default; text index beta |
| 26.2 | 2026-02 | Text index GA ([Full-text search is GA](https://clickhouse.com/blog/full-text-search-ga-release)) |
| 26.3 LTS | 2026-03 | `map_serialization_version = 'with_buckets'`: a `Map` is stored as up to 32 hashed sub-arrays and a constant-key lookup reads one bucket; `m['key']` rewritten to a key subcolumn ([release 26.3](https://clickhouse.com/blog/clickhouse-release-26-03)) |
| 26.4 | 2026-04 | Skip indexes over `JSONAllPaths()` and `JSONAllValues()`; `LIKE` served by the text index |
| 26.9 | 2026-09 | `keyValuePairs` text tokenizer directly on `Map(String, String)`; `mapKeys()`, `mapValues()` and `mapContainsValue()` filters read only the matching subcolumn |
| 26.10 | 2026-10 | `JSON` lazy type hints GA; text index answers `map['key'] IN (...)`; `mapContainsKeyValue()` |
| 26.11 | pending | `PREWHERE` read steps are grouped by exact columns, and the `m['key']` subcolumn rewrite is restricted to `with_buckets` tables, because on the default `basic` serialization "the whole Map is read once, as before 26.3" ([PR 124553](https://github.com/ClickHouse/ClickHouse/pull/124553)); backported to 26.3, 26.8, 26.9 and 26.10, first released on the LTS lines in 26.3.63 and 26.8.37, where the backports have an open report of `PREWHERE` query failures under a non-default planner setting |

Jaeger's CI tests two LTS lines, 26.3.57 and 26.8.32 (`docker-compose/clickhouse/`). The rows through 26.3 are available on both, the 26.4 row only on 26.8, and the 26.9 and 26.10 rows on neither; both images predate the LTS backports of the 26.11 change. Three facts from this timeline bear on the proposal.

**The `Map` on-disk layout is the same arrays as `Nested`.** The [`Map` documentation](https://clickhouse.com/docs/sql-reference/data-types/map) describes the default serialization as `Array(Tuple(K, V))`, which on disk is a keys array, a values array and offsets, the layout `Nested` has always had. The bucketed serialization is opt-in, off by default, disables vertical merges, and is recommended only for maps over about 32 keys where single-key lookups dominate. An internal benchmark (Addendum A) stored the same spans in Jaeger's array layout, in one typed `Map` per attribute group, and in the same `Map` with the bucketed serialization, with `LowCardinality(String)` keys in all three, and measured the whole tables, which also carry identical promoted columns and indexes, within 0.4% of each other and the bucketed one 6% larger. Compression therefore does not separate `Map` from `Nested` on the basic serialization.

**ClickHouse's recent attribute work targets `Map`.** Part of it applies equally to the arrays underneath `Nested`: the text index with the `array` tokenizer indexes any `Array(String)` column, its `mapKeys()` and `mapValues()` forms are that tokenizer applied to the two arrays a `Map` holds, and ClickStack's `key=value` item index is an `ALIAS` computed by `arrayMap` over the map cast to an array of tuples, which the same `ALIAS` over `str_attributes.key` and `str_attributes.value` reproduces. The rest names `Map` and has no array counterpart: the bucketed serialization (26.3), the `keyValuePairs` tokenizer and the subcolumn reads behind `mapKeys()`, `mapValues()` and `mapContainsValue()` (26.9), and text-index support for `map['key'] IN (...)` together with `mapContainsKeyValue()` (26.10). One gap may run the other way on 26.8: in the internal benchmark, whole-row queries on a `Map` took roughly as long as whole-row queries on the arrays with lazy materialization switched off, which suggests the `Map` columns did not get the lazy materialization the arrays got (Addendum A).

**`JSON` has been re-scoped from the future to a niche.** The ClickStack reversal described in §2.2 has a field report behind it, [ClickHouse issue 89944](https://github.com/ClickHouse/ClickHouse/issues/89944): a `JSON` `otel_traces` table at about 700k spans/s with 388 distinct span-attribute paths had merges running over 60 hours on merged parts of about 130 GB and a one-day TTL that never applied, and rolling back to the `Map` table removed the problem. The issue is open a year later. ClickStack's July 2026 performance post ends with "For now, maps remain the most practical representation for dynamic telemetry attributes" ([How we made ClickStack 5x faster](https://clickhouse.com/blog/making-clickstack-5x-faster-clickhouse-observability)), and neither ClickStack nor Langfuse stores attributes as `JSON` by default. Langfuse, an LLM observability product, redesigned its schema in 2026 with a 25.12 minimum and uses no `JSON` column: GenAI fields are typed columns, token counts and costs are typed `Map` columns, and unrecognized attributes are two parallel `Array(String)` columns ([migration 0039](https://github.com/langfuse/langfuse/blob/main/packages/shared/clickhouse/migrations/canonical/0039_create_events_full.up.sql)).

---

## 3. Differences

The differences fall into eight groups. For each, the table states what each schema does and whether the Jaeger position is a recorded decision (ADR-008 argues for it), an omission (ADR-008 does not discuss it), or a consequence of a decision made elsewhere.

### 3.1 Identity and scalar columns

| Field | Jaeger | ClickStack | Note |
| --- | --- | --- | --- |
| Trace / span / parent IDs | `String`, lowercase hex | `String`, lowercase hex | Identical encoding. |
| Timestamp | `start_time DateTime64(9)` | `Timestamp DateTime64(9)` | Same type; ClickStack adds `CODEC(Delta(8), ZSTD(1))`. |
| Duration | `Int64` nanoseconds | `UInt64` nanoseconds | Same unit; sign differs. A negative duration is not meaningful, but `Int64` costs nothing and matches Go's `time.Duration`. |
| Span kind | `String`, lowercase (`server`), empty for unspecified | `LowCardinality(String)`, Go enum name (`Server`, `Unspecified`) | Casing differs. Jaeger's convention predates the ClickHouse backend and is shared with its other storages through `jptrace.SpanKindToString`. |
| Status code | `String` (`Ok`, `Error`, `Unset`) | `LowCardinality(String)`, same values | Identical values. |
| Scope attributes | Stored as typed `Nested` groups | **Not stored** | The exporter drops instrumentation-scope attributes entirely. Jaeger round-trips them. |
| Column naming | `snake_case` | `CamelCase` | Cosmetic; matters only for a read adapter (§6). |

Nothing here is a design disagreement. The one substantive gap, scope attributes, is a ClickStack omission rather than a Jaeger one.

### 3.2 Attribute representation

This is the deepest divergence and the one ADR-008 spends most of its argument on.

| Aspect | Jaeger (typed `Nested`) | ClickStack (`Map`) | Exporter JSON variant |
| --- | --- | --- | --- |
| Value types | Preserved: one `Nested(key, value)` group per primitive type, complex values serialized to strings (maps and slices as JSON, bytes as base64) under a `@bytes@`/`@map@`/`@slice@` key prefix | **Lost**: every value passes through `AsString()` into `Map(LowCardinality(String), String)` | Mostly preserved by ClickHouse's dynamic `JSON` typing; the exporter marshals attributes through `json.Marshal`, so bytes become base64 strings and a double `1.0` is indistinguishable from an integer `1` |
| Key identity | Preserved: a key is an opaque string | Preserved | **Lost for dotted keys by default**: the `JSON` type reads `.` as nesting, so `{"http.status_code": 200}` and `{"http": {"status_code": 200}}` are the same path, and a map holding both fails insertion with `Duplicate path found`. The `json_type_escape_dots_in_keys` setting (available on 25.12.11) has the server escape dots to `%2E` on parse and unescape them on output, so both forms coexist, but it is not reversible for a key that already contains a literal `%2E`: on 25.12.11.4 `{"a%2Eb": 3}` reads back as `{"a.b": 3}`, and a map holding both `a.b` and `a%2Eb` fails with `Duplicate path found`. A Jaeger variant therefore escapes in the writer, `%` first (`%` → `%25`, then `.` → `%2E`), leaves the server setting off, and inverts the mapping in the reader; subcolumn predicates and path type hints name the escaped path (`attrs.\`http%2Estatus_code\``) |
| Level separation | Separate column groups for resource, scope, span, event, link | Separate `Map` per level (no scope) | Separate `JSON` per level (no scope) |
| Query-time type resolution | Needs the `attribute_metadata` table to learn which typed column a string filter should hit | Not needed: everything is a string | Needed: an indexable predicate has to name the typed subcolumn (`attrs.key.:Int64`), so something must know the type; the exporter has no reader and does not address it |
| Filter expression | `arrayExists((k, v) -> k = ? AND v = ?, col.key, col.value)`, one per `(type, level)` observed | `SpanAttributes['key'] = 'value'` | `SpanAttributes.key.:Int64 = value` on the typed subcolumn; the untyped `SpanAttributes.key = value` is a `Dynamic` comparison that no index serves |
| Point-lookup cost per row | Scan the short arrays for that type | Scan the map's key array | Subcolumn read; cheapest of the three, provided the path is typed |
| Compression | 8.6x whole-table ratio measured ([BENCHMARKING.md](../../internal/storage/v2/clickhouse/BENCHMARKING.md)); the attribute columns' share of it is not broken out | Same bytes as the arrays under `Nested` for the basic serialization: an internal benchmark measured one typed `Map` per attribute group in a table 0.4% larger than Jaeger's layout on the same spans, and 6% larger with the bucketed serialization (Addendum A) | Not measured |

ADR-008 made two separable choices here: typed groups rather than one string type, and `Nested` rather than `Map` as the container for each group. The first stands, and the type fidelity argument is stronger than the ADR states: the Jaeger query API accepts typed `pcommon.Value` filters, and a `Map(String, String)` cannot answer `http.status_code = 500` (integer) differently from `= "500"` (string), nor can it support the ordered predicates of [RFC 0005](0005-structured-query-filters.md) (`http.status_code >= 400` on an attribute) without parsing strings at query time. ClickStack accepts that limitation; Jaeger's typed query API cannot. Langfuse reached the same conclusion from a different need: its `events_full` table stores token counts as `Map(LowCardinality(String), UInt64)` and cost as `Map(LowCardinality(String), Decimal(18, 12))`, because a cost dashboard cannot sum strings. That is ADR-008's rejected Option 3, one typed `Map` per value type, applied to the fields where the type matters.

The second choice, `Nested` over `Map`, no longer holds. OTLP attributes are a map from key to value, and a `Map` column states that in its type, where a `Nested` group holds two arrays that are paired only by position and that every lookup reassembles with `indexOf`. On the basic serialization the two store the same arrays and measured the same size (§2.3), `Map` guarantees equal key and value counts as `Nested` does, and `Map` holds one value type per column, so the typed layout needs five `Map` columns per level either way. Where the two differ, `Map` is ahead: ClickHouse's newer index forms, functions and serializations name `Map` (§2.3), and a `Map` with the bucketed settings of footnote ⁵ in §4.2 reads one bucket for `m['http.route']` where the `Nested` group reads the whole key and value arrays of that type. Langfuse moved from `Map` to parallel arrays in 2026: its earlier `traces` table kept metadata in a `Map(LowCardinality(String), String)` column ([migration 0001](https://github.com/langfuse/langfuse/blob/main/packages/shared/clickhouse/migrations/canonical/0001_traces.up.sql)), and `events_full` keeps it in the two parallel `Array(String)` columns of §2.3. Neither the migration nor the pull request that added it states a reason, and those columns hold untyped strings searched through text indexes on the names and the values, so the move says little about the container for Jaeger's typed groups. §5.4 weighs the `Map` advantages against the migration cost and the maturity of the `Map` read path on the pinned releases, and Addendum A holds the measurements.

The JSON variant gives most of the type fidelity of Jaeger's layout (JSON has no bytes type and no integer/double distinction for integral values, so a Jaeger variant would need the `@bytes@`-style tagging for those two cases) with a per-key subcolumn on disk, which is exactly what ADR-008's "Negative / Limitations" section wishes for when it says attribute filters "can't be SIMD-vectorized or skip-indexed the way a flat column can". It has five costs today. The type is production-ready only in ClickHouse 25.3 and later. The exporter still labels its variant experimental and ClickStack has withdrawn it to a beta option for a small, stable key set (§2.2). Dotted keys need escaping in the writer, because the server's own `json_type_escape_dots_in_keys` is not reversible for keys that contain a literal `%2E`, and predicates and type hints then name the escaped path, as the table says. Its indexing is not automatic: on 25.12 a path without a type hint has type `Dynamic`, and neither `minmax` nor `bloom_filter` can be declared on it, while an index on the typed subcolumn (`attrs.key.:Int64`) works and prunes only when the predicate names that same typed subcolumn. So the query builder still has to know each key's type, which is exactly what `attribute_metadata` records today, and per-key skip indexes have to be created deliberately for the keys that need them. And its merge behavior at hundreds of distinct paths is the subject of an open production report of 60-hour merges (§2.3), which no 10M-span benchmark exercises. §5 does not propose adopting it; §5.4 keeps it as a measurement with a merge-behavior criterion.

### 3.3 Events and links

Both schemas store events and links as `Nested` arrays on the span row, so a span with five links is one row with arrays of length five. The only difference is inside: ClickStack's `Events.Attributes` is `Array(Map(...))`, Jaeger's is five typed `Nested` groups nested inside the outer `Nested`. Jaeger's query builder searches them with a doubly nested `arrayExists`; HyperDX searches them with `arrayExists(m -> m['key'] = 'v', Events.Attributes)`. Same cost class, same absence of any index. Neither schema indexes event or link attributes, and the ClickStack `*AttributeItems` aliases cover only resource and span attributes.

### 3.4 Storage tuning

| Setting | Jaeger | ClickStack | Effect |
| --- | --- | --- | --- |
| Column codecs | None (server default, LZ4) | `ZSTD(3)` on every column, `Delta(8), ZSTD(3)` on `Timestamp`; `ZSTD(1)` until November 2026 | ZSTD is expected to compress repetitive telemetry strings better than LZ4, at a CPU cost on write that grows with the level, and benchmark B1 (§7.2) measures both; `Delta` on a sorted timestamp turns nanosecond values into small differences before compression. Both ClickStack and the exporter moved from level 1 to level 3 in November 2026 (§2.2). |
| `LowCardinality` | None | `ServiceName`, `SpanName`, `SpanKind`, `StatusCode`, `Events.Name`, all `Map` keys | Dictionary-encodes the column, which is expected to shrink it on disk and speed up `GROUP BY` and equality filters. |
| `index_granularity` | Default (8192) | Explicit 8192 | No behavioral difference; explicit is documentation. |
| `ttl_only_drop_parts` | Default (0) | `1` | With `0`, an expired part is rewritten to drop expired rows, a heavy merge. With `1`, a part is dropped only once every row in it has expired, which for a day-partitioned table means whole parts vanish at once for free. |
| TTL expression | `start_time + INTERVAL n SECOND` | `toDate(Timestamp) + INTERVAL n` | Equivalent to within a day. |

Every row in this table is an omission on Jaeger's side. None of them was weighed in ADR-008, and the only behavioral change among them is the retention slack that `ttl_only_drop_parts` introduces, which §5.1 quantifies. `LowCardinality` on `service_name` and `name` deserves one caveat: ClickHouse's own guidance is to use it for columns under roughly ten thousand distinct values ([`LowCardinality`](https://clickhouse.com/docs/sql-reference/data-types/lowcardinality)), and a deployment with more operation names than that would see the dictionary spill and lose the benefit without becoming incorrect.

### 3.5 Skip indexes

| Index | Jaeger | Exporter default | ClickStack seed (26.2 or later) |
| --- | --- | --- | --- |
| Trace ID | `bloom_filter` (default false-positive rate 0.025) | `bloom_filter(0.001)` | `text(tokenizer = 'array')`; no lookup table |
| Duration | `minmax` | `minmax` | `minmax` |
| Attribute keys | none | `bloom_filter(0.01)` on `mapKeys(...)` | `text(tokenizer = 'array')` on `mapKeys(...)` |
| Attribute values | none | `bloom_filter(0.01)` on `mapValues(...)` | `text(tokenizer = 'array')` on the `key=value` alias columns |
| Span name tokens | none | none | `text(tokenizer = 'splitByNonAlpha')` on lowercased `SpanName` |

Three observations.

**The trace-ID Bloom filter is the same idea at a different tolerance.** Jaeger relies on the server default of a 2.5 percent false-positive rate. At granularity 1 every granule carries its own filter, so a false positive costs one decompressed granule of roughly 8,192 rows. The reporter of [#8918](https://github.com/jaegertracing/jaeger/issues/8918), running a billion-row table, had to tighten the rate to 0.0001 by hand to make `GetTraces` acceptable; [#8923](https://github.com/jaegertracing/jaeger/issues/8923), which proposed rewriting `FindTraces` into two queries and making the rate configurable, was closed unmerged, and the DDL on `main` still carries the 0.025 default. The exporter's 0.001 is twenty-five times tighter than Jaeger's default at about twice the index size, because a Bloom filter's bits per element grow with ln(1/p). ClickStack has since replaced the Bloom filter with a text index on the trace ID and dropped the lookup table (§2.2); whether the text index prunes well enough to make Jaeger's `trace_id_timestamps` table optional is a measurement for benchmark B1, not a decision this RFC makes.

**Separate key and value Bloom filters cannot prove a pair absent.** This is a pruning limit, not a correctness one: a skip index only chooses which granules to read, and the `WHERE` clause is still evaluated on every surviving row, so the result set is the same whichever index shape is used and only the latency differs. The exporter's `mapKeys`/`mapValues` indexes each answer "does any row in this granule contain this key" and "does any row contain this value". A granule that contains the key `http.method` on one row and the value `POST` on another passes both filters even if no row has `http.method=POST`. For a high-cardinality attribute (a user ID, a request ID) the value filter alone can be selective. ClickStack's `key=value` item index closes the gap exactly. It depends on the `text` index type, which was beta on 25.12.11, the release the §5.3 measurements were made on, and is [generally available from ClickHouse 26.2](https://clickhouse.com/docs/engines/table-engines/mergetree-family/invertedindexes); from 26.5 the index serves the `direct_read` optimization, and ClickStack's measurement had it run "between 1.4 and 10 times faster than the equivalent map-subscript query" with insert overhead "broadly comparable" to Bloom filters ([April 2026 post](https://clickhouse.com/blog/whats-new-in-clickstack-april-2026)). The text index costs more on disk: the GA post reports a 216 GiB `splitByNonAlpha` text index against a 7 GiB Bloom filter on a free-text column of a 10-billion-row table, and describes text indexes as "typically slightly slower on inserts" than Bloom filters ([Full-text search is GA](https://clickhouse.com/blog/full-text-search-ga-release)). Neither number is for a `key=value` array index. [#8715](https://github.com/jaegertracing/jaeger/issues/8715) proposes the exporter's separate-filter shape for Jaeger; §5.3 argues for the pairwise shape, as a text index on a `key=value` alias where the storage cost is acceptable and as a hashed `bloom_filter` where it is not.

**Whether an index is consulted depends on the query expression.** A skip index is used only when the `WHERE` clause contains the indexed expression in a form the planner recognizes: `has(arr, x)`, `mapContains(m, k)`, `m[k] = v`, and similar. Jaeger's query builder emits `arrayExists` with a lambda, which no skip index can serve. Adding indexes without changing the predicate shape does nothing, which is a detail [#8715](https://github.com/jaegertracing/jaeger/issues/8715) leaves implicit and §5.3 makes explicit.

### 3.6 Derived tables

| Table | Jaeger | ClickStack |
| --- | --- | --- |
| Per-trace time bounds | `trace_id_timestamps`: `AggregatingMergeTree`, `ORDER BY trace_id`, `SimpleAggregateFunction(min/max, DateTime64(9))`, no partition, TTL on `end` | `otel_traces_trace_id_ts`: `MergeTree`, `PARTITION BY toDate(Start)`, `ORDER BY (TraceId, Start)`, `DateTime` (second) precision, own `bloom_filter(0.01)` on `TraceId` |
| Services | `services` (`AggregatingMergeTree`) | none; `SELECT DISTINCT ServiceName` on the main table |
| Operations | `operations` keyed `(service_name, span_kind)` | none; `GROUP BY SpanName` on the main table |
| Attribute metadata | `attribute_metadata` (`(key, type, level)` triples) | none; not needed for untyped `Map` |
| Dependencies | `dependencies` (JSON written by the dependency writer, not by a materialized view) | none |

The time-bounds tables differ in an instructive way. Jaeger's collapses to one row per trace in the background and is not partitioned, so a lookup is a point read on the sort key but TTL expiry must rewrite parts. The exporter's keeps one row per trace per insert block, partitions by day so that TTL drops whole parts, and leaves the `min`/`max` to the reader. With `ttl_only_drop_parts` this is the cheaper design at retention time and the marginally more expensive one at read time; for Jaeger the read happens once per search, on both the `FindTraceIDs` and `FindTraces` paths, over a bounded candidate set, so the difference is expected to be small either way; it is not measured. The ClickStack seed schema has no bounds table at all and prunes trace ID lookups by the text index alone (§2.2). Jaeger's table exists so that `FindTraceIDs` can hand `GetTraces` a time window, and its value depends on the Bloom filter's false-positive rate, which [#8918](https://github.com/jaegertracing/jaeger/issues/8918) showed to be too loose at a billion rows. §5.1 adds the text index to benchmark B1; §5 otherwise leaves the table alone.

The services and operations tables exist because Jaeger's API needs them as first-class lists and ADR-008 chose to precompute them. ClickStack answers the same questions with `DISTINCT` and `GROUP BY` over the `LowCardinality` columns of the main table. If Jaeger adopts `LowCardinality` (§5.1) the same shortcut becomes available, but replacing a working precomputation is a maintainer's call and is out of scope here; [#8906](https://github.com/jaegertracing/jaeger/issues/8906) (operations table collapsing names) is the place that decision would be made.

### 3.7 Product-specific columns

The `__hdx_materialized_rum.sessionId` and `SampleRate` materialized columns and the `idx_lower_span_name` token filter are HyperDX features (session replay correlation, tail-sampling weights, substring search on span names). Their product-specific expressions are not proposed for Jaeger. The reusable mechanism is a `MATERIALIZED` column computed from an attribute lookup, plus an operator-selected index. Section 5.2 applies that mechanism to configured typed attributes in Jaeger's attribute groups, whichever container §5.4 settles on, retaining the original attributes and leaving index creation to the operator. A dedicated column avoids scanning the attribute arrays for that value; its pruning benefit depends on the chosen index and data distribution, and it does not acquire `service_name`'s position at the front of the sort key.

### 3.8 Schema management

| Capability | Jaeger | Exporter |
| --- | --- | --- |
| Create schema on start | `create_schema: true` | `create_schema: true` (default) |
| Bring your own schema | Supported by setting `create_schema: false` and creating compatible tables | Same, and documented as the recommended production mode |
| Table engine | `table_engine` block, `merge_tree` or `replicated`, required with `create_schema: true` | `table_engine` config (name plus parameters), so `ReplicatedMergeTree` works |
| Cluster DDL | none | `cluster_name` adds `ON CLUSTER` to every statement |
| Database creation | Assumed to exist | Created if missing |

Jaeger's single-node limitation was recorded in ADR-008 until §5.5 lifted it; the exporter's `table_engine` option was the model.

---

## 4. Assessment

### 4.1 Deployment criteria

The criteria below are the ones a Jaeger deployment cares about. The columns are Jaeger's schema as it stands, Jaeger's schema with the §5 proposals applied, and ClickStack's `Map` schema for comparison. Legend: 🟢 good · 🟨 partial or caveated · ❌ poor.

| Criterion | Jaeger today | Jaeger + §5 | ClickStack (`Map`) |
| --- | --- | --- | --- |
| Attribute type fidelity | 🟢 | 🟢 | ❌ ¹ |
| Attribute-only search latency | ❌ ² | 🟨 ³ | 🟨 ³ |
| Compression on disk | 🟨 ⁴ | 🟢 | 🟢 |
| Trace retrieval by ID at scale | 🟨 ⁵ | 🟢 | 🟢 ⁶ |
| Retention cost | 🟨 ⁷ | 🟢 ⁸ | 🟢 |
| Scope attributes preserved | 🟢 | 🟢 | ❌ |
| Replicated / clustered deployment | 🟢 ⁹ | 🟢 | 🟢 |
| Reads tables written by the OTel `clickhouseexporter` | ❌ | ❌ ¹⁰ | 🟢 |
| Schema simplicity (tables, views) | 🟨 ¹¹ | 🟨 ¹¹ | 🟢 |

- ¹ Every value is stringified on write; integer, boolean, and ordered predicates are unanswerable without query-time parsing.
- ² 1.77 s attribute-only search versus 37 ms to 47 ms for the single-predicate searches that the sort key or a `minmax` index can prune, on the 10M-span benchmark; no skip index exists on any attribute column.
- ³ Either shape §5.3 selects prunes equality by granule and still pays a per-row check on the survivors (Jaeger's `arrayExists`, ClickStack's map lookup), so the result is bounded by selectivity rather than fixed; neither serves ordered predicates. ClickStack's pairwise item index prunes a common key with a common value; the exporter's default separate key and value filters do not (§3.5), which costs latency and never correctness. Hoisting (§5.2) gives selected attributes their own columns; without the general index, unhoisted attributes retain today's search behavior, and hoisted-column pruning depends on operator-managed indexes.
- ⁴ 8.6x whole-table ratio measured, but with the server default LZ4 codec and no dictionary encoding on the sort-key columns.
- ⁵ Default 2.5 percent Bloom false-positive rate; a billion-row deployment reported needing 0.0001.
- ⁶ The seed schema's text index on the trace ID (§2.2); ClickStack publishes no comparison with its earlier `bloom_filter(0.001)`. The exporter's table keeps the Bloom filter with the rate fixed in the DDL.
- ⁷ TTL expiry rewrites parts to remove expired rows instead of dropping whole day-partition parts.
- ⁸ Whole-part expiry applies to `spans`, which holds nearly all the data; `trace_id_timestamps` keeps row-level TTL (§5.1), and expiry lags the configured TTL by up to one partition day.
- ⁹ Through the `table_engine` block of §5.5; before it, the SQL templates hard-coded `MergeTree` and a cluster had to create every table by hand with `create_schema: false`.
- ¹⁰ The proposals change neither column names nor the string-typed representation, so a ClickStack table remains unreadable by Jaeger's reader; §6 covers what would.
- ¹¹ One main table plus five derived tables and six materialized views, against ClickStack's one plus one.

The "Jaeger + §5" column scores the same under either attribute container of §4.2, because every mechanism in §5.1 to §5.3 applies to the arrays of a `Nested` group and to the arrays of a typed `Map` alike. On that basis the matrix puts Jaeger's gaps in the tuning and indexing layer, which ADR-008 did not address, and the proposals below change everything in the tuning and indexing layer and, conditionally, the container of the attribute groups.

### 4.2 Attribute layout options

The second matrix scores the attribute layout itself. The options are the `Nested` groups as they stand, one typed `Map(String, T)` column per value type in place of each `Nested` group with the basic serialization by default and `with_buckets` as a table setting a deployment may enable, one `Map(String, Variant(Bool, Int64, Float64, String))` per level in which each value carries its own type, the ClickStack schema as shipped, and the exporter's `JSON` variant. In every column the §5.1 to §5.3 mechanisms are assumed applied. Legend: 🟢 good · 🟨 partial or caveated · ❌ poor.

| Criterion | Keep `Nested` | Typed `Map` per value type | One `Map(String, Variant)` per level | Adopt the ClickStack `Map` schema | Adopt the exporter `JSON` schema |
| --- | --- | --- | --- | --- | --- |
| Typed and ordered attribute predicates (RFC 0005) | 🟢 | 🟢 | 🟨 ¹⁵ | ❌ | 🟨 ¹ |
| Column type matches OTLP's key-to-value attributes | 🟨 ¹⁹ | 🟢 | 🟢 | 🟢 | 🟨 ²⁰ |
| Attribute equality search with index support | 🟢 ² | 🟢 ² | 🟨 ¹⁶ | 🟢 | 🟨 ³ |
| Single hot-key reads without a dedicated column | ❌ ⁴ | 🟨 ⁵ | 🟨 ⁵ ¹⁷ | ❌ ⁵ | 🟢 |
| Compression and TTL cost | 🟢 | 🟢 ⁶ | 🟨 ¹⁷ | 🟢 | 🟨 ⁷ |
| Merge behavior at hundreds of distinct keys | 🟢 | 🟢 ⁸ | 🟨 ¹⁷ | 🟢 | ❌ ⁹ |
| Reads tables written by the OTel exporter | ❌ | ❌ | ❌ | 🟢 | ❌ |
| Migration cost for existing deployments | 🟢 ¹⁰ | ❌ ¹¹ | ❌ ¹¹ | ❌ | ❌ |
| Follows ClickHouse's own current recommendation | 🟨 ¹² | 🟢 | 🟢 | 🟢 | ❌ |
| Ad hoc SQL for operators who expose ClickHouse directly | 🟨 ¹³ | 🟢 | 🟨 ¹⁸ | 🟢 | 🟢 |
| Planner behavior settled on the pinned releases | 🟢 | 🟨 ¹⁴ | 🟨 ¹⁴ | 🟨 ¹⁴ | 🟨 |

- ¹ Typed subcolumns answer ordered predicates once the path is typed; `JSON` has no bytes type and does not distinguish `1` from `1.0`.
- ² Through a text index on a `key=value` `ALIAS` array over each attribute group on 26.2 or later, the shape ClickStack ships today (§5.3).
- ³ A skip index needs a typed subcolumn declared per key; dynamically discovered paths cannot carry one.
- ⁴ Reads the whole typed group's arrays. In the internal benchmark of Addendum A, single-attribute filters, sorts and groups ran 1.3 s to 1.8 s on the arrays against 18 ms to 58 ms on a bucketed `Map` over the same rows. Hoisting (§5.2) applies to every column of this matrix and is the preferred answer for an attribute that is queried often; the row scores the keys that are not hoisted.
- ⁵ Only with the bucketed serialization, which is off by default and disables vertical merges. With only `map_serialization_version = 'with_buckets'` set, newly inserted parts stay `basic` and a merged map that averages under 32 keys gets one bucket, so a typed group of a few keys gains nothing; the benchmark of Addendum A set `map_serialization_version = 'with_buckets'` with a constant 32 buckets, `map_buckets_min_avg_size = 0`, and bucketing applied to inserted as well as merged parts, and measured 4x to 6x slower whole-row queries and 2.8x the insert CPU with them. A deployment enables it after measuring its own write budget. ClickStack's seed schema does not enable it, so the schema as shipped reads the whole `Map` like the arrays do, and a fork that adds the setting is no longer the ClickStack schema.
- ⁶ Measured at 0.4% larger than the arrays on the same spans with the same key type (Addendum A); the basic `Map` serialization is the same keys array, values array and offsets.
- ⁷ The `advanced` shared-data format keeps a second copy of overflow paths in map format, which the ClickHouse post says "effectively doubles the storage requirement" for that data ([JSON data type gets even better](https://clickhouse.com/blog/json-data-type-gets-even-better)).
- ⁸ With the basic serialization. `with_buckets` disables vertical merges, so a wide table that enables it pays horizontal merges over all columns.
- ⁹ Issue 89944: merges over 60 hours and TTL not applied at 388 paths and 700k spans/s on 25.8.8, still open (§2.3).
- ¹⁰ Codecs and settings are metadata-only `ALTER`s; `LowCardinality` rewrites parts; indexes are `ADD INDEX` plus `MATERIALIZE INDEX`.
- ¹¹ A new deployment pays nothing. An existing one rewrites its span tables or dual-writes for its retention window; the backend's schema is still changing under this RFC, so the cost is lowest now and grows with every month of retained data.
- ¹² ClickStack's documentation recommends "the default Map-based schema" for typical observability workloads ([Map vs JSON](https://clickhouse.com/docs/clickstack/ingesting-data/schema/map-vs-json)), and ClickHouse's schema guidance does not mention `Nested`; the recommendation is about mechanisms (text indexes, materialized hot columns, no attribute keys in the sort key), each of which applies to the arrays under `Nested`.
- ¹³ Jaeger's own query builder hides the SQL, and a deployment that keeps ClickHouse behind Jaeger's API never sees the difference. An operator who gives users or agents direct SQL access writes `value[indexOf(key, 'k')]` on `Nested` and `m['k']` on `Map`, and the ClickHouse documentation and ClickStack examples use the second form.
- ¹⁴ On 26.8 the basic `Map` measured slower than the arrays on top-N lists, whole-row queries, and attribute filters that select other attributes (Addendum A, §5.4). The benchmark attributes these to a second read of the `Map` and to lazy materialization, and [PR 124553](https://github.com/ClickHouse/ClickHouse/pull/124553) changes the `PREWHERE` and `m['key']` read paths in 26.11 and in the 26.3.63 and 26.8.37 backports. The `Map` read path is still moving, and a number measured today may not hold on the next LTS.
- ¹⁵ Each value keeps its OTLP type, and complex values keep their key-prefix encoding as strings. A typed predicate names the variant subcolumn (`m['k'].Int64 >= 400`); whether a skip index serves that form is unmeasured.
- ¹⁶ The `key=value` alias has to convert every value to a string, `concat(k, '=', toString(v))`, as §5.3 already does for the integer and boolean groups; whether the text index serves that alias over `Variant` values is unmeasured.
- ¹⁷ Not measured: Addendum A did not include this layout. The type tag adds one small column per map, and the fixed list of types avoids the dynamic-path merges of `JSON`.
- ¹⁸ `m['k']` returns a `Variant`, so a typed comparison or aggregation writes `m['k'].Int64` or `variantElement(m['k'], 'Int64')`.
- ¹⁹ A key and its value are paired only by array position, so every lookup reassembles the pair with `indexOf`.
- ²⁰ `JSON` reads `.` in a key as nesting, so OTLP's dotted keys need an escaping scheme in the writer (§3.2).

This RFC recommends the second option, typed `Map` per value type. The rows that decide it are typed predicates, which both the `Nested` and the typed `Map` layouts serve and the ClickStack and `JSON` schemas do not; the column type, which for a `Map` states what OTLP attributes are; ClickHouse's own direction, since its new index forms, functions and serializations name `Map` and none name `Nested` (§2.3); the opt-in bucketed read path, which gives a deployment table settings for single-key lookups on keys nobody chose to hoist, at the whole-row and insert costs of footnote ⁵; and ad hoc SQL for the operators who expose ClickHouse directly. On compression, merges and indexed equality search the two tie. The rows it loses are migration cost, which a new deployment does not pay and an existing one pays least now, and the maturity of the `Map` read path on the pinned releases (footnote ¹⁴); the read-path losses come from planner behavior on a code path ClickHouse is changing release by release, not from the type, so they decide when `map` becomes the default rather than whether. The benchmark also measured 34% more insert CPU for the basic `Map`, through a load path that was not Jaeger's writer; the retest measures it again and the configuration documentation reports it, and it does not gate the default. Until the retest passes, the backend offers the layout as the experimental `map` value of `attribute_layout`, and the retest in §5.4 decides when it becomes the default. The `Variant` map is not recommended: it would replace five columns per level with one, but none of its index or planner behavior is measured, and every typed predicate has to name the variant type. The ClickStack schema is rejected because its single `Map(String, String)` discards the value types Jaeger's query API requires, and the `JSON` schema because of the merge row and because ClickStack itself has withdrawn it.

---

## 5. Proposal

### 5.1 Adopt the storage tuning

Apply to `spans`, and the codecs and `LowCardinality` wrappers to the derived tables where the column exists:

- `CODEC(ZSTD(n))` on every column, and `CODEC(Delta(8), ZSTD(n))` on `start_time`, which the sort key orders to the second within each `(service_name, name)` run, so consecutive differences are small. Event timestamps are arrays with no ordering across rows, so they take plain `ZSTD(n)` as in ClickStack. The level is decided by benchmark B1 between 1 and 3: ClickStack and the exporter both moved to level 3 in November 2026 (§2.2) and Langfuse chose it for payload columns from the start, and level 3 costs CPU on insert that the benchmark has to put a number on.
- `LowCardinality(String)` on `service_name`, `name`, `kind`, `status_code`, `events.name`, and, one step past ClickStack's set, on `scope_name` and `scope_version`, because an instrumentation scope is a library name and version and a service carries a handful of them, and on the `key` member of every attribute `Nested` group (`Nested(key LowCardinality(String), value ...)`).
- `SETTINGS index_granularity = 8192, ttl_only_drop_parts = 1` on `spans` only. `trace_id_timestamps` has no partition key, so one of its parts can hold bounds from many days and whole-part expiry would keep expired trace IDs alive for as long as the newest row in the part; it keeps row-level TTL.

The factory only ever runs `CREATE TABLE IF NOT EXISTS`, so an existing table is untouched by the change and a new deployment gets all of it. For an existing table the three parts migrate differently: `ALTER TABLE ... MODIFY SETTING` is a metadata change, `MODIFY COLUMN ... CODEC(...)` is also metadata-only and takes effect on parts as background merges rewrite them, but `MODIFY COLUMN ... LowCardinality(String)` is a type change and ClickHouse rewrites every part that holds the column. The storage tuning therefore ships the DDL for new tables, and existing tables receive it through the schema versions that the [schema migrations RFC](./0022-clickhouse-schema-migrations.md) proposes, rather than through a hand-run `ALTER` sequence written for this change alone. The settings and codecs form a version that the binary applies at startup. The type change and the replacement of the trace-ID Bloom filter form a later version that the operator applies, because the first rewrites existing parts and the second has to be materialized over them. `ttl_only_drop_parts = 1` changes when data disappears: a row is removed only when its whole part has expired, so a day-partitioned table with a 7-day TTL retains up to one extra day. The trade is worth making, because the alternative is a merge that rewrites every expiring part, and the configuration documentation should say so.

**Tighten the trace-ID Bloom filter** to `bloom_filter(0.001)`, matching the exporter. A configurable rate, which [#8923](https://github.com/jaegertracing/jaeger/issues/8923) proposed before it was closed, is not part of this RFC; a deployment at [#8918](https://github.com/jaegertracing/jaeger/issues/8918)'s scale still edits the DDL to go tighter. The index name stays `idx_trace_id`, so on an existing table the change is `DROP INDEX` then `ADD INDEX` and `MATERIALIZE INDEX` for old parts. Benchmark B1 also measures a `text(tokenizer = 'array')` index on `trace_id`, the shape the ClickStack seed schema uses in place of both the Bloom filter and the lookup table (§2.2), against the tightened Bloom filter plus `trace_id_timestamps`. If the text index alone prunes trace ID lookups well enough, the lookup table becomes optional; that is a finding for the benchmark to produce, not a change this RFC proposes.

### 5.2 Make typed attribute hoisting configurable

A deployment can select frequently searched attributes to copy into dedicated `MATERIALIZED` columns on `spans`. This provides direct column access for those keys while keeping the typed attribute groups as the source of truth. Hoisting does not depend on the storage tuning (§5.1), the attribute index (§5.3) or the `map` default (§5.4), which benchmarks B1, B3 and B4 decide separately.

Hoisting is independent of the attribute container. A materialized column computed from a key lookup works the same over a `Nested` group and over a typed `Map` column; only the extraction expression differs. What the container decides is the cost of the keys that are not hoisted. Hoisting is also not free in operation: the operator chooses the keys, every change is an `ALTER TABLE ... ADD COLUMN ... MATERIALIZED` followed by a `MATERIALIZE COLUMN` mutation over existing parts, and a deployment that serves several teams or projects has no single hot set. One project filters on `gen_ai.request.model`, another on a session identifier, a third on a score, and the table either carries all of their columns or serves most of them from the attribute groups. Hoisting therefore stays the preferred answer for the attributes a deployment queries most, and the bucketed `Map` serialization of §5.4 is the candidate for the long tail, since it serves every key the same way with no per-key operation, at the insert cost that benchmark B5 measures.

**Configuration and types.** One optional `hoisted_attributes` list on the ClickHouse backend defines the source and destination of each copy. The same definition drives schema creation, schema validation, and query routing; there is no second reader-only mapping. An empty list preserves today's schema and queries.

```yaml
hoisted_attributes:
  - key: gen_ai.output
    level: span
    type: string
    column: gen_ai_output
    codec: ZSTD(3)
  - key: gen_ai.usage.input_tokens
    level: span
    type: int
    column: input_tokens
```

The initial levels are `span` and `resource`. The type names identify the OTLP source type, not a request to cast the attribute into a different type:

| Configured type | Source value group | Hoisted column type |
| --- | --- | --- |
| `string` | `str_attributes` | `Nullable(String)` |
| `int` | `int_attributes` | `Nullable(Int64)` |
| `double` | `double_attributes` | `Nullable(Float64)` |
| `bool` | `bool_attributes` | `Nullable(Bool)` |

Resource-level sources use the corresponding `resource_` groups. Each expression searches the group's paired key/value arrays and returns the value only when the configured key exists in that type group; otherwise it returns `NULL`. There is no conversion of strings to numbers, no narrowing of an OTLP integer to `UInt32`, and no replacement of missing values with zero, false, or an empty string. A key seen at multiple types can have a separate hoist for each type.

`key`, `level`, `type`, and `column` are required. Reject unsupported levels and types, empty keys, duplicate `(level, key, type)` entries, and duplicate destination names. Destination names must match `[a-z][a-z0-9_]*` and must not collide with existing schema columns; a leading underscore is not accepted, because ClickHouse refuses names of its own virtual columns such as `_row_exists`. Attribute keys remain opaque strings and are escaped as SQL literals rather than treated as identifiers or SQL expressions.

The optional `codec` defaults to `ZSTD(1)`. The storage tuning does not change that default, because startup compares a hoisted column's stored codec with the configured one and a changed default would refuse every deployment that omitted `codec`. Initially accept only `ZSTD(level)` with an integer level from 1 through 22, validated and rendered by the backend. This permits `ZSTD(3)` for a large payload without accepting arbitrary SQL or introducing codec overrides for every existing column. Hoisted columns do not implicitly gain `LowCardinality` or a secondary index.

**Writes and reads.** ClickHouse computes the materialized columns from the original attributes on insert. The writer already names its inserted columns explicitly, so it continues writing the original groups. Those groups also remain the source for trace reconstruction and `attribute_metadata`; hoisting copies an attribute rather than moving it or changing its recorded type.

Both the legacy tag-search builder and the structured-filter builder use a shared lookup by `(level, key, type)`. An equality branch with a configured hoist compares the dedicated column; every other branch retains its existing array predicate. Existing metadata resolution remains authoritative for untyped values, and an explicitly typed filter still matches only its declared type. Hoisting one occurrence of a key does not suppress its other types or levels: legacy searches retain their current level coverage, and unqualified structured references still search span and resource attributes. Missing values must make an equality false before boolean composition, including `NOT`, so nullable columns preserve the existing filter semantics. Unit and integration tests must demonstrate this behavior rather than relying on SQL's default null propagation. Ordered attribute predicates and new search capabilities are outside this milestone.

**Schema lifecycle and indexes.** With `create_schema: true`, the `spans` creation template includes the configured materialized columns. With `create_schema: false`, operators create them themselves. In either mode, startup validates the configured columns' types, materialized expressions, and codec declarations before enabling query routing; a missing or incompatible column is an actionable startup error. This check is limited to configured hoists, not general schema reconciliation. `CREATE TABLE IF NOT EXISTS` cannot add columns to an existing table, so the documentation provides a hand-run `ALTER TABLE ... ADD COLUMN ... MATERIALIZED ...` sequence before enabling the configuration. It also describes operator-controlled `MATERIALIZE COLUMN` mutations for existing parts and how to wait for them in `system.mutations`; startup never launches a historical backfill. Reads of old parts must retain correct extraction semantics before materialization, with their extra computation cost included in the migration benchmark. Removing a mapping restores the existing array predicates and leaves the physical column in place; changing a mapping requires an explicit schema migration.

Secondary indexes on these columns are operator-managed. Jaeger neither creates nor materializes them through `hoisted_attributes`; this is separate from §5.1's tuning of the existing trace-ID index. Operators choose Bloom, `minmax`, or another supported index based on their workload, and materialize indexes for historical parts when needed. A column without an index can reduce attribute-array reads but does not guarantee granule pruning. An unhoisted or unindexed branch in a cross-level or mixed-type `OR` can also prevent pruning of the combined predicate. Hoisting alone does not resolve [#8715](https://github.com/jaegertracing/jaeger/issues/8715)'s request for general attribute indexes.

**Scope and acceptance.** The first implementation supports direct extraction of the four scalar types at the span and resource levels. Scope, event, and link attributes, bytes, maps, slices, arbitrary SQL, fallback chains, JSON extraction, and derived metrics maps are outside this milestone. The shared storage integration corpus verifies identical search and round-trip results with and without hoisting in both direct and e2e modes, including missing versus default values, mixed types, equal keys at different levels, untyped filters, and negation. ClickHouse-specific tests cover configuration validation, schema rendering and compatibility, removal of mappings, and historical parts before and after materialization. Those tests are the delivered acceptance. Benchmark B2 (§7.2), which runs after delivery, reports search latency and rows/bytes read with and without operator-created indexes, confirms actual index use with `EXPLAIN indexes = 1`, and measures insert throughput, total compressed storage including the original attributes, and materialization time. Large payloads include a comparison of `ZSTD(1)` and `ZSTD(3)` so codec selection accounts for CPU and the cost of storing a second copy.

### 5.3 Index the attribute pair: a text index on `key=value` items, the hashed pair filter as fallback

Issue [#8715](https://github.com/jaegertracing/jaeger/issues/8715) asks for Bloom filter skip indexes on the attribute columns in the exporter's separate key and value shape. That shape is not adopted, because it cannot prune a common key with a common value (§3.5) and ClickHouse's own guidance now advises against copying it. The section first sets out what each candidate can answer, then the decision, then the two pairwise shapes in enough detail to build them.

**What an attribute index can and cannot answer.** A Bloom filter or a text index over the items of a shared attribute group answers one question per granule: might this granule contain item X. Over `key=value` items it serves equality and `IN` and nothing else. Ordered predicates on typed values (RFC 0005's `http.status_code >= 400`) and aggregations over an attribute (`GROUP BY` a key's values) need the key's values addressable as a column, which no index over a shared attribute group provides. The shapes on offer differ in which equalities they prune, in what they cost, and in whether they open a path to the rest. Legend: 🟢 good · 🟨 partial · ❌ not served.

| Criterion | Key filter + value filter (exporter) | Pair text index on a `key=value` alias (ClickStack) | Pair filter, hashed `bloom_filter` | Per-key `JSON` subcolumn | Inverted `(key, value)` table |
| --- | --- | --- | --- | --- | --- |
| Equality, rare key | 🟢 | 🟢 | 🟢 | 🟢 | 🟢 |
| Equality, common key and common value | ❌ ¹ | 🟢 | 🟢 | 🟨 ² | 🟢 |
| Ordered predicate on a typed value | ❌ | ❌ | ❌ | 🟨 ³ | 🟢 |
| Aggregation over a key's values | ❌ | ❌ | ❌ | 🟢 ⁴ | 🟨 ⁵ |
| Extra storage | 🟨 ⁶ | ❌ ⁷ | 🟨 ⁸ | 🟨 ⁹ | ❌ ¹⁰ |
| Served on both tested LTS lines | 🟢 | 🟢 ¹¹ | 🟢 | 🟨 | 🟢 |
| Fits the typed attribute groups | 🟢 | 🟢 | 🟢 | ❌ ¹² | 🟨 ¹³ |

- ¹ Both filters pass a granule that has the key on one row and the value on another; for `http.status_code=500` that is every granule.
- ² A typed `JSON` subcolumn is a plain column, so a Bloom filter on that one path works, but only when the path is typed and the index is declared per key; a dynamically discovered path cannot carry one.
- ³ Without an index, an ordered predicate on `attrs.key.:Int64` is a vectorized scan of that one path's data, far less than the attribute arrays but still a scan. `minmax` pruning, the flat-column indexing ADR-008's limitations section wishes for, requires a per-key index on the typed subcolumn and a predicate that names it; on 25.12 the same predicate written against the untyped path read every granule.
- ⁴ `GROUP BY attrs.key.:String` scans one path's subcolumn; it needs the type, not an index.
- ⁵ A `GROUP BY` over one key is a range scan of that key's slice of the table; joining back to spans for anything else is a second query.
- ⁶ Two Bloom filters per attribute group, each over every key or every value.
- ⁷ A text index stores a posting list per token. The GA post's 216 GiB against 7 GiB is for a free-text column, not a `key=value` array (§3.5); ClickStack reports insert overhead "broadly comparable" to Bloom filters on its own tables. Benchmark B3 measures it on Jaeger's data.
- ⁸ The alias is never stored, but each Bloom filter is, and a Bloom filter is incompressible by construction: its size is fixed by element count and target rate, and its bits are as random as the hashes that set them. The size of fifteen such filters relative to the table is a number for benchmark B3.
- ⁹ Dynamic paths above the `max_dynamic_paths` limit fall back to a shared column, and the type's per-path bookkeeping costs space.
- ¹⁰ One row per attribute pair per span, sorted independently of `spans`: a second copy of every attribute.
- ¹¹ On 25.12.11 a `has()` predicate on a string `ALIAS` was not served by any skip index, which is the limitation the hashed shape was designed around. The text index serves a string alias from 26.2, so on both tested LTS lines; `direct_read` needs 26.5 or a patch release of 26.2 through 26.4 that carries it.
- ¹² Replaces the five typed groups and the query builder over them; `attribute_metadata` stays as the source of each path's type. A superseding RFC.
- ¹³ Adds a table and a materialized view beside the existing layout, and the reader has to join.

**The decision.** Two pairwise shapes survive the matrix, and they differ only in cost and version floor. The text index on a `key=value` alias is the shape ClickStack ships, and it has no false positives for keys that contain no `=`. Its `direct_read` path does not help Jaeger, because the exact check that follows the prefilter reads the attribute columns anyway, so its gain is granule pruning. The hashed `bloom_filter` was measured as served on 25.12.11, is expected to cost less on disk than the text index (benchmark B3 measures both on Jaeger's data), and admits hash collisions that the exact check below absorbs. This RFC makes the text index the primary shape and keeps the hashed filter as the fallback for a deployment whose index storage or insert budget benchmark B3 shows the text index exceeding, or whose server predates 26.2. The two shapes differ in the alias expression (a `key=value` string or a hash), the index clause and the prefilter, and share everything else below. The `JSON` subcolumn is no longer a candidate for the attribute index: it serves ordered predicates and aggregation that the pairwise shapes do not, but it replaces the attribute layout to do so, ClickStack has withdrawn it, and its merge behavior at scale is an open production report (§2.3); §5.4 keeps it as a measurement. The inverted table is rejected: it stores a second copy of every attribute to serve two of the three query rows. Ordered predicates and aggregation over attributes stay outside this RFC, served for chosen keys by hoisting (§5.2) and for the rest by the scan they get today.

**The primary shape: a text index on a `key=value` alias.** For each string-attribute group, add an `ALIAS` column that joins each pair into one item, and a text index with the `array` tokenizer on it, which is the ClickStack seed schema's `SpanAttributeItems` shape over Jaeger's arrays:

```sql
ALTER TABLE spans
    ADD COLUMN str_attribute_items Array(String)
        ALIAS arrayMap((k, v) -> concat(k, '=', v), str_attributes.key, str_attributes.value),
    ADD INDEX idx_str_attribute_items str_attribute_items TYPE text(tokenizer = 'array');
```

The query builder's prefilter is `has(str_attribute_items, concat(?, '=', ?))`, spelled out as the `arrayMap` expression for the reason given under the hashed shape below. The `=` separator is ambiguous for a key that contains `=`, which OTLP permits; the exact `arrayExists` check that follows the prefilter makes the ambiguity a pruning loss and never a correctness one, and benchmark B3 checks with `EXPLAIN indexes = 1` on 26.8 that the inline expression is matched to the index as the alias name is. Everything that follows about levels, the `OR` rule, the typed groups, the migration sequence and acceptance applies to this shape unchanged, with `concat(k, '=', toString(v))` in place of `cityHash64(k, toString(v))` for the integer and boolean groups.

**The fallback: a hashed pairwise `bloom_filter`.** What follows is the complete design, so that a negative B3 measurement of the text index's cost leads straight to implementation.

**Index the pair, not the key and value separately.** For each string-attribute group, add an `ALIAS` column that hashes each pair to one 64-bit value, and a `bloom_filter` on it:

```sql
ALTER TABLE spans
    ADD COLUMN str_attribute_hashes Array(UInt64)
        ALIAS arrayMap((k, v) -> cityHash64(k, v), str_attributes.key, str_attributes.value),
    ADD INDEX idx_str_attribute_hashes str_attribute_hashes TYPE bloom_filter(0.01) GRANULARITY 4;
```

and likewise for the resource and scope groups, and for the event and link groups through `arrayFlatten` over the nested arrays. `bloom_filter` on an `Array` column indexes each element and is served by `has()`. The shape is ClickStack's pairwise item index with two changes.

- **The item is a hash, not a `key=value` string.** `cityHash64(k, v)` hashes the two arguments as a tuple, so there is no separator to collide on, and the alias itself is never written to disk (the Bloom filter built from it is; footnote ⁸ above). A string alias under a `bloom_filter` is not served: on 25.12.11.4, `EXPLAIN indexes = 1` over a 200k-row test table showed `has()` on a `concat(k, '=', v)` `ALIAS` column reading all 25 granules with the index never consulted, while the same predicate on the hashed `ALIAS` column consulted the index and read 1 of 25. The text index of the primary shape is what lifted that limitation for strings from 26.2; the `bloom_filter` type still needs the hash. A `MATERIALIZED` string column is also served, at the cost of storing every pair twice, which the hash makes unnecessary. The check holds with the `LowCardinality(String)` keys that §5.1 introduces: `cityHash64` of a `LowCardinality(String)` equals `cityHash64` of the same `String`, and on a table with `LowCardinality` keys the inline predicate read the same 1 of 25 granules. Benchmark B3 repeats this check on the benchmark table before anything else, since the whole design rests on it.
- **Every level is indexed, including events and links.** A skip index can prune a granule only when the whole `WHERE` clause is provably false for it, and an `OR` is provably false only when every branch is. The query builder's fallback for a key that `attribute_metadata` has not seen ORs five branches, one per level, and the metadata path ORs one branch per observed `(level, type)` pair whose type can parse the value. One unindexed branch in that `OR` therefore disables pruning for the entire attribute predicate, which is exactly the attribute-only benchmark case. So the hash columns and indexes cover all five levels, and the nested `arrayExists` for events and links gains the same `has()` prefilter.

`GRANULARITY 4` (one filter per four granules) is a starting point rather than a measurement: every row contributes every attribute pair, so a per-granule filter at 0.01 is large, and coarser granularity trades pruning resolution for index size. Benchmark B3 measures 1 against 4.

The string, integer, and boolean groups are indexed, the last two through `cityHash64(k, toString(v))`. Strings are where the UI's filters land, since every value the search box sends arrives as a string and `attribute_metadata` resolves it to a typed column only when the key has been seen with that type. Integers are included for two reasons: they carry high-cardinality values (request IDs, ports, user IDs), and a key that `attribute_metadata` has seen as both a string and an integer produces an `OR` of both typed branches, which the `OR` rule above turns into a full scan unless both branches are indexed. An `OR` whose branches are all indexed does prune: on 25.12.11.4 two `has()` branches on two indexed columns read 2 of 25 granules through the planner's combined skip indexes. Booleans ride along for the same mixed-type reason and because the indexed item is the pair, so a rare pair such as `error=true` is exactly what a Bloom filter prunes well. Double and complex groups stay unindexed, doubles because equality on a floating-point value is fragile and a range is not Bloom-shaped, and complex values because the search box offers no way to write a serialized map or slice, so a key whose metadata includes either type keeps today's scan; benchmark B3 includes such a mixed-type key so that this limit is measured. Range predicates on attributes ([RFC 0005](0005-structured-query-filters.md)) are not Bloom-shaped and are out of scope here.

**Change the predicate the query builder emits** for every indexed group at every level. For the string groups it goes from

```sql
arrayExists((key, value) -> key = ? AND value = ?, s.str_attributes.key, s.str_attributes.value)
```

to

```sql
has(arrayMap((k, v) -> cityHash64(k, v), s.str_attributes.key, s.str_attributes.value), cityHash64(?, ?))
AND arrayExists((key, value) -> key = ? AND value = ?, s.str_attributes.key, s.str_attributes.value)
```

The predicate spells out the hash expression instead of naming the alias, so the same query runs against a table that predates the alias (an upgraded deployment that has not run the `ALTER` sequence yet) and simply scans as it does today, while on a table that has the alias and its index the planner matches the expression to the index and prunes. On 25.12.11.4 the inline form read the same 1 of 25 granules as the alias name did, with and without the trailing `arrayExists`. The `has()` is the index-driving prefilter and the `arrayExists` stays as the exact check, because both the hash and the Bloom filter admit collisions. On every granule the prefilter admits, each row now computes the alias and `has()` in addition to the array scan it runs today, so the change wins only when the index drops enough granules to pay for that. Benchmark B3 therefore includes a low-selectivity attribute (a key present on most spans with few distinct values) alongside the high-selectivity case, so the worst case is measured rather than assumed. The integer and boolean branches gain the same prefilter, with the bound value formatted on the query side as `cityHash64(?, toString(?))` for integers and `cityHash64(?, toString(CAST(? AS Bool)))` for booleans. Both sides go through `toString` so that the hash depends only on the decimal text and not on how the driver types the bound parameter; on 25.12 `cityHash64` happens to hash an integer literal and an `Int64` alike, but that is an implementation property rather than a documented one. The boolean cast is mandatory rather than defensive: the driver binds a Go boolean as `1` or `0` while `toString` of a stored `Bool` yields `true` or `false`, and an unnormalized bound value would hash to a different item and silently reject every match. The typed `arrayExists` stays as their exact check, and the correctness tests include driver-bound `true` and `false` filters. A key seen only as string, integer, or boolean thus emits only prefiltered branches, which is what the `OR` rule above requires; a key also seen as double or complex keeps an unprefiltered branch and today's scan, as the previous paragraph says. For events and links the exact check is the existing doubly nested `arrayExists` and the prefilter is `has()` on the flattened hash column.

On an existing table `ADD INDEX` covers only parts written afterwards, so the migration sequence follows it with `MATERIALIZE INDEX` and waits for the mutation to finish in `system.mutations`; until then historical parts are scanned as before, and a before/after benchmark that skips this step measures nothing. The alias itself needs no backfill.

Acceptance for either shape is benchmark B3 (§7.2), which [#8715](https://github.com/jaegertracing/jaeger/issues/8715) asks for: attribute-only search on the 10M-span dataset before and after, on the `nested` and `map` layouts, with `EXPLAIN indexes = 1` output showing granules dropped. Because fifteen indexes (three types across five levels) each process every attribute pair on insert, the benchmark also reports insert throughput, the on-disk size of the indexes relative to the table, and the wall time of `MATERIALIZE INDEX` over the existing data, so that the write-side cost is weighed against the read-side gain. The benchmark runs both shapes, and the text index's storage and insert cost relative to the hashed filter is the number that decides whether the fallback is ever built.

### 5.4 Adopt typed `Map` columns for the attribute groups, as the default once a retest passes

ADR-008's typed groups stand, for the type-fidelity reasons §3.2 adds to the ADR's own, and `Map(String, String)` is rejected outright. The container of each group is what this section changes. §4.2 recommends one `Map(String, T)` column per value type in place of each `Nested(key String, value T)` group, at every level (span, resource, scope, event and link), with the basic serialization by default and `map_serialization_version = 'with_buckets'` as a table setting a deployment may enable on a release that carries [ClickHouse PR 109178](https://github.com/ClickHouse/ClickHouse/pull/109178), since bucketed parts written before it lose the order of their keys. The backend offers both containers through the `attribute_layout` setting ([backend README](../../internal/storage/v2/clickhouse/README.md#attribute-layout)): `nested`, the default, and `map`, which is experimental until the retest below shows that the 26.8 read-path gaps are closed. On disk the basic `Map` is the same keys array, values array and offsets as the `Nested` group, so the mechanisms of §5.1 to §5.3 are expected to carry over unchanged, and benchmark B3 verifies the index part: `LowCardinality` on the key, the codecs, the `key=value` alias and its index declared over `mapKeys(m)` and `mapValues(m)` instead of `g.key` and `g.value`, through an `arrayMap` over the event and link arrays, whose attributes are `Array(Map(String, T))`. What the change buys is stated in §4.2. What it costs is a reader and writer change, a new `MATERIALIZED` expression for hoisted columns (§5.2), a schema migration for existing deployments, insert CPU that Addendum A measured 34% higher, and the planner-maturity risk that the retest below measures. The `complex_attributes` encoding (JSON for maps and slices, base64 for bytes, under the `@bytes@`/`@map@`/`@slice@` key prefix) and the `attribute_metadata` table are unchanged by the container.

**The retest.** The internal benchmark of Addendum A measured the basic `Map` slower than the arrays on three query classes on ClickHouse 26.8: top-N lists keyed on one attribute, whole-row queries, and queries that filter on one attribute and select others. Jaeger's `FindSpans` issues the second and third shapes in one statement, an attribute filter with whole span rows under `ORDER BY ... LIMIT`; `FindTraces` searches trace IDs and fetches the spans of those traces in separate statements. The benchmark attributes the losses to two causes. The first is a second read of the `Map`: the top-N shapes decompressed about three times the bytes of the arrays and the filter-plus-select shapes up to 1.7 times, and on the two filter-plus-select shapes where it was tried, `optimize_move_to_prewhere = 0` brought the decompressed bytes back to the arrays' level; the top-N shapes were not tried. [PR 124553](https://github.com/ClickHouse/ClickHouse/pull/124553) groups the `PREWHERE` read steps and restricts the `m['key']` subcolumn rewrite to bucketed tables. The second, a hypothesis that no toggle isolated, is lazy materialization: the benchmark reported that the whole-row times of the `Map` lined up roughly with the arrays' times with lazy materialization switched off, although its two figures differ from those times by −27% and +61% (Addendum A). No ClickHouse change addressing it is known. The same PR also removes the key subcolumn that made the basic `Map` faster on queries that read several keys of one row, so the benchmark's wins do not predict post-fix behavior either.

Benchmark B4 (§7.2) runs the backend's `nested` layout against its `map` layout on the 10M-span benchmark dataset, on each LTS line that Jaeger's CI pins when the retest runs. On each line it uses the "candidate release", the newest pinned release that carries the backport with no open regression reported against it, and, where one is supported, a release from before the backport as the baseline. It runs the queries Jaeger's reader issues, `FindTraces`, `FindTraceIDs` and `FindSpans` with and without attribute filters and `GetTraces`, the top-N, whole-row, filter-plus-select and multi-key shapes of Addendum A, and the storage suites as a correctness check. Both the query cache and the query condition cache are off, and the effective settings are recorded. It records decompressed bytes, peak memory and `EXPLAIN` plans including `EXPLAIN indexes = 1` for the attribute index that benchmark B3 selects, on both layouts, and runs with `optimize_move_to_prewhere` and lazy materialization each switched off, so that the two causes are told apart. Insert CPU is measured through Jaeger's writer and reported in the configuration documentation; it does not gate the default.

Every shape must pass on every candidate release. A shape passes when its `map` median over six runs in rotated order exceeds the `nested` median by no more than the noise floor, the spread between two `nested` tables on the same hardware capped at 10%, and when its `map` peak memory is no more than twice the `nested` peak. Storage passes when the `map` tables are no more than 5% larger. The baseline release does not gate: a regression that reproduces on it and is absent on the candidate shows that the backport fixed it, and a regression the dataset reproduces on no release is retested on a dataset that does before the default changes.

When every shape passes, `map` becomes the default for new tables in the Jaeger release that pins the candidate releases. The configuration documentation names the minimum release on each LTS line for the `map` layout, and a deployment on an older server sets `attribute_layout: nested`; the backend does not check the server version, which is the cost of keeping one default. An existing deployment keeps its layout: the same change makes the backend read the layout of the existing tables when `attribute_layout` is unset. `nested` stays supported. A gap that survives keeps `nested` the default until a ClickHouse release closes it, and this section records the measured reason. Benchmark B5 measures hoisted columns against a typed bucketed `Map` for the unhoisted keys, on a dataset wide enough to fill the buckets, with the bucket settings of Addendum A and with the defaults, and with Jaeger's `mapContains(g, k) AND g[k] = v` predicate as well as `g[k]` alone, verifying the serialization of the physical parts, since the 10M-span dataset's 11 attributes per span stay below the 32-key average at which the default settings start splitting, and measures the insert CPU of the bucketed serialization through Jaeger's writer, which tells a deployment when the bucketed setting is worth that cost (Addendum A measured 2.8x); that result informs the configuration documentation and does not gate the layout.

**Migration.** The layout belongs to the table, and the backend does not convert spans between layouts; the backend README states the operational rules. A new deployment creates the `Map` tables and pays nothing. An existing deployment creates the new tables in a new database, and spans in the old tables are not searched from the new ones: it either accepts the gap in search history or writes to both databases for its retention window and switches the reader when the old tables have expired. The backend does not convert spans in place; an operator who wants the old spans in the new tables copies them with `INSERT ... SELECT`. The cost is lowest now, while the backend's schema is still changing under this RFC, and grows with every month of retained data (§4.2 footnote ¹¹).

**The `JSON` measurement, benchmark B6.** The `JSON` type is no longer a candidate to replace the layout under this RFC: the exporter's variant is still experimental, ClickStack has withdrawn it to a beta option for small, stable key sets, and an open production report shows merges running over 60 hours at 388 paths and 700k spans/s (§2.3). It stays as a measurement because it is the only type that would serve ordered predicates and aggregation without hoisting, and because the question of whether its merge behavior fails at a representative path count should be answered by a number rather than by one report. Benchmark B6 has to settle two design questions before it measures anything: a collision-safe key encoding, since the `JSON` type reads `.` as nesting and rejects a map that holds both `a.b` and `a` → `b`, and the server's `json_type_escape_dots_in_keys` is not reversible for a key that contains a literal `%2E`; the writer escapes `%` first and then `.`, the server setting stays off, the reader inverts the mapping, and the correctness gate includes a dotted key, its nested-object twin, and a key containing a literal `%2E` in the same map, with predicates and type hints written against the escaped form shown to be served; and how the query builder learns each key's type so it can name the typed subcolumn (`attrs.key.:Int64`), because an index can be declared only on a typed path and a predicate against the untyped path is not served by it. The natural answer to the second is the existing `attribute_metadata` table, so benchmark B6 should assume it stays. The pass criteria are then compression no worse than the array layout with the storage tuning of §5.1; round-trip and equality-filter correctness for all seven `pcommon` value types (bytes and integral doubles need the `@bytes@`-style tagging §3.2 describes) and for dotted keys, including a map that holds both a dotted key and its nested-object twin; attribute search at least as fast as the pair index's measured result on the same data, both with and without per-key skip indexes on the typed subcolumns, so the operational cost of declaring them per key is visible; an ordered predicate on a typed subcolumn pruning granules where the pair index cannot; trace retrieval by ID with a trace-ID index added back, since the exporter's JSON template has none; and merge duration, parts count and TTL expiry under sustained insert at a distinct-path count of at least 400, above the 388 paths of the production report and well beyond what the 10M-span load-once benchmark exercises. A pass on every criterion is the evidence a future superseding RFC would need; a fail on the merge criterion closes the question.

### 5.5 Make the table engine configurable

A `table_engine` block on the backend selects the engine family for every table the factory creates, and the factory substitutes it into every `ENGINE =` clause. The exporter's option is a literal engine name because the exporter has one table per signal; Jaeger has `MergeTree` and `AggregatingMergeTree` tables from the same DDL set, so the block names the family only, and the factory composes `ReplicatedMergeTree(...)` for `spans` and `dependencies` and `ReplicatedAggregatingMergeTree(...)` for the four derived tables. The variant is selected by key, with its parameters beneath it, which is how the backend already selects `auth: { basic: {...} }`:

```yaml
table_engine:
  replicated:
    # Optional together: omitted, the engine is rendered without arguments and the
    # server's default_replica_path and default_replica_name apply. One template
    # serves all six tables, so keeper_path must vary per table via {table}.
    keeper_path: /clickhouse/tables/{shard}/{database}/{table}
    replica_name: "{replica}"
```

The block is required when `create_schema` is true, and the backend refuses to start without it, recommending `merge_tree` for a single node and `replicated` for a cluster with more than one replica. A default of `merge_tree` was rejected: a local table on a two-replica cluster accepts every insert and holds only the rows its own server received, so the failure is silent until a trace is "not found" on one replica and present on the other, and an operator who has to choose the engine once is the cheapest point at which to make the single-node assumption visible. `create_schema: false` ignores the block. `ON CLUSTER` DDL is outside this RFC. [#9886](https://github.com/jaegertracing/jaeger/issues/9886) adds it with a `cluster` block under `table_engine`, together with the `Distributed` tables of a sharded cluster, and so reverses the earlier position here that a deployment needing cluster DDL should own its schema with `create_schema: false`. The comparison of this shape against a per-table map and against raw engine clauses is recorded on [#9812](https://github.com/jaegertracing/jaeger/issues/9812).

### 5.6 Do not change what is not broken

Column names stay `snake_case`, `kind` stays lowercase, `duration` stays `Int64`, scope attributes stay, and the five derived tables stay. Each is either shared with Jaeger's other backends or a recorded decision, and none of the ClickStack differences in those areas is an improvement.

---

## 6. Reading ClickStack Tables Directly

The question this section answers is whether Jaeger can point at an existing `otel_traces` table. The comparison makes the answer concrete.

**What is the same** is what matters most: the partition key and the sort key. Every query shape Jaeger's reader issues (search narrowed by service, name, and time; trace retrieval by ID with time hints) has an efficient equivalent against `otel_traces`, through the Bloom filter and bounds table on an exporter-created table or through the text index on a ClickStack seed table, which has no bounds table (§2.2); an adapter would have to serve both.

**What differs** is mechanical: column names, `kind` casing, `UInt64` duration, `DateTime` seconds in the exporter's bounds table, and the absence of scope attributes and of the `services`, `operations`, and `attribute_metadata` tables. Services and operations become `DISTINCT` queries on `LowCardinality` columns, which is what HyperDX does. Attribute metadata is unnecessary because there is exactly one type: string. That means the read adapter would accept only string attribute filters, which is exactly what the Jaeger UI sends today and exactly what a ClickStack user already lives with.

**What is lost** is type fidelity on the way out: every attribute comes back as a string, so a trace written by the exporter and read by Jaeger shows `http.status_code: "200"` where the SDK emitted an integer. This is a property of the data on disk, not of the adapter.

A read-only adapter is therefore feasible as a second `dbmodel` and a second set of query templates behind the same `tracestore.Reader` interface, selected by configuration. It is not proposed in this RFC because it is a feature with its own scope (a writer is neither needed nor wanted, since the exporter is the writer), its own compatibility surface (the exporter's schema is versioned by the exporter), and its own tests. It is not planned.

---

## 7. Implementation Plan

The options that ship as configuration are built, and the changes to the default schema wait on a benchmark. §7.1 lists what is delivered, and §7.2 lists the benchmarks in the order they run, each with the decision it informs. Every benchmark runs on the [documented setup](../../internal/storage/v2/clickhouse/BENCHMARKING.md) unless it says otherwise. The storage tuning of §5.1, the attribute index of §5.3 and the `map` default of §5.4 are proposals until their benchmarks run, and each is implemented once its benchmark supports it. The storage tuning, the attribute index and the `JSON` measurement, which this plan first numbered M1 to M3, are now benchmarks B1, B3 and B6.

### 7.1 Delivered

- ✅ **M4 — Configurable table engine (§5.5).** The `table_engine` option and its rendering into every DDL statement. Delivered in [#9813](https://github.com/jaegertracing/jaeger/pull/9813), which requires the block with `create_schema: true` and runs the direct storage integration suite against the replicated engines on an embedded Keeper.
- ✅ **M5 — Configurable typed attribute hoisting (§5.2).** The configuration, materialized columns, schema validation, query routing and migration procedure for selected span and resource attributes. Delivered in [#10109](https://github.com/jaegertracing/jaeger/pull/10109) (configuration and columns), [#10110](https://github.com/jaegertracing/jaeger/pull/10110) (schema validation), [#10111](https://github.com/jaegertracing/jaeger/pull/10111) (query routing) and [#10112](https://github.com/jaegertracing/jaeger/pull/10112) (storage suites and migration procedure). Benchmark B2 measures its performance.
- ✅ **M6 — Typed `Map` attribute layout (§5.4).** The `attribute_layout` setting, with `nested` as the default and `map` as an experimental opt-in. Delivered in [#10162](https://github.com/jaegertracing/jaeger/pull/10162) (writer), [#10163](https://github.com/jaegertracing/jaeger/pull/10163) (reader and search), [#10164](https://github.com/jaegertracing/jaeger/pull/10164) (setting, schema and startup check) and [#10165](https://github.com/jaegertracing/jaeger/pull/10165) (storage suites against a `Map` table with hoisted columns). Benchmark B4 decides whether `map` becomes the default.

### 7.2 Benchmark roadmap

B1 runs first because the settings it selects are the baseline for the others. B3 runs before B4 so that the retest runs both layouts with the attribute index that B3 selects.

- **B1 — Storage tuning (§5.1).** Compressed size and insert throughput of the codecs, `LowCardinality` and table settings against the current DDL, with ZSTD levels 1 and 3, and trace retrieval with a text index on `trace_id` against the Bloom filter tightened to 0.001 plus `trace_id_timestamps`. It decides the ZSTD level and whether `trace_id_timestamps` can become optional.
- **B2 — Hoisting (§5.2).** The benchmarks that §5.2 lists under "Scope and acceptance". It informs the configuration documentation; hoisting stays opt-in whatever the result.
- **B3 — Attribute index (§5.3).** The acceptance benchmark that §5.3 defines, with no index, the text index on `key=value` items and the hashed `bloom_filter`, on both layouts. It decides whether an attribute index ships and which shape; the index that ships closes [#8715](https://github.com/jaegertracing/jaeger/issues/8715).
- **B4 — `map` against `nested` (§5.4).** The retest that §5.4 defines, with its query shapes, releases and pass criteria. A pass makes `map` the default for new tables and gives the superseding ADR described below its evidence; a surviving gap keeps `nested` the default until a ClickHouse release closes it.
- **B5 — Bucketed `Map` against hoisting (§5.4).** Hoisted columns against a typed bucketed `Map` for the unhoisted keys, with the insert CPU of the bucketed serialization, as §5.4 describes. It needs a dataset with more attributes per span than the documented setup's, so that the buckets fill. It tells a deployment when the bucketed setting is worth that cost, and informs the configuration documentation only.
- **B6 — `JSON` attributes (§5.4).** A benchmark-only branch storing attributes as `JSON` columns on the release the storage integration tests pin (`docker-compose/clickhouse/`), with the pass criteria of §5.4. Its merge criterion needs sustained insert at 400 or more distinct paths, which the documented setup's load-once dataset does not provide. Its result is a finding for a future RFC; nothing in this RFC waits on it.

ADR-008 is extended in place for each change that reverses none of its decisions: M4 extended its limitations section and M5 its attribute-search sections, and if they ship, the storage tuning extends its Secondary Indexes and TTL sections and the attribute index its Secondary Indexes section. While `map` is experimental, ADR-008 is not changed for it. If B4 makes `map` the default, that change replaces ADR-008's attribute-representation decision and gets a superseding ADR that states the new layout, links this RFC and Addendum A, and leaves ADR-008's other decisions in force.

---

## 8. Limitations

- The causes given in §5.4 for the basic `Map` measuring slower on top-N and whole-row reads come from the benchmark's byte counts and its lazy-materialization comparison, not from a profile of the ClickHouse planner; benchmark B4 shows whether they hold on Jaeger's data and releases. [PR 124553](https://github.com/ClickHouse/ClickHouse/pull/124553) also changes the read path behind the basic `Map`'s measured wins, so Addendum A's numbers predict neither its gains nor its losses on the fixed releases.
- The `Map` measurements in Addendum A come from one internal benchmark on one dataset and query mix on ClickHouse 26.8. A workload dominated by paginated whole-row queries, such as span lists with `ORDER BY ... LIMIT`, would weight those losses more heavily; the benchmark's trace-retrieval queries ran at parity on all three tables. The benchmark is not reproducible from this repository; benchmark B4 repeats the losing shapes on the public 10M-span dataset.
- The relative cost of the text index against the hashed Bloom filter on Jaeger's data, the hoisted array layout against the bucketed `Map`, and the effect of `ZSTD(3)` on Jaeger's insert throughput are unmeasured until benchmarks B1, B3 and B5 run.
- ClickHouse documentation pages carry no dates; their content is as fetched on 2026-11-06.

---

## Addendum A: Internal benchmark, arrays against typed `Map`

The numbers in §2.3, §4.2 and §5.4 come from a benchmark run in October 2026 on a ClickHouse 26.8.10 cluster that is not public. The setup and results are recorded here so that the RFC's claims can be weighed; benchmark B4 repeats the decisive shapes on the public benchmark dataset.

**Setup.** The same spans were loaded into three tables with identical sort keys, partitions, promoted columns, and skip and text indexes: (A) five typed attribute groups per level stored as `Array(LowCardinality(String))` keys and typed value arrays, as in Jaeger's layout; (B) one `Map(LowCardinality(String), T)` per value type and level with the default `basic` serialization, loaded from A by `INSERT ... SELECT` through `mapFromArrays`; (C) the same as B with `map_serialization_version = 'with_buckets'` with a constant 32 buckets, `map_buckets_min_avg_size = 0`, and bucketing applied to inserted as well as merged parts. Forty query shapes were run against each table, covering single-attribute filters, sorts and groups, multi-key reads of one row, top-N lists keyed on one attribute, span lists and `SELECT *`, and trace reconstruction, each restricted to one service over a 1-day window (1.08M spans) and a 3-day window (2.97M spans). Results agreed on all three tables under the benchmark's equivalence rules, which allowed different tied rows at a `LIMIT`, differing approximate quantiles, and the formatting difference between a `Map` and a pair of arrays in whole-row output. Each query ran six times per table in rotated order and the median is reported; on the shared cluster, differences under about 15% are noise. A traffic-weighted latency was computed from the usage log of a query tool, by mapping each of eight query families to its closest benchmark shape.

| Measurement | A: arrays | B: basic `Map` | C: bucketed `Map` |
| --- | --- | --- | --- |
| Storage per replica | 9.69GiB | 9.73GiB | 10.3GiB |
| Single-attribute filter, sort or group | 1.28s to 1.80s | 964ms to 1.61s | 18ms to 58ms |
| Top-N keyed on one attribute | 198ms to 233ms | 345ms to 443ms | 13ms to 20ms |
| Wide multi-key reads and rollups | 631ms to 4.49s | 506ms to 3.10s | 16ms to 808ms |
| Attribute filter plus other attributes in `SELECT` | 199ms to 2.87s | 392ms to 2.88s | 45ms to 878ms |
| Span list and `SELECT *` | 51ms to 137ms | 72ms to 539ms | 284ms to 618ms |
| Traffic-weighted mix, 3-day window | 1.91s | 1.39s | 448ms |
| Traffic-weighted mix, 1-day window | 1.06s | 955ms | 372ms |
| Insert CPU per 1M rows | 9.9s | 13.3s | 28.2s |
| Insert batching | full day in one batch | same as A | small batches required; a full-day batch exceeded 18.6GiB and was killed |

**Findings as recorded by the benchmark.** The basic `Map` was faster on reads that touch several keys of one row and slower on top-N and whole-row reads. On the weighted mix it was 27% faster on the 3-day window and within noise on the 1-day window, a gain the benchmark judged too small to justify migrating its own deployment. On the top-N shapes it decompressed about three times the bytes of the arrays, and up to 1.7 times on an attribute filter combined with other attributes in `SELECT`. The benchmark reported that its whole-row times lined up roughly with the arrays with lazy materialization switched off: 215 ms and 539 ms on two shapes, against 294 ms and 335 ms. The bucketed `Map` cut single-attribute shapes by one to two orders of magnitude and the weighted mix by 4.3x on the 3-day window and 2.8x on the 1-day window, at the cost of 4x to 6x on whole-row reads, 2.8x the insert CPU, small insert batches, and no vertical merges. The benchmark kept the array layout for its own deployment and named the bucketed `Map` as the option to watch, and noted that hoisting hot keys and moving payloads to their own columns should recover much of the gain on arrays without measuring that variant.

**What the benchmark did not control.** It ran one service's query mix on one 26.8 build, the only Jaeger API queries it ran carried no attribute filter, and its insert CPU for B and C came from `INSERT ... SELECT` rather than from Jaeger's writer. The causes it gives for the shapes where B lost to A point to planner behavior on 26.8 rather than to the container; a `PREWHERE` toggle isolated the second read on two filter-plus-select shapes, and nothing isolated the lazy-materialization cause (§5.4), which is why §5.4 makes the default conditional on a retest rather than on these numbers.
