# Badger SpanKind Index

* **Status**: Proposed
* **Date**: 2026-04-18

## Context

Badger's GetOperations cannot return or filter by spanKind, tracked in #1922.
The existing operation index (`0x82`) encodes `[serviceName+operationName]`
as a concatenated byte sequence with no field boundary. spanKind is absent
from the key entirely.

Badger is a pure key-value store: `f: K → V`. Unlike relational storage,
there is no catalog separating field definitions from data and no JOIN to
reinterpret keys at query time. Unlike document stores, values are opaque
bytes with no queryable structure.

The key is the only schema carrier. Each key prefix defines a separate
logical schema, structurally equivalent to a table or index in a
relational store (see [ADR-005](005-badger-storage-record-layouts.md)).
Schema changes that touch the key require either rewriting existing
entries or maintaining multiple key formats simultaneously.

A prior attempt (#6376) tried to modify `0x82` in place with spanKind
embedded in the key, requiring migration or breaking backward
compatibility.

## Decision

Introduce a new schema under key prefix `0x85` that encodes spanKind as a
fixed-width byte between serviceName and operationName:

```text
[0x85][serviceName][kindByte][operationName][startTime][traceID]
```

The fixed-width `kindByte` sits at a known position after the service
name and enables efficient kind-filtered prefix scans on
`[0x85][service][kindByte]`. This index replaces `0x82` through a
phased rollout (see [Rollout](#rollout)).

## Rationale

### Why a new prefix instead of modifying 0x82

`0x82` encodes `[serviceName+operationName]` as a concatenated byte
sequence with no field boundary, so spanKind cannot be inserted without
either rewriting every entry or introducing ambiguity in existing keys.
A new operation index under key prefix `0x85` sidesteps both problems
and keeps existing data valid without requiring migration.

### Why kindByte before operationName

Two orderings are possible:

1. `[service][kindByte][operation]` — optimizes `GetOperations(service, kind)`
2. `[service][operation][kindByte]` — optimizes `FindTraceIDs(service, operation)`

Ordering 1 is chosen because `GetOperations` filtered by kind is the
primary motivation of #1922, and the cache absorbs `FindTraceIDs` kind
resolution at query time without requiring ordering 2's key layout.

Operation name stays in the key so that the cache preload can recover
both fields from a single scan.

### Why fixed-width byte

`spanKind` is a closed enum with six values (OTel proto 0-5), so one byte
is sufficient.

## Compatibility

### Trace search

`FindTraceIDs` migrates from scanning `0x82` directly to resolving
kinds through the cache and seeking `0x85`. The cache holds the kinds
that actually exist for each `(service, operation)` pair, so resolution
is bounded by real data rather than the six-value spanKind enum. The
common case stays at one seek per query.

### GetOperations on old data

Legacy entries written before `0x85` existed are not visible in
kind-filtered queries. They drain via TTL as spans expire.

### Cache schema change

The in-memory cache key changes from `string` (operation name) to
`tracestore.Operation{Name, SpanKind}`. All writes and reads are updated
to use the struct key. No on-disk format change to the cache (cache is
rebuilt on startup).

## Rollout

### Legacy operation index deprecation

A feature flag `use_new_operation_index` controls adoption of the `0x85`
operation index, following the OTel Collector feature gate lifecycle:

| Stage   | Gate behavior                    | `0x85` operation index                    |
| ------- | -------------------------------- | ----------------------------------------- |
| Alpha   | off by default, can enable       | New index is opt-in                       |
| Beta    | on by default, can disable       | New index is opt-out                      |
| Stable  | permanently on, cannot disable   | New index is always on                    |
| Removed | gate deleted                     | Legacy code not possible                  |

When the gate is on, the writer emits only `0x85`, the reader preloads
only from `0x85`, and `FindTraceIDs` resolves kinds via the cache before
seeking `0x85`. When off, both indexes are written and read, and
`FindTraceIDs` scans `0x82` directly.

Phase durations should exceed the configured span TTL so pre-gate data
drains before the next phase tightens.

### Integration test workaround removed

The `GetOperationsMissingSpanKind` flag in `badgerstore_test.go` is removed, bringing Badger in line with other backends.
