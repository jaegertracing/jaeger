# ClickHouse Storage Schema

* **Status**: Implemented
* **Date**: 2026-04-25

## Context

ClickHouse has been one of the most-requested storage backends for Jaeger. This ADR proposes a native ClickHouse backend for Jaeger v2 — covering schema design, primary-key choice, secondary indexes, and the materialized views that support Jaeger's query API. It also documents the alternatives we considered and why we rejected them.

The implementation lives in:

* [`internal/storage/v2/clickhouse/sql/`](../../internal/storage/v2/clickhouse/sql/) — table and materialized-view DDL
* [`internal/storage/v2/clickhouse/`](../../internal/storage/v2/clickhouse/) — reader, writer, and config

### Why ClickHouse?

ClickHouse is a column-oriented analytical database optimized for high-throughput ingest and fast queries over large datasets. The properties that make it a strong fit for trace storage:

* **Column-oriented compression.** Telemetry data is highly repetitive across rows (service names, attribute keys, status codes), and column-major layout compresses such data far better than row-major formats. Less data on disk means less I/O and faster queries.
* **`MergeTree` engine.** Inserts produce immutable parts that are combined and transformed by background merges, keeping the write path lightweight. Specialized engines (`AggregatingMergeTree`, `ReplacingMergeTree`) push deduplication and aggregation off the write path.
* **Vectorized query processing.** Operators process columnar batches with SIMD instructions and parallelize across cores.

### Relevant ClickHouse Concepts

* **`Nested` data type** — stores arrays of structured values aligned within each row, behaving like a sub-table per row. Used here for per-span collections (events, links, attributes).
* **Primary key** — does *not* enforce uniqueness in ClickHouse. It defines the on-disk sort order and powers a sparse index that stores one entry per granule (8,192 rows). Filtering by the primary key turns into a fast in-memory binary search over granules.
* **Skip indexes (data-skipping indexes)** — secondary indexes that store small per-granule summaries (`minmax`, `set`, `bloom_filter`). Before reading a granule, the engine consults the index; if it can prove no row in the granule matches the filter, the granule is skipped entirely.
* **Materialized views** — server-side triggers that transform rows on insert into a source table and write the result to a target table.

### Existing Implementations

* **`jaeger-clickhouse` gRPC plugin.** The original ClickHouse integration was an out-of-process gRPC plugin. Plugin-based storage was deprecated in [#4647](https://github.com/jaegertracing/jaeger/pull/4647) and removed in subsequent releases, so this plugin no longer works with current Jaeger versions.
* **OpenTelemetry `clickhouseexporter`.** OTel provides an exporter for sending telemetry to ClickHouse, but it only handles writes. Jaeger needs to read trace data as well, so we cannot depend on it directly.
* **GSoC 2024 (Jaeger v1).** A GSoC mentee benchmarked both implementations and proposed a v1 schema in [this blog post](https://medium.com/jaegertracing/jaeger-clickhouse-storage-implementation-comparison-b756dafde114). The proposed design — single spans table, materialized views for services/operations and trace-id timestamps, attributes stored with `Nested`, client-side batching — informs much of the v2 design here.

---

## Decision

The ClickHouse backend is built around a single `spans` table (storing one row per span with all attached metadata as `Nested` columns) plus a small number of `AggregatingMergeTree` derived tables maintained by materialized views. The `spans` table is sorted to optimize trace search, `GetTraces` is made viable by a Bloom filter on `trace_id`, and a precomputed time-bounds table lets `FindTraceIDs` populate the optional `(start, end)` hint on each result so the follow-up `GetTraces` call can prune partitions and granules.

The remainder of this section walks through the schema and the alternatives that were considered.

---

## Storing Typed Attributes

Jaeger v1 tags were always strings. The Jaeger v2 reader API accepts a `pcommon.Map`, so an attribute value can be one of `Bool`, `Int64`, `Float64`, `String`, `Bytes`, `Slice`, or `KvList`. The storage layer must support filtering by all of these types, not just round-tripping them.

In every option below, complex types (`Bytes`, `Slice`, `KvList`) are encoded to strings and stored alongside other strings in a `complex_*` group. Their keys are prefixed with a type tag (`@bytes@`, `@slice@`, `@map@`) on ingest and stripped on read so the original key and type can be recovered.

### Option 1: Parallel Arrays

Each attribute type is represented by a pair of arrays. Both arrays must always be the same length; the element at index `i` in the keys array corresponds to the value at index `i` in the values array.

```sql
bool_keys     Array(String), bool_values     Array(Bool),
double_keys   Array(String), double_values   Array(Float64),
int_keys      Array(String), int_values      Array(Int64),
str_keys      Array(String), str_values      Array(String),
complex_keys  Array(String), complex_values  Array(String),
```

### Option 2: Nested ✅

Each attribute type is represented by a `Nested(key, value)` column. ClickHouse stores `Nested` columns as a pair of aligned arrays under the hood, but the engine guarantees the arrays stay the same length and gives us natural `ARRAY JOIN` semantics over them.

```sql
bool_attributes    Nested(key String, value Bool),
double_attributes  Nested(key String, value Float64),
int_attributes     Nested(key String, value Int64),
str_attributes     Nested(key String, value String),
complex_attributes Nested(key String, value String),
```

### Option 3: Map

Each attribute type is represented as a `Map(String, T)`. Because `Map` requires a single value type, we still need one column per primitive type.

```sql
bool_attrs    Map(String, Bool),
double_attrs  Map(String, Float64),
int_attrs     Map(String, Int64),
str_attrs     Map(String, String),
complex_attrs Map(String, String),
```

### Comparative Analysis

|Criterion|Parallel Arrays|**Nested ✅**|Map|
|---|---|---|---|
|Compression|🟢|🟢|🔴|
|Filtering ergonomics|🟡|🟢|🔴|
|Schema clarity|🔴|🟢|🟢|

`Map` was rejected because its on-disk representation as `Tuple(Array, Array)` compresses poorly compared to the array-based options. Parallel arrays match `Nested` in compression, but the engine does not enforce that the `*_keys` and `*_values` columns stay the same length, and filter expressions are noisier.

The `Nested` representation is reused at every level — span, event, link, resource, scope — to keep the attribute query path uniform.

---

## Resolving Attributes at Query Time

Storing typed attributes is only half the problem. The other half is figuring out, *at query time*, which typed column and which level to filter against for any given user-supplied attribute filter.

### The Query-Time Type Problem

The Jaeger query API surfaces attribute filters as a `pcommon.Map`, but in practice every value the UI sends arrives as a **string**: the user typed it into a search box, and there is no way for the client to know whether `200` is an integer status code, a string label, or a span attribute vs. a resource attribute. Two pieces of information are missing for every search:

* **Which typed column** to filter? The same logical key (e.g. `http.status_code`) might be stored in `int_attributes` for one service and `str_attributes` for another, depending on which SDK emitted it.
* **At which level** does the attribute live? A given key might appear as a span attribute, a resource attribute, a scope attribute, or inside an event/link.

Without that information, the only safe query is "search every typed column at every level," which fans the filter out across ten or more `arrayExists` predicates per attribute — wasted work in the common case where the key only ever shows up in one place at one type.

### A Dedicated `attribute_metadata` Table

The schema maintains a small, dedicated table that records every `(attribute_key, type, level)` triple ever seen, populated by materialized views off `spans`. The `type` column takes one of `bool`, `double`, `int`, `str`, `bytes`, `map`, or `slice`; `level` is one of `resource`, `scope`, `span`, `event`, or `link`.

DDL: [`create_attribute_metadata_table.sql`](../../internal/storage/v2/clickhouse/sql/create_attribute_metadata_table.sql), populated by [`create_attribute_metadata_mv.sql`](../../internal/storage/v2/clickhouse/sql/create_attribute_metadata_mv.sql) (span/resource/scope), [`create_event_attribute_metadata_mv.sql`](../../internal/storage/v2/clickhouse/sql/create_event_attribute_metadata_mv.sql), and [`create_link_attribute_metadata_mv.sql`](../../internal/storage/v2/clickhouse/sql/create_link_attribute_metadata_mv.sql).

Splitting events and links into their own views avoids forcing one large `ARRAY JOIN` across all nested collections in a single view, which would multiply the work per insert (a span with 10 attributes and 5 links would otherwise cause 50 rows to be processed instead of 15).

### How the Reader Uses It

The lookup only runs for **string-typed** attribute filters. When the API client supplies `Bool`/`Int`/`Double`/`Bytes`/`Slice`/`Map` directly (e.g., from a programmatic caller, not the search UI), the type is already unambiguous and the query builder goes straight to the matching typed column. The ambiguity-resolution path is reserved for the case the table was built for: a string from the UI search box that could plausibly be any type at any level.

For each string-typed key in the request, the reader looks the key up in `attribute_metadata`:

```sql
SELECT attribute_key, type, level FROM attribute_metadata
WHERE attribute_key IN (?, ?, ...)
GROUP BY attribute_key, type, level
```

The result tells the query builder, for each requested string key, exactly which `(type, level)` combinations have ever been observed. The builder then:

1. Parses the user-supplied string into each observed type (`strconv.ParseBool`, `strconv.ParseInt`, `strconv.ParseFloat`, …).
2. For every `(type, level)` pair that parses successfully, emits one `arrayExists(...)` predicate against the corresponding column (e.g. `int_attributes` at the span level, `resource_str_attributes` at the resource level).
3. `OR`s the predicates together inside a single `AND ( ... )` block per attribute key.

If `attribute_metadata` has no entry for a key (newly-seen key, or the materialized view hasn't caught up yet), or if the supplied value can't be parsed as any of the observed types, the builder falls back to "string at every level" — preserving correctness at the cost of a wider scan. The fallback path is in [`query_builder.go`](../../internal/storage/v2/clickhouse/tracestore/query_builder.go) as `appendStringAttributeFallback`.

---

## Table Schema

### `spans` Table

The `spans` table holds one row per span with all attached metadata. The full DDL is in [`create_spans_table.sql`](../../internal/storage/v2/clickhouse/sql/create_spans_table.sql).

### Partitioning

The `spans` table is partitioned by day on `toDate(start_time)`. Most trace queries are time-bounded, and partition pruning is the cheapest form of pruning available — entire days can be excluded from a query before any granule-level work happens.

### Primary Key (Sort Order)

Two candidate orderings were considered for the `spans` table.

#### Option A: Optimize for Trace Retrieval

Sort by `trace_id`. Every span belonging to the same trace lands in one contiguous block, so `GetTrace` becomes a single seek and one sequential read.

The cost is that search queries (`FindTraces`/`FindTraceIDs`) become expensive. Spans for any given service are scattered across the entire keyspace, so filters on `service_name`, `name`, or `start_time` cannot use the primary-key index — they degrade to scans, mitigated only by skip indexes on those columns (`set` on `service_name` and `name`, `minmax` on `start_time`).

#### Option B: Optimize for Search ✅

Sort by `(service_name, name, toDateTime(start_time))`. Search queries — the dominant interaction in Jaeger — become direct primary-key lookups. The cost is that spans for a single trace are no longer co-located; `GetTrace` must locate the matching rows by other means.

We chose Option B because the trade-off is asymmetric. Sorting by `trace_id` (Option A) makes search performance terrible — `service_name` and `name` filters degrade to scans across the entire keyspace, mitigated only by skip indexes that still need to read every granule's index entry. Sorting by `(service_name, name, start_time)` (Option B) hurts trace retrieval much less: the `bloom_filter` skip index on `trace_id` (see below) lets `GetTraces` skip the vast majority of granules, and the per-trace time-bounds hint that `FindTraceIDs` returns alongside each candidate ID further narrows the partitions and granules that need to be consulted. In benchmarks, Option B's hit on retrieval was small, while Option A's hit on search was severe.

### Secondary (Skip) Indexes

Two skip indexes are defined on the `spans` table:

|Column|Index type|Justification|
|---|---|---|
|`trace_id`|`bloom_filter` (granularity 1)|Trace IDs are high-cardinality "needle in a haystack" lookups — the Bloom filter sweet spot. This is what makes `GetTraces` viable despite sorting by `(service_name, name, start_time)`.|
|`duration`|`minmax` (granularity 1)|The query API exposes `DurationMin`/`DurationMax` filters; `minmax` lets the engine skip granules whose duration range falls entirely outside the query bounds.|

`start_time` does not need a separate `minmax` index: daily partitioning provides coarse time pruning, and the third component of the primary key provides fine-grained pruning within each partition.

### TTL

The `spans` and `trace_id_timestamps` tables accept an optional TTL on their time columns (`start_time` and `end`, respectively). When configured, ClickHouse automatically deletes rows past the TTL during background merges, so retention has no separate write path.

The remaining tables (`services`, `operations`, `attribute_metadata`, `dependencies`) do not carry a TTL. They are small relative to `spans` — they accumulate at most one row per unique `(service)`, `(service, span_kind)`, or `(attribute_key, type, level)` triple — and `AggregatingMergeTree` deduplicates them in the background, so unbounded growth is not a practical concern in the alpha. If a service or attribute key permanently disappears from the workload, its row in these tables will linger; that is an accepted trade-off for the simpler write path. Adding TTL (or a periodic compaction step) to these tables is something we may revisit if long-running deployments accumulate enough stale entries to hurt query-builder performance.

---

## Derived Tables

A few queries don't fit the `spans` sort order or would require expensive scans. These are precomputed by materialized views into dedicated `AggregatingMergeTree` tables that the reader queries directly.

### `services`

Used by `GetServices`. Stores one row per unique service name.

DDL: [`create_services_table.sql`](../../internal/storage/v2/clickhouse/sql/create_services_table.sql), populated by [`create_services_mv.sql`](../../internal/storage/v2/clickhouse/sql/create_services_mv.sql).

### `operations`

Used by `GetOperations`. The composite key `(service_name, span_kind)` covers both query patterns supported by the API: filtering by service alone, or by service + span kind.

DDL: [`create_operations_table.sql`](../../internal/storage/v2/clickhouse/sql/create_operations_table.sql), populated by [`create_operations_mv.sql`](../../internal/storage/v2/clickhouse/sql/create_operations_mv.sql).

### `trace_id_timestamps`

Stores `min(start_time)` and `max(start_time)` per `trace_id`, keyed on `trace_id`.

DDL: [`create_trace_id_timestamps_table.sql`](../../internal/storage/v2/clickhouse/sql/create_trace_id_timestamps_table.sql), populated by [`create_trace_id_timestamps_mv.sql`](../../internal/storage/v2/clickhouse/sql/create_trace_id_timestamps_mv.sql).

This table exists to make the search → fetch handoff cheap. `FindTraceIDs` returns each matching trace ID together with an optional `(start, end)` time range — documented in the v2 reader API as "an optimization hint for some storage backends that can perform more efficient queries when they know the approximate time range." The hint is optional, but populating it lets `GetTraces` skip a large amount of work on the ClickHouse backend specifically:

* **Partition pruning.** `spans` is partitioned by `toDate(start_time)`. Without a time bound, `GetTraces` has to consult every partition still on disk; with one, it touches only the days the trace actually spans (typically one, occasionally two).
* **Primary-key range narrowing.** `spans` is sorted by `(service_name, name, toDateTime(start_time))`. A time range lets the engine restrict the granules it considers within each partition before the `bloom_filter` on `trace_id` is even consulted.

Computing those bounds inside `FindTraceIDs` on the fly would be expensive — `min`/`max` of `start_time` over the matching spans of every candidate trace would scan a large slice of the search window. Instead, the search runs against `spans` to get the candidate trace IDs, then `LEFT JOIN`s `trace_id_timestamps` to attach the precomputed bounds:

The join is cheap because the candidate set is bounded by the search depth (typically a few hundred IDs) and `trace_id_timestamps` is keyed on `trace_id`. If the materialized view hasn't yet observed a freshly-ingested trace, the `LEFT JOIN` simply yields `NULL` bounds and `GetTraces` falls back to the unbounded `bloom_filter` lookup — correct, just slower.

### `attribute_metadata`

Populates the query-time attribute-resolution path. The schema, the role it plays during search, and the rationale for splitting events/links into separate materialized views are covered in [Resolving Attributes at Query Time](#resolving-attributes-at-query-time).

### `dependencies`

A simple `MergeTree` table holding precomputed service-dependency graphs as JSON, partitioned by day on `timestamp`. Populated externally (e.g., by a scheduled job) rather than by a materialized view.

DDL: [`create_dependencies_table.sql`](../../internal/storage/v2/clickhouse/sql/create_dependencies_table.sql).

---

## Consequences

### Positive

* **Search-optimized layout.** Filters on service, operation, and time range are direct primary-key lookups, satisfying the latency target for the dominant interaction.
* **Compact storage.** The `Nested`-based attribute layout, combined with column-oriented compression, delivers high compression ratios on real trace data.
* **Background-driven derived state.** All ancillary tables (`services`, `operations`, `trace_id_timestamps`, `attribute_metadata`) are maintained by materialized views; the writer only inserts into `spans`.
* **Cheap attribute resolution.** The `attribute_metadata` table lets the query builder resolve user-supplied string filters to the correct typed columns and levels without ever scanning `spans`.
* **TTL-driven retention.** Retention is enforced by ClickHouse during background merges; no separate cleanup process is needed.

### Negative / Limitations

* **Trace retrieval is slower than search.** Sorting `spans` by `(service_name, name, start_time)` means spans for a single trace are scattered across the keyspace. `GetTraces` cannot use the primary-key index and instead relies on the `bloom_filter` on `trace_id` plus the time-bounds hint from `FindTraceIDs` to skip granules. This is a deliberate trade-off — the search path benefits much more from primary-key alignment than the retrieval path loses from it — but it does mean retrieval latency is higher than a `trace_id`-sorted layout would deliver.
* **Search depends on a derived table for fast follow-up reads.** `FindTraceIDs` joins against `trace_id_timestamps` to populate the optional time-bounds hint on each result. If the materialized view falls behind under sustained heavy load, freshly-ingested traces come back with `NULL` bounds and the subsequent `GetTraces` call falls back to an unbounded `bloom_filter` scan — correct, but slower until the view catches up.
* **Attribute filters cost more than flat-column filters.** Because attribute keys aren't fixed up-front, every key shares one `Nested` column per type. Filtering by an attribute therefore turns into an `arrayExists(...)` over that row's `Nested` slice rather than a direct comparison on a dedicated column. ClickHouse runs `arrayExists` per row, so it can't be SIMD-vectorized or skip-indexed the way a flat column can. In practice the cost is small (the arrays are short and heavily compressed, and earlier filters usually shrink the row set first), but it is strictly more work than filtering on a dedicated column would be.
* **No TTL on the small derived tables.** Only `spans` and `trace_id_timestamps` carry a TTL. `services`, `operations`, `attribute_metadata`, and `dependencies` accumulate one row per unique entry indefinitely, so a service or attribute key that permanently disappears from the workload will leave a stale row behind.
* **Single-node assumptions.** The schema and benchmarks assume a single ClickHouse node.
