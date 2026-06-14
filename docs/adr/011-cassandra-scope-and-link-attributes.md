# ADR-011: Cassandra Schema Extension for Scope and Link Attributes

- **Status**: Proposed
- **Date**: 2026-06-14

## Context

### Background

Jaeger's Cassandra backend is being migrated to the storage v2 API (tracked in issue [#8080](https://github.com/jaegertracing/jaeger/issues/8080)), which involves moving from the legacy `ptrace.Traces` ↔ `model.Trace` ↔ `dbmodel.Span` pipeline to a direct `ptrace.Traces` ↔ `dbmodel.Span` conversion.

Most of the v2 migration work has been completed. The outstanding item is:

> Fix `dbmodel` having no space to store scope and link attributes.

This ADR addresses the schema design decisions required before implementing that fix.

### What Are Scope and Link Attributes?

The OpenTelemetry (OTLP) data model includes two first-class concepts that the current Cassandra `dbmodel.Span` struct cannot represent:

**Instrumentation Scope** (`ptrace.ScopeSpans`): Every span belongs to an instrumentation scope, which carries:
- `scope.name` — the name of the instrumentation library (e.g. `go.opentelemetry.io/otel`)
- `scope.version` — the version of the instrumentation library
- `scope.attributes` — a map of arbitrary key-value attributes on the scope itself

**Span Links** (`ptrace.SpanLinkSlice`): A span may contain zero or more links to causally-related spans in other traces, each with:
- `trace_id` and `span_id` identifying the referenced span
- `trace_state`
- `attributes` — a key-value attribute map on the link itself
- `flags`

### Current State of Cassandra's dbmodel

The current `dbmodel.Span` struct (in `plugin/storage/cassandra/spanstore/dbmodel/model.go`) and the backing CQL schema (`internal/storage/v1/cassandra/schema/v004.cql.tmpl`) have **no columns** for either scope fields or span links. The existing `tags` column stores span-level key-value attributes as `frozen<list<tag>>`, but there is no analogous structure for scope attributes or for any part of a span link.

As a result, any scope or link data present in `ptrace.Traces` is silently dropped during the `toDBModel` conversion. This means the Cassandra backend cannot provide a lossless round-trip for OTLP trace data.

### Why This Requires an ADR

Adding scope and link storage to Cassandra requires **CQL schema changes** — either new columns on the existing `traces` table or a new schema version. This is a **breaking change** for existing Cassandra deployments: users who upgrade Jaeger without migrating their schema will encounter errors when reading or writing rows that use new columns.

The schema changes must be agreed upon by maintainers before implementation begins, because:

1. The design has lasting consequences on the on-disk layout and cannot easily be changed later.
2. A migration path for existing Cassandra clusters must be defined.
3. The approach must be consistent with how other Jaeger backends (ClickHouse, Elasticsearch/OpenSearch) handle the same OTLP fields.

### How Other Backends Handle This

**ClickHouse** (ADR-008, PRs [#7619](https://github.com/jaegertracing/jaeger/pull/7619), [#7627](https://github.com/jaegertracing/jaeger/pull/7627)): Uses `Nested` columns — one per primitive attribute type — repeated at the span, event, link, resource, and scope levels. Each Nested column behaves as a sub-table within a row, storing parallel arrays of keys and typed values (string, int, bool, float, bytes). Scope and link attributes are native first-class columns in the spans table.

**Elasticsearch/OpenSearch**: Stores all attributes in a flat JSON document per span, where scope fields are stored as top-level keys (e.g. `scope.name`, `scope.version`) and span links are stored as a nested JSON array.

**Cassandra's current approach**: Uses a `frozen<list<tag>>` structure for span-level tags, where each `tag` is a UDT with `key`, `value`, and `type` fields. The same pattern could naturally be extended to scope attributes and link attributes.

---

## Decision

### Schema Approach: New Columns on a New Schema Version

We will add the necessary scope and link fields as **new columns on the existing `traces` table**, delivered as a **new schema version (`v005.cql.tmpl`)**.

The following columns will be added:

**Scope fields** (stored directly on the span row, since each span belongs to exactly one scope):

```cql
-- Scope identity and attributes
scope_name       text
scope_version    text
scope_attributes frozen<list<tag>>
```

**Span links** (stored as a frozen list of a new `link` UDT, since a span may have zero or more links):

```cql
-- New UDT to represent a single span link
CREATE TYPE IF NOT EXISTS link (
    trace_id    blob,
    span_id     blob,
    trace_state text,
    attributes  frozen<list<tag>>,
    flags       int
);

-- New column on the traces table
links frozen<list<link>>
```

The existing `tag` UDT already used by the `tags` column (`key text, value text, type text`) will be reused for `scope_attributes` and `link.attributes` to maintain consistency.

### Schema Versioning and Migration

A new schema template `v005.cql.tmpl` will be created. The existing `create.sh` script selects the template by Cassandra major version; a new condition will be added to use v005 for Cassandra 5.x and as the default for new installations.

For **existing deployments** upgrading from v004: an `ALTER TABLE` migration will be provided in the schema README, adding the new columns to an existing keyspace:


CREATE TYPE IF NOT EXISTS link (
    trace_id    blob,
    span_id     blob,
    trace_state text,
    attributes  frozen<list<tag>>,
    flags       int
);

ALTER TABLE ${keyspace}.traces ADD scope_name text;
ALTER TABLE ${keyspace}.traces ADD scope_version text;
ALTER TABLE ${keyspace}.traces ADD scope_attributes frozen<list<tag>>;
ALTER TABLE ${keyspace}.traces ADD links frozen<list<link>>;


> **Note**: Cassandra supports `ALTER TABLE ... ADD` for non-primary-key columns, so this migration does not require a table rebuild. Existing rows will return `null` for the new columns, which the reader must handle gracefully by treating `null` as empty/absent.

### Go Model Changes

The `dbmodel.Span` struct will be extended with the corresponding Go fields:

```go
// in plugin/storage/cassandra/spanstore/dbmodel/model.go

type Span struct {
    // ... existing fields ...

    // Instrumentation scope
    ScopeName       string `db:"scope_name"`
    ScopeVersion    string `db:"scope_version"`
    ScopeAttributes []Tag  `db:"scope_attributes"`

    // Span links
    Links []SpanLink `db:"links"`
}

type SpanLink struct {
    TraceID    dbmodel.TraceID `db:"trace_id"`
    SpanID     dbmodel.SpanID  `db:"span_id"`
    TraceState string          `db:"trace_state"`
    Attributes []Tag           `db:"attributes"`
    Flags      int32           `db:"flags"`
}
```

The `Tag` type already exists and is reused for attributes at both the scope and link level.

### Converter Changes

`toDBModel` (in `plugin/storage/cassandra/spanstore/dbmodel/converter.go`) will be extended to populate the new fields from the `ptrace.Span` and its parent `ptrace.ScopeSpans`.

`fromDBModel` will be extended to reconstruct scope and link data from the new columns, treating `nil` slices (from old rows) as empty.

### Handling Old Rows

Rows written before the schema migration will have `null` for all new columns. The reader will check for `nil` and treat it as:
- `ScopeName` / `ScopeVersion` → empty string
- `ScopeAttributes` → empty attribute map
- `Links` → empty span link slice

This ensures backward read compatibility with data written by older Jaeger versions.

---

## Alternatives Considered

### Option A: Serialize Scope and Links as Blob/JSON

Store scope and link data as a single opaque `blob` or `text` column containing serialized JSON or protobuf.

**Rejected because**: This would avoid a schema change, but would make the data opaque to CQL queries and would be inconsistent with how the existing `tags` column works. It also complicates future querying capabilities and round-trip fidelity testing.

### Option B: New Separate Tables for Links

Create a separate `span_links` table keyed on `(trace_id, span_id)` and join at read time.

**Rejected because**: Cassandra does not support server-side joins. Fetching links would require additional round-trip queries per span, significantly increasing read latency. The current Cassandra model colocates all span data in a single row; this pattern should be preserved for links, which are integral to a span's data.

### Option C: Encode Scope/Links Inside Existing `tags` Column Using Namespaced Keys

Encode scope name as a synthetic tag like `_scope.name`, scope version as `_scope.version`, and links as `_link.0.trace_id` etc., all stored in the existing `tags` list.

**Rejected because**: This pollutes the user-visible span tag namespace with internal synthetic keys, would interfere with tag filtering queries, and results in a fragile encoding that's hard to evolve. It also provides no type safety for link fields such as `trace_id` (a blob) vs. a text key.

---

## Consequences

### Positive

1. **Full OTLP round-trip fidelity**: Scope name, scope version, scope attributes, and span links will be preserved end-to-end in the Cassandra backend, matching the data model used by other Jaeger backends.
2. **Consistent with existing patterns**: Reusing the `tag` UDT for scope and link attributes is consistent with the existing span-level tag storage, keeping the schema familiar.
3. **Backward read compatibility**: Old rows return `null` for new columns; readers handle this gracefully, so existing data continues to be readable without data migration.
4. **Non-destructive migration**: `ALTER TABLE ... ADD` does not require a table rebuild or downtime in Cassandra 4.x/5.x.
5. **Queryable in future**: Storing scope and link attributes as structured columns leaves room for future indexing or filtering capabilities, unlike blob encoding.

### Negative

1. **Breaking schema change**: Existing deployments must run the `ALTER TABLE` migration before upgrading to the Jaeger version that writes these fields. Deployments that upgrade Jaeger without migrating the schema will encounter errors on write (attempting to set a non-existent column).
2. **Write overhead**: Adding new columns increases the size of each row, slightly increasing write and storage costs for deployments that use instrumentation scopes or span links heavily.
3. **No backfill of historical data**: Spans written before the migration will permanently lack scope and link data. There is no practical way to backfill this information.
4. **Frozen list limitation**: Using `frozen<list<link>>` means the entire link list must be read and written atomically. Partial updates to individual links are not possible, but this is acceptable given that span data in Jaeger is immutable after write.

---

## Migration Guidance for Operators

Operators upgrading an existing Cassandra-backed Jaeger deployment must:

1. **Before upgrading Jaeger**: Run the following CQL against each keyspace used by Jaeger:

```cql
USE ${keyspace};

CREATE TYPE IF NOT EXISTS link (
    trace_id    blob,
    span_id     blob,
    trace_state text,
    attributes  frozen<list<tag>>,
    flags       int
);

ALTER TABLE traces ADD scope_name       text;
ALTER TABLE traces ADD scope_version    text;
ALTER TABLE traces ADD scope_attributes frozen<list<tag>>;
ALTER TABLE traces ADD links            frozen<list<link>>;
```

2. **After the migration**, upgrade Jaeger. The new version will write scope and link data to the new columns.

The schema README (`internal/storage/v1/cassandra/schema/README.md`) will be updated with a new entry for this migration.

For deployments using Jaeger's automatic schema creation (`schema.create: true`), the new schema version will include these columns from the start and no manual migration is needed.

---

## References

- Issue: [#8080 — Upgrade Cassandra backend to storage v2 API](https://github.com/jaegertracing/jaeger/issues/8080)
- Parent issue: [#6458 — Storage v2 API](https://github.com/jaegertracing/jaeger/issues/6458)
- ClickHouse scope attributes PR: [#7619](https://github.com/jaegertracing/jaeger/pull/7619)
- ClickHouse complex/link attributes PR: [#7627](https://github.com/jaegertracing/jaeger/pull/7627)
- ClickHouse ADR: [docs/adr/008-clickhouse-storage-schema.md](./008-clickhouse-storage-schema.md)
- Cassandra duration query ADR: [docs/adr/001-cassandra-find-traces-duration.md](./001-cassandra-find-traces-duration.md)
- Relevant files:
  - `plugin/storage/cassandra/spanstore/dbmodel/model.go` — `Span` struct
  - `plugin/storage/cassandra/spanstore/dbmodel/converter.go` — `toDBModel` / `fromDBModel`
  - `internal/storage/v1/cassandra/schema/v004.cql.tmpl` — current CQL schema template
  - `internal/storage/v1/cassandra/schema/README.md` — schema version history