# RFC 0017: ClickHouse Schema — Comparison With the ClickStack / OTel Exporter Schema

- **Status:** Draft
- **Author:** Yuri Shkuro
- **Created:** 2026-09-21
- **Last Updated:** 2026-09-21
- **Related:** [ADR-008 (ClickHouse storage schema)](../adr/008-clickhouse-storage-schema.md) · [#8715 (attribute search skip indexes)](https://github.com/jaegertracing/jaeger/issues/8715) · [#8918 (search performance, Bloom filter tuning)](https://github.com/jaegertracing/jaeger/issues/8918) · [ClickStack schema reference](https://clickhouse.com/docs/clickstack/ingesting-data/schemas#traces) · [OTel `clickhouseexporter` DDL templates](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/exporter/clickhouseexporter/internal/sqltemplates)

---

## Abstract

Jaeger's native ClickHouse backend ([ADR-008](../adr/008-clickhouse-storage-schema.md)) and ClickStack, the ClickHouse-maintained observability stack built on the OpenTelemetry Collector's `clickhouseexporter`, store the same OTLP spans in the same database with strikingly similar skeletons and materially different bodies. Both partition by day and sort by `(service, span name, second-resolution timestamp)`; both maintain a per-trace time-bounds table through a materialized view; both put a Bloom filter on the trace ID and a `minmax` index on duration. They part ways on how attributes are represented (typed `Nested` groups with a metadata side table versus a single untyped `Map`, or the newer `JSON` type), on storage tuning (Jaeger applies no codecs, no `LowCardinality`, and no table settings), and on how attribute search is indexed (Jaeger has no attribute skip indexes at all, and its attribute-only search runs about forty times slower than its other queries).

This RFC sets the two schemas side by side, classifies every difference by whether it reflects a deliberate Jaeger decision or an omission, and proposes to adopt the tuning that carries no trade-off (codecs, `LowCardinality`, `ttl_only_drop_parts`, a tighter trace-ID Bloom filter), and to settle attribute indexing by measurement rather than by adopting ClickStack's answer: the ClickHouse `JSON` type is the one layout that would serve equality, the ordered predicates of [RFC 0005](0005-structured-query-filters.md), and aggregation alike, provided its paths are typed and its dotted keys escaped, so it is benchmarked first, and ClickStack's pairwise Bloom filter, which serves equality only, is specified in full as the fallback if `JSON` fails its spike. The typed `Nested` layout stays until that decision. Reading ClickStack-written tables directly from Jaeger is assessed as feasible but is left to a follow-up RFC.

---

## 1. Motivation

ClickHouse has one dominant ingestion path for OpenTelemetry data: the Collector's `clickhouseexporter`, whose default DDL the ClickStack documentation reproduces with a handful of HyperDX-specific additions. That schema is what most ClickHouse users who arrive at Jaeger already have on disk, and its design reflects operational experience with trace volumes well beyond what Jaeger's own [benchmarks](../../internal/storage/v2/clickhouse/BENCHMARKING.md) cover (10M spans on a single node).

Three things make the comparison worth writing down now.

1. **Jaeger's attribute search is slow, and ClickStack's is indexed.** Jaeger's benchmark shows an attribute-only search at 1,769 ms against 37 to 47 ms for every other single-predicate search, and #8715 proposes Bloom filter skip indexes on the attribute columns to close the gap. The `clickhouseexporter` has shipped exactly such indexes on `mapKeys` and `mapValues` for years, and ClickStack has since replaced the value index with a text index over `key=value` pairs. Whatever Jaeger does here should be informed by what those two choices learned.
2. **Jaeger's table carries no storage tuning.** The `spans` DDL declares no compression codecs, no `LowCardinality` wrappers, and no `SETTINGS`. Every one of those appears in the ClickStack schema, and each is a pure win or a well-understood trade-off. Omitting them was not a decision recorded in ADR-008; they were simply never considered.
3. **Users ask whether Jaeger can read the tables they already have.** A deployment that already runs the `clickhouseexporter` has two options today: dual-write into Jaeger's schema, or not use Jaeger. Knowing exactly how far apart the schemas are is the prerequisite for deciding whether a read-only adapter is worth building.

ADR-008 records the decisions behind Jaeger's schema and this RFC does not reopen the ones it argues for. It does identify which parts of the schema were never argued for and proposes filling those in.

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

Five derived tables hang off it through materialized views: `trace_id_timestamps` (per-trace min/max start time, `AggregatingMergeTree`), `services`, `operations`, `attribute_metadata` (every `(key, type, level)` triple ever seen, fed by three views), plus a `dependencies` table that the dependency writer fills from a separate aggregation job rather than a materialized view.

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

The exporter also ships an experimental **JSON variant** ([`traces_json_table.sql`](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/exporter/clickhouseexporter/internal/sqltemplates/traces_json_table.sql)) in which `ResourceAttributes` and `SpanAttributes` are ClickHouse `JSON` columns accompanied by `*AttributesKeys Array(LowCardinality(String))` columns that carry the key Bloom filters, the sort key gains `Timestamp` as a fourth component, and the trace-ID Bloom filter is absent, so trace retrieval by ID falls back to the sort key and partition pruning alone. The ClickStack documentation does not yet describe it, but it is the direction the exporter is heading and §3.2 accounts for it.

---

## 3. Differences

The differences fall into eight groups. For each, the table states what each schema does and whether the Jaeger position is a recorded decision (ADR-008 argues for it), an omission (never considered), or a consequence of a decision made elsewhere.

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
| Key identity | Preserved: a key is an opaque string | Preserved | **Lost for dotted keys by default**: the `JSON` type reads `.` as nesting, so `{"http.status_code": 200}` and `{"http": {"status_code": 200}}` are the same path, and a map holding both fails insertion with `Duplicate path found`. The `json_type_escape_dots_in_keys` setting (available on the pinned 25.12.11) has the server escape dots to `%2E` on parse and unescape them on output, so both forms coexist and round-trip; subcolumn predicates and path type hints must then name the escaped path (`attrs.\`http%2Estatus_code\``) |
| Level separation | Separate column groups for resource, scope, span, event, link | Separate `Map` per level (no scope) | Separate `JSON` per level (no scope) |
| Query-time type resolution | Needs the `attribute_metadata` table to learn which typed column a string filter should hit | Not needed: everything is a string | Needed: an indexable predicate has to name the typed subcolumn (`attrs.key.:Int64`), so something must know the type; the exporter has no reader and does not address it |
| Filter expression | `arrayExists((k, v) -> k = ? AND v = ?, col.key, col.value)`, one per `(type, level)` observed | `SpanAttributes['key'] = 'value'` | `SpanAttributes.key.:Int64 = value` on the typed subcolumn; the untyped `SpanAttributes.key = value` is a `Dynamic` comparison that no index serves |
| Point-lookup cost per row | Scan the short arrays for that type | Scan the map's key array | Subcolumn read; cheapest of the three, provided the path is typed |
| Compression | 8.6x whole-table ratio measured ([BENCHMARKING.md](../../internal/storage/v2/clickhouse/BENCHMARKING.md)); the attribute columns' share of it is not broken out | Not measured here. ADR-008 rejected `Map` on compression grounds, but its Option 3 was one `Map(String, T)` per primitive type with plain `String` keys; `LowCardinality` keys narrow the gap and the ClickStack table is the production evidence that it compresses acceptably | Not measured; the ClickHouse `JSON` type stores each path as its own subcolumn, so compression should approach a flat-column layout |

ADR-008 chose typed `Nested` over `Map` on compression, filter ergonomics, and schema clarity, and that reasoning stands. The type fidelity argument is stronger than the ADR states: the Jaeger query API accepts typed `pcommon.Value` filters, and a `Map(String, String)` cannot answer `http.status_code = 500` (integer) differently from `= "500"` (string), nor can it support the ordered predicates of [RFC 0005](0005-structured-query-filters.md) (`duration_ms > 100` on an attribute) without parsing strings at query time. ClickStack accepts that limitation because HyperDX's search box is string-typed anyway; Jaeger should not.

The JSON variant is the interesting one. It gives most of the type fidelity of Jaeger's layout (JSON has no bytes type and no integer/double distinction for integral values, so a Jaeger variant would need the `@bytes@`-style tagging for those two cases) with a per-key subcolumn on disk, which is exactly what ADR-008's "Negative / Limitations" section wishes for when it says attribute filters "can't be SIMD-vectorized or skip-indexed the way a flat column can". It has four costs today. The type is production-ready only in ClickHouse 25.3 and later. The exporter still labels its variant experimental. Dotted keys need the `json_type_escape_dots_in_keys` setting on every insert, and predicates and type hints then name the escaped path, as the table says. And its indexing is not automatic: on 25.12 a path without a type hint has type `Dynamic`, and neither `minmax` nor `bloom_filter` can be declared on it, while an index on the typed subcolumn (`attrs.key.:Int64`) works and prunes only when the predicate names that same typed subcolumn. So the query builder still has to know each key's type, which is exactly what `attribute_metadata` records today, and per-key skip indexes have to be created deliberately for the keys that need them. The mechanism carries over rather than disappearing. §5 does not propose adopting it, but §7 reserves a milestone to measure it.

### 3.3 Events and links

Both schemas store events and links as `Nested` arrays on the span row, so a span with five links is one row with arrays of length five. The only difference is inside: ClickStack's `Events.Attributes` is `Array(Map(...))`, Jaeger's is five typed `Nested` groups nested inside the outer `Nested`. Jaeger's query builder searches them with a doubly nested `arrayExists`; HyperDX searches them with `arrayExists(m -> m['key'] = 'v', Events.Attributes)`. Same cost class, same absence of any index. Neither schema indexes event or link attributes, and the ClickStack `*AttributeItems` aliases cover only resource and span attributes.

### 3.4 Storage tuning

| Setting | Jaeger | ClickStack | Effect |
| --- | --- | --- | --- |
| Column codecs | None (server default, LZ4) | `ZSTD(1)` on every column, `Delta(8), ZSTD(1)` on `Timestamp` | ZSTD(1) compresses repetitive telemetry strings noticeably better than LZ4 at a modest CPU cost on write; `Delta` on a sorted timestamp turns nanosecond values into small differences before compression. |
| `LowCardinality` | None | `ServiceName`, `SpanName`, `SpanKind`, `StatusCode`, `Events.Name`, all `Map` keys | Dictionary-encodes the column: smaller on disk, faster `GROUP BY` and equality filters, and the sort key columns become integer comparisons. |
| `index_granularity` | Default (8192) | Explicit 8192 | No behavioral difference; explicit is documentation. |
| `ttl_only_drop_parts` | Default (0) | `1` | With `0`, an expired part is rewritten to drop expired rows, a heavy merge. With `1`, a part is dropped only once every row in it has expired, which for a day-partitioned table means whole parts vanish at once for free. |
| TTL expression | `start_time + INTERVAL n SECOND` | `toDate(Timestamp) + INTERVAL n` | Equivalent to within a day. |

Every row in this table is an omission on Jaeger's side. None of them was weighed in ADR-008, and the only behavioral change among them is the retention slack that `ttl_only_drop_parts` introduces, which §5.1 quantifies. `LowCardinality` on `service_name` and `name` deserves one caveat: ClickHouse's own guidance is to use it for columns under roughly ten thousand distinct values, and a deployment with more operation names than that would see the dictionary spill and lose the benefit without becoming incorrect.

### 3.5 Skip indexes

| Index | Jaeger | Exporter default | ClickStack (HyperDX) |
| --- | --- | --- | --- |
| Trace ID | `bloom_filter` (default false-positive rate 0.025) | `bloom_filter(0.001)` | `bloom_filter(0.001)` |
| Duration | `minmax` | `minmax` | `minmax` |
| Attribute keys | none | `bloom_filter(0.01)` on `mapKeys(...)` | same |
| Attribute values | none | `bloom_filter(0.01)` on `mapValues(...)` | `text(tokenizer = 'array')` on the `key=value` alias columns |
| Span name tokens | none | none | `tokenbf_v1(32768, 3, 0)` on `lower(SpanName)` |

Three observations.

**The trace-ID Bloom filter is the same idea at a different tolerance.** Jaeger relies on the server default of a 2.5 percent false-positive rate. At granularity 1 every granule carries its own filter, so a false positive costs one decompressed granule of roughly 8,192 rows. The reporter of #8918, running a billion-row table, had to tighten the rate to 0.0001 by hand to make `GetTraces` acceptable; #8923, which is open, rewrites `FindTraces` into two queries to avoid the subquery ClickHouse was not short-circuiting and, as its secondary part, makes the rate configurable for newly created tables while keeping the 0.025 default; the DDL on `main` still carries that default. ClickStack's 0.001 is twenty-five times tighter than Jaeger's default at a modest increase in index size.

**Separate key and value Bloom filters cannot prove a pair absent.** This is a pruning limit, not a correctness one: a skip index only chooses which granules to read, and the `WHERE` clause is still evaluated on every surviving row, so the result set is the same whichever index shape is used and only the latency differs. The exporter's `mapKeys`/`mapValues` indexes each answer "does any row in this granule contain this key" and "does any row contain this value". A granule that contains the key `http.method` on one row and the value `POST` on another passes both filters even if no row has `http.method=POST`. For the common high-cardinality attribute (a user ID, a request ID) the value filter alone is selective enough, which is why the exporter's design works in practice. ClickStack's move to a `key=value` item index closes the gap exactly, at the cost of depending on the `text` index type, which is beta on the 25.12.11 release that the ClickHouse storage integration tests pin (`docker-compose/clickhouse/`) and [generally available from ClickHouse 26.2](https://clickhouse.com/docs/en/guides/searching-data/full-text-search-with-text-indexes). #8715 proposes the exporter's separate-filter shape for Jaeger; §5.2 argues for the pairwise shape using the stable `bloom_filter` type instead.

**Whether an index is consulted depends on the query expression.** A skip index is used only when the `WHERE` clause contains the indexed expression in a form the planner recognizes: `has(arr, x)`, `mapContains(m, k)`, `m[k] = v`, and similar. Jaeger's query builder emits `arrayExists` with a lambda, which no skip index can serve. Adding indexes without changing the predicate shape does nothing, which is a detail #8715 leaves implicit and §5.2 makes explicit.

### 3.6 Derived tables

| Table | Jaeger | ClickStack |
| --- | --- | --- |
| Per-trace time bounds | `trace_id_timestamps`: `AggregatingMergeTree`, `ORDER BY trace_id`, `SimpleAggregateFunction(min/max, DateTime64(9))`, no partition, TTL on `end` | `otel_traces_trace_id_ts`: `MergeTree`, `PARTITION BY toDate(Start)`, `ORDER BY (TraceId, Start)`, `DateTime` (second) precision, own `bloom_filter(0.01)` on `TraceId` |
| Services | `services` (`AggregatingMergeTree`) | none; `SELECT DISTINCT ServiceName` on the main table |
| Operations | `operations` keyed `(service_name, span_kind)` | none; `GROUP BY SpanName` on the main table |
| Attribute metadata | `attribute_metadata` (`(key, type, level)` triples) | none; not needed for untyped `Map` |
| Dependencies | `dependencies` (JSON written by the dependency writer, not by a materialized view) | none |

The time-bounds tables differ in an instructive way. Jaeger's collapses to one row per trace in the background and is not partitioned, so a lookup is a point read on the sort key but TTL expiry must rewrite parts. ClickStack's keeps one row per trace per insert block, partitions by day so that TTL drops whole parts, and leaves the `min`/`max` to the reader. With `ttl_only_drop_parts` this is the cheaper design at retention time and the marginally more expensive one at read time; for Jaeger the read happens once per search, on both the `FindTraceIDs` and `FindTraces` paths, over a bounded candidate set, so the difference is small either way and §5 leaves the table alone.

The services and operations tables exist because Jaeger's API needs them as first-class lists and ADR-008 chose to precompute them. ClickStack does without because `LowCardinality(ServiceName)` makes the `DISTINCT` cheap enough on the main table. If Jaeger adopts `LowCardinality` (§5.1) the same shortcut becomes available, but replacing a working precomputation is a maintainer's call and is out of scope here; #8906 (operations table collapsing names) is the place that decision would be made.

### 3.7 Product-specific columns

The `__hdx_materialized_rum.sessionId` and `SampleRate` materialized columns and the `idx_lower_span_name` token filter are HyperDX features (session replay correlation, tail-sampling weights, substring search on span names). None has a Jaeger counterpart and none is proposed here. The mechanism behind them is worth noting: a `MATERIALIZED` column computed from a `Map` lookup, plus a Bloom filter on it, is how ClickStack promotes a hot attribute to a first-class indexed column without changing the writer. Jaeger's typed `Nested` layout would need an `arrayFirst` instead of a map subscript but the pattern transfers, and it is the natural answer if a deployment needs one attribute searched as fast as `service_name`.

### 3.8 Schema management

| Capability | Jaeger | Exporter |
| --- | --- | --- |
| Create schema on start | `create_schema: true` | `create_schema: true` (default) |
| Bring your own schema | Supported by setting `create_schema: false` and creating compatible tables | Same, and documented as the recommended production mode |
| Table engine | Hard-coded `MergeTree` | `table_engine` config (name plus parameters), so `ReplicatedMergeTree` works |
| Cluster DDL | none | `cluster_name` adds `ON CLUSTER` to every statement |
| Database creation | Assumed to exist | Created if missing |

Jaeger's single-node assumption is recorded in ADR-008's limitations. The exporter's `table_engine` option is the smallest change that lifts it, and §5.4 proposes it.

---

## 4. Assessment

The criteria below are the ones a Jaeger deployment cares about. The columns are Jaeger's schema as it stands, Jaeger's schema with the §5 proposals applied, and ClickStack's `Map` schema as a reference point. Legend: 🟢 good · 🟡 partial or caveated · 🔴 poor.

| Criterion | Jaeger today | Jaeger + §5 | ClickStack (`Map`) |
| --- | --- | --- | --- |
| Attribute type fidelity | 🟢 | 🟢 | 🔴 ¹ |
| Attribute-only search latency | 🔴 ² | 🟡 ³ | 🟡 ³ |
| Compression on disk | 🟡 ⁴ | 🟢 | 🟢 |
| Trace retrieval by ID at scale | 🟡 ⁵ | 🟢 | 🟡 ⁶ |
| Retention cost | 🟡 ⁷ | 🟢 ⁸ | 🟢 |
| Scope attributes preserved | 🟢 | 🟢 | 🔴 |
| Replicated / clustered deployment | 🟡 ⁹ | 🟢 | 🟢 |
| Reads tables written by the OTel `clickhouseexporter` | 🔴 | 🔴 ¹⁰ | 🟢 |
| Schema simplicity (tables, views) | 🟡 ¹¹ | 🟡 ¹¹ | 🟢 |

- ¹ Every value is stringified on write; integer, boolean, and ordered predicates are unanswerable without query-time parsing.
- ² 1,769 ms attribute-only search versus 37 to 47 ms for every other single-predicate search on the 10M-span benchmark; no skip index exists on any attribute column.
- ³ Whichever branch §5.2 selects prunes equality by granule and still pays a per-row check on the survivors (Jaeger's `arrayExists`, ClickStack's map lookup), so the result is bounded by selectivity rather than fixed; only the `JSON` branch also serves ordered predicates. ClickStack's pairwise item index prunes a common key with a common value; the exporter's default separate key and value filters do not (§3.5), which costs latency and never correctness.
- ⁴ 8.6x whole-table ratio measured, but with the server default LZ4 codec and no dictionary encoding on the sort-key columns.
- ⁵ Default 2.5 percent Bloom false-positive rate; a billion-row deployment reported needing 0.0001.
- ⁶ The 0.001 rate is fixed in the DDL; a deployment that needs the 0.0001 of footnote ⁵ has to own its schema.
- ⁷ TTL expiry rewrites parts to remove expired rows instead of dropping whole day-partition parts.
- ⁸ Whole-part expiry applies to `spans`, which holds nearly all the data; `trace_id_timestamps` keeps row-level TTL (§5.1), and expiry lags the configured TTL by up to one partition day.
- ⁹ Possible only by creating every table by hand with `create_schema: false`; the SQL templates hard-code `MergeTree` and nothing else.
- ¹⁰ On the fallback branch the proposals change neither column names nor attribute representation, so a ClickStack table remains unreadable by Jaeger's reader; §6 covers what would.
- ¹¹ One main table plus five derived tables and six materialized views, against ClickStack's one plus one.

The "Jaeger + §5" column is scored with the `Nested` layout staying, which is §5.2's fallback branch; the `JSON` branch replaces the attribute model and would be scored by its own superseding RFC. On that basis the matrix says Jaeger's schema is right where it made a decision and behind where it made none, and the proposals below change nothing in the attribute model or the derived tables and everything in the tuning and indexing layer.

---

## 5. Proposal

### 5.1 Adopt the storage tuning

Apply to `spans`, and the codecs and `LowCardinality` wrappers to the derived tables where the column exists:

- `CODEC(ZSTD(1))` on every column, and `CODEC(Delta(8), ZSTD(1))` on `start_time`, which the sort key orders to the second within each `(service_name, name)` run, so consecutive differences are small. Event timestamps are arrays with no ordering across rows, so they take plain `ZSTD(1)` as in ClickStack.
- `LowCardinality(String)` on `service_name`, `name`, `kind`, `status_code`, `events.name`, and, one step past ClickStack's set, on `scope_name` and `scope_version`, because an instrumentation scope is a library name and version and a service carries a handful of them, and on the `key` member of every attribute `Nested` group (`Nested(key LowCardinality(String), value ...)`).
- `SETTINGS index_granularity = 8192, ttl_only_drop_parts = 1` on `spans` only. `trace_id_timestamps` has no partition key, so one of its parts can hold bounds from many days and whole-part expiry would keep expired trace IDs alive for as long as the newest row in the part; it keeps row-level TTL.

The factory only ever runs `CREATE TABLE IF NOT EXISTS`, so an existing table is untouched by the change and a new deployment gets all of it. For an existing table the three parts migrate differently: `ALTER TABLE ... MODIFY SETTING` is a metadata change, `MODIFY COLUMN ... CODEC(...)` is also metadata-only and takes effect on parts as background merges rewrite them, but `MODIFY COLUMN ... LowCardinality(String)` is a type change and ClickHouse rewrites every part that holds the column. M1 therefore ships the DDL for new tables and a documented, hand-run `ALTER` sequence for existing ones, with the type change called out as the expensive step that an operator may choose to skip. `ttl_only_drop_parts = 1` changes when data disappears: a row is removed only when its whole part has expired, so a day-partitioned table with a 7-day TTL retains up to one extra day. The trade is worth making, and the configuration documentation should say so.

**Tighten the trace-ID Bloom filter** to `bloom_filter(0.001)`, matching ClickStack. #8923 proposes to expose the rate in configuration so that a deployment at #8918's scale can go tighter without hand-editing DDL; this RFC adds only the change of default, which lands on top of that option once it merges rather than in a second mechanism. The index name stays `idx_trace_id`, so on an existing table the change is `DROP INDEX` then `ADD INDEX` and `MATERIALIZE INDEX` for old parts.

### 5.2 Decide the attribute index by measurement: `JSON` subcolumns first, the pair filter as fallback

Issue #8715 asks for Bloom filter skip indexes on the attribute columns, and ClickStack shows that shape working. It is not adopted here outright, because a Bloom filter serves equality only and the project's query surface is moving past equality. The section first sets out what each candidate can answer, then the decision, then the pair filter in enough detail to build it if the decision falls that way.

**What an attribute index can and cannot answer.** A Bloom filter skip index, whatever expression it is built on, answers one question per granule: might this granule contain value X. It serves equality and `IN`, and nothing else. Ordered predicates on typed values (RFC 0005's `http.status_code >= 500`) and aggregations over an attribute (`GROUP BY` a key's values) need the key's values addressable as a column, which no index over a shared `Nested` group provides. The shapes on offer differ in which equalities they prune and in whether they open a path to the rest. Legend: 🟢 good · 🟡 partial · 🔴 not served.

| Criterion | Key filter + value filter (exporter) | Pair filter, hashed (fallback) | Per-key `JSON` subcolumn (spike first) | Inverted `(key, value)` table |
| --- | --- | --- | --- | --- |
| Equality, rare key | 🟢 | 🟢 | 🟢 | 🟢 |
| Equality, common key and common value | 🔴 ¹ | 🟢 | 🟡 ² | 🟢 |
| Ordered predicate on a typed value | 🔴 | 🔴 | 🟡 ³ | 🟢 |
| Aggregation over a key's values | 🔴 | 🔴 | 🟢 ⁴ | 🟡 ⁵ |
| Extra storage | 🟡 ⁶ | 🟡 ⁷ | 🟡 ⁸ | 🔴 ⁹ |
| Fits the current typed `Nested` layout | 🟢 | 🟢 | 🔴 ¹⁰ | 🟡 ¹¹ |

- ¹ Both filters pass a granule that has the key on one row and the value on another; for `http.status_code=500` that is every granule.
- ² A typed `JSON` subcolumn is a plain column, so a Bloom filter on that one path works, but only when the path is typed and the index is declared per key; a dynamically discovered path cannot carry one.
- ³ Without an index, an ordered predicate on `attrs.key.:Int64` is a vectorized scan of that one path's data, far less than the `Nested` arrays but still a scan. `minmax` pruning, the flat-column indexing ADR-008's limitations section wishes for, requires a per-key index on the typed subcolumn and a predicate that names it; on 25.12 the same predicate written against the untyped path read every granule.
- ⁴ `GROUP BY attrs.key.:String` scans one path's subcolumn; it needs the type, not an index.
- ⁵ A `GROUP BY` over one key is a range scan of that key's slice of the table; joining back to spans for anything else is a second query.
- ⁶ Two Bloom filters per attribute group, each over every key or every value.
- ⁷ The alias is never stored, but each Bloom filter is, and a Bloom filter is incompressible by construction: its size is fixed by element count and target rate, and its bits are as random as the hashes that set them. Fifteen filters over every attribute pair of every span are a real fraction of the table.
- ⁸ Dynamic paths above the `max_dynamic_paths` limit fall back to a shared column, and the type's per-path bookkeeping costs space.
- ⁹ One row per attribute pair per span, sorted independently of `spans`: a second copy of every attribute.
- ¹⁰ Replaces the five typed `Nested` groups and the query builder over them; `attribute_metadata` stays as the source of each path's type. A superseding RFC.
- ¹¹ Adds a table and a materialized view beside the existing layout, and the reader has to join.

**The decision.** Two options survive the matrix. The pair filter is the strongest equality index available without changing the attribute layout, and equality is the whole of what the UI sends today. `JSON` subcolumns serve equality, ordered predicates, and aggregation at once, once the paths are typed and the keys escaped, and the type is generally available on the 25.12.11 release the storage integration tests pin. The inverted table is rejected: it stores a second copy of every attribute to serve two of the three query rows that `JSON` serves with one.

Between the two survivors, the order matters more than the choice. Everything the pair filter needs (fifteen indexes, prefilter branches in the query builder, a migration procedure, the boolean normalization below) is work that a `JSON` layout would delete, apart from the type resolution that both layouts share, and [RFC 0005](0005-structured-query-filters.md) and [RFC 0015](0015-typed-attribute-indexing-elasticsearch.md) show where the query surface is going. So the `JSON` spike runs first (§7, M2). It is benchmark-only and cheap, and it decides: if `JSON` passes on compression, typed round-trip correctness, attribute search, and trace retrieval, the attribute layout is replaced under a superseding RFC that also covers migration of existing tables, and the pair filter is never built; if it fails, the pair filter below ships as M3 against the `Nested` layout that stays. Issue #8715 waits one spike longer than it would otherwise, which is the cost of not building an index the project would then remove.

**The fallback: pairwise skip indexes on the `Nested` layout.** What follows is the complete design, so that a negative spike result leads straight to implementation.

**Index the pair, not the key and value separately.** For each string-attribute group, add an `ALIAS` column that hashes each pair to one 64-bit value, and a `bloom_filter` on it:

```sql
ALTER TABLE spans
    ADD COLUMN str_attribute_hashes Array(UInt64)
        ALIAS arrayMap((k, v) -> cityHash64(k, v), str_attributes.key, str_attributes.value),
    ADD INDEX idx_str_attribute_hashes str_attribute_hashes TYPE bloom_filter(0.01) GRANULARITY 4;
```

and likewise for the resource and scope groups, and for the event and link groups through `arrayFlatten` over the nested arrays. `bloom_filter` on an `Array` column indexes each element and is served by `has()`. The shape is ClickStack's pairwise item index with two changes.

- **The item is a hash, not a `key=value` string.** `cityHash64(k, v)` hashes the two arguments as a tuple, so there is no separator to collide on, and the alias itself is never written to disk (the Bloom filter built from it is; footnote ⁷ above). A string alias is not an option on the 25.12.11 release the storage integration tests pin: on 25.12.11.4, `EXPLAIN indexes = 1` over a 200k-row test table showed `has()` on a `concat(k, '=', v)` `ALIAS` column reading all 25 granules with the index never consulted, while the same predicate on the hashed `ALIAS` column consulted the index and read 1 of 25. A `MATERIALIZED` string column is also served, at the cost of storing every pair twice, which the hash makes unnecessary. The check holds with the `LowCardinality(String)` keys that M1 introduces: `cityHash64` of a `LowCardinality(String)` equals `cityHash64` of the same `String`, and on a table with `LowCardinality` keys the inline predicate read the same 1 of 25 granules. The fallback milestone repeats this check on the benchmark table before anything else, since the whole design rests on it.
- **Every level is indexed, including events and links.** A skip index can prune a granule only when the whole `WHERE` clause is provably false for it, and an `OR` is provably false only when every branch is. The query builder's fallback for a key that `attribute_metadata` has not seen ORs five branches, one per level, and the metadata path ORs one branch per observed `(level, type)` pair whose type can parse the value. One unindexed branch in that `OR` therefore disables pruning for the entire attribute predicate, which is exactly the attribute-only benchmark case. So the hash columns and indexes cover all five levels, and the nested `arrayExists` for events and links gains the same `has()` prefilter.

`GRANULARITY 4` (one filter per four granules) is a starting point rather than a measurement: every row contributes every attribute pair, so a per-granule filter at 0.01 is large, and coarser granularity trades pruning resolution for index size. The fallback milestone measures 1 against 4.

The string, integer, and boolean groups are indexed, the last two through `cityHash64(k, toString(v))`. Strings are where the UI's filters land, since every value the search box sends arrives as a string and `attribute_metadata` resolves it to a typed column only when the key has been seen with that type. Integers are included for two reasons: they carry high-cardinality values (request IDs, ports, user IDs), and a key that `attribute_metadata` has seen as both a string and an integer produces an `OR` of both typed branches, which the `OR` rule above turns into a full scan unless both branches are indexed. An `OR` whose branches are all indexed does prune: on 25.12.11.4 two `has()` branches on two indexed columns read 2 of 25 granules through the planner's combined skip indexes. Booleans ride along for the same mixed-type reason and because the indexed item is the pair, so a rare pair such as `error=true` is exactly what a Bloom filter prunes well. Double and complex groups stay unindexed, doubles because their filters are ranges more often than equalities and complex values because equality on a serialized map or slice is not a search anyone runs, so a key whose metadata includes either type keeps today's scan; the acceptance benchmark includes such a mixed-type key so that this limit is measured. Range predicates on attributes ([RFC 0005](0005-structured-query-filters.md)) are not Bloom-shaped and are out of scope here.

**Change the predicate the query builder emits** for every indexed group at every level. For the string groups it goes from

```sql
arrayExists((key, value) -> key = ? AND value = ?, s.str_attributes.key, s.str_attributes.value)
```

to

```sql
has(arrayMap((k, v) -> cityHash64(k, v), s.str_attributes.key, s.str_attributes.value), cityHash64(?, ?))
AND arrayExists((key, value) -> key = ? AND value = ?, s.str_attributes.key, s.str_attributes.value)
```

The predicate spells out the hash expression instead of naming the alias, so the same query runs against a table that predates the alias (an upgraded deployment that has not run the `ALTER` sequence yet) and simply scans as it does today, while on a table that has the alias and its index the planner matches the expression to the index and prunes. On 25.12.11.4 the inline form read the same 1 of 25 granules as the alias name did, with and without the trailing `arrayExists`. The `has()` is the index-driving prefilter and the `arrayExists` stays as the exact check, because both the hash and the Bloom filter admit collisions. On every granule the prefilter admits, each row now computes the alias and `has()` in addition to the array scan it runs today, so the change wins only when the index drops enough granules to pay for that. The acceptance benchmark therefore includes a low-selectivity attribute (a key present on most spans with few distinct values) alongside the high-selectivity case, so the worst case is measured rather than assumed. The integer and boolean branches gain the same prefilter, with the bound value formatted on the query side as `cityHash64(?, toString(?))` for integers and `cityHash64(?, toString(CAST(? AS Bool)))` for booleans. Both sides go through `toString` so that the hash depends only on the decimal text and not on how the driver types the bound parameter; on 25.12 `cityHash64` happens to hash an integer literal and an `Int64` alike, but that is an implementation property rather than a documented one. The boolean cast is mandatory rather than defensive: the driver binds a Go boolean as `1` or `0` while `toString` of a stored `Bool` yields `true` or `false`, and an unnormalized bound value would hash to a different item and silently reject every match. The typed `arrayExists` stays as their exact check, and the correctness tests include driver-bound `true` and `false` filters. A key seen only as string, integer, or boolean thus emits only prefiltered branches, which is what the `OR` rule above requires; a key also seen as double or complex keeps an unprefiltered branch and today's scan, as the previous paragraph says. For events and links the exact check is the existing doubly nested `arrayExists` and the prefilter is `has()` on the flattened hash column.

On an existing table `ADD INDEX` covers only parts written afterwards, so the migration sequence follows it with `MATERIALIZE INDEX` and waits for the mutation to finish in `system.mutations`; until then historical parts are scanned as before, and a before/after benchmark that skips this step measures nothing. The alias itself needs no backfill.

Acceptance for the fallback is the #8715 benchmark: attribute-only search on the 10M-span dataset before and after, with `EXPLAIN indexes = 1` output showing granules dropped. Because fifteen Bloom filters (three types across five levels) each hash every attribute pair on insert, the benchmark also reports insert throughput, the on-disk size of the indexes relative to the table, and the wall time of `MATERIALIZE INDEX` over the existing data, so that the write-side cost is weighed against the read-side gain.

### 5.3 Keep the typed `Nested` attribute layout until the `JSON` spike decides

ADR-008's decision stands until §5.2's spike says otherwise, for the type-fidelity reasons §3.2 adds to it. `Map(String, String)` is rejected outright. The `JSON` type is the only alternative that preserves types and improves on the current layout, and it is measured rather than adopted here because it requires ClickHouse 25.3 or later, because the exporter's own JSON variant is still experimental, and because it replaces the five typed `Nested` groups and their query builder outright, with `attribute_metadata` carrying over as the source of each path's type. The spike has to settle two design questions before it measures anything: whether `json_type_escape_dots_in_keys` covers dotted keys end to end, since the `JSON` type otherwise reads `.` as nesting and rejects a map that holds both `a.b` and `a` → `b`, which means confirming that the writer sets it on every insert, that the reader gets the original keys back, and that predicates and type hints written against the `%2E` form are served; and how the query builder learns each key's type so it can name the typed subcolumn (`attrs.key.:Int64`), because an index can be declared only on a typed path and a predicate against the untyped path is not served by it. The natural answer to the second is the existing `attribute_metadata` table, so the spike should assume it stays. The pass criteria are then compression no worse than the `Nested` layout after M1; round-trip and equality-filter correctness for all seven `pcommon` value types (bytes and integral doubles need the `@bytes@`-style tagging §3.2 describes) and for dotted keys, including a map that holds both a dotted key and its nested-object twin; attribute search at least as fast as the pair filter's measured result on the same data, both with and without per-key skip indexes on the typed subcolumns, so the operational cost of declaring them per key is visible; an ordered predicate on a typed subcolumn pruning granules where the pair filter cannot; and trace retrieval by ID with a trace-ID Bloom filter added back, since the exporter's JSON template has none. §5.2 states what each outcome leads to.

### 5.4 Make the table engine configurable

Add a `table_engine` option, defaulting to plain `MergeTree`, and substitute it into every `ENGINE =` clause the factory renders. The exporter's option is a literal engine name because the exporter has one table per signal; Jaeger has `MergeTree` and `AggregatingMergeTree` tables from the same DDL set, so the option is modeled as an engine-family prefix (`Replicated`) plus its parameters (ZooKeeper path and replica name), and the factory composes `ReplicatedMergeTree(...)` or `ReplicatedAggregatingMergeTree(...)` per table. This lifts ADR-008's single-node limitation for deployments that manage a cluster themselves; `ON CLUSTER` DDL is not proposed, because deployments that need it also need to own their DDL and `create_schema: false` already serves them.

### 5.5 Do not change what is not broken

Column names stay `snake_case`, `kind` stays lowercase, `duration` stays `Int64`, scope attributes stay, and the five derived tables stay. Each is either shared with Jaeger's other backends or a recorded decision, and none of the ClickStack differences in those areas is an improvement.

---

## 6. Reading ClickStack Tables Directly

The question users actually ask is whether Jaeger can point at an existing `otel_traces` table. The comparison makes the answer concrete.

**What is the same** is what matters most: the partition key, the sort key, the trace-ID Bloom filter, and the existence of a per-trace time-bounds table. Every query shape Jaeger's reader issues (search narrowed by service, name, and time; trace retrieval by ID with time hints; per-trace bounds lookup) has an efficient equivalent against `otel_traces`.

**What differs** is mechanical: column names, `kind` casing, `UInt64` duration, `DateTime` seconds in the bounds table, and the absence of scope attributes and of the `services`, `operations`, and `attribute_metadata` tables. Services and operations become `DISTINCT` queries on `LowCardinality` columns, which is what HyperDX does. Attribute metadata is unnecessary because there is exactly one type: string. That means the read adapter would accept only string attribute filters, which is exactly what the Jaeger UI sends today and exactly what a ClickStack user already lives with.

**What is lost** is type fidelity on the way out: every attribute comes back as a string, so a trace written by the exporter and read by Jaeger shows `http.status_code: "200"` where the SDK emitted an integer. This is a property of the data on disk, not of the adapter.

A read-only adapter is therefore feasible as a second `dbmodel` and a second set of query templates behind the same `tracestore.Reader` interface, selected by configuration. It is not proposed in this RFC because it is a feature with its own scope (a writer is neither needed nor wanted, since the exporter is the writer), its own compatibility surface (the exporter's schema is versioned by the exporter), and its own tests. It should be its own RFC once there is a demand signal beyond the question being asked.

---

## 7. Implementation Plan

Each milestone is independently shippable and each carries a before/after benchmark on the [documented setup](../../internal/storage/v2/clickhouse/BENCHMARKING.md).

- **M1 — Storage tuning (§5.1).** Codecs, `LowCardinality`, table settings, the configuration note on `ttl_only_drop_parts`, and the 0.001 default for the trace-ID filter on top of the option #8923 proposes. Measured by compressed size and insert throughput; no query-shape change. #8923 itself resolves #8918, whose main subject is the `FindTraces` subquery rather than the Bloom rate.
- **M2 — JSON attribute spike (§5.3).** A benchmark-only branch storing attributes as `JSON` columns on the 25.12.11 release the storage integration tests pin, run against the §5.2 pair filter on the same 10M-span data so that the two are compared under one setup. Settles the two design questions and measures the pass criteria in §5.3. Its output is a decision, recorded in this RFC's status, and on a pass a superseding RFC.
- **M3 — Attribute index (§5.2).** One of two shapes, chosen by M2. On a pass: the `JSON` layout, under its own RFC, which closes #8715 by replacing the mechanism it asks about. On a fail: the pairwise `bloom_filter` design, the inline `has()` prefilter in the query builder, and the migration procedure, measured by the acceptance benchmark in §5.2, which closes #8715 directly.
- **M4 — Configurable table engine (§5.4).** The `table_engine` option and its rendering into every DDL statement, exercised by an integration test against a `ReplicatedMergeTree` single-replica Keeper setup.

ADR-008 is extended in place when M1 lands, in its Secondary Indexes and TTL sections, because nothing there reverses a decision it records, and again if M3 takes the pair-filter branch. The `JSON` branch of M3 supersedes ADR-008's attribute sections under its own RFC. M4 updates ADR-008's limitations section.
