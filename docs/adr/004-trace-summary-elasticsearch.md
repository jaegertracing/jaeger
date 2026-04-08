# ADR: Trace Summary Index Optimization

## Status

Proposed

## Context

`FindTraceIDs` queries `jaeger-span-*`, which is large and write-heavy. Elasticsearch's Transform API can maintain a `trace-summary` index with one pre-aggregated document per trace (traceID, start time, end time), which is far cheaper to query.

## How the Index is Set Up

On startup, when `UseTraceSummary=true`, the factory:

1. Derives the summary index name — from `TraceSummaryIndex` if set, otherwise uses prefix-aware logic (`HasSuffix` + `TrimSuffix`) to replace `jaeger-span` with `trace-summary` (e.g. `prod-jaeger-span` → `prod-trace-summary`).
2. Checks if a transform job already exists and whether its description matches the configured version (e.g. `"Jaeger Trace Summary - v1"`).
3. If missing or version-mismatched, deletes the old job and creates a new one.
4. Starts the transform, which continuously aggregates spans from `jaeger-span-*` into the summary index grouped by `traceID`.

The reader then queries this index in `multiRead` instead of the full span index.

## Decision

Three fields added to `Configuration`:

| Field | Type | Default | Purpose |
|-------|------|---------|---------|
| `UseTraceSummary` | `bool` | `false` | Opt-in flag |
| `TraceSummaryIndex` | `string` | `""` (auto-derived) | Override index name |
| `TraceSummaryVersion` | `string` | `"v1"` | Transform schema version |

## Consequences

### Positive

- `FindTraceIDs` queries a purpose-built summary index rather than the full span index, reducing query latency and cluster load.
- Opt-in flag means zero impact on existing deployments.
- Default derivation means no extra configuration needed for standard setups.
- `TraceSummaryVersion` is operator-configurable, allowing controlled schema migrations without a code change.

### Negative

- **Implicit naming contract.** The factory uses prefix-aware logic (`HasSuffix` + `TrimSuffix`) while the reader uses a simpler `strings.Replace`. For standard setups they produce the same result, but they are not identical logic — if one is updated the other may silently diverge, causing reads to query the wrong index.
- **Silent failure on custom prefixes.** If `spanIndexPrefix` does not contain `jaeger-span` and `TraceSummaryIndex` is not set, both derivations return the prefix unchanged with no error or warning.
  ```
  spanIndexPrefix = "acme-spans"
  derived index   = "acme-spans"   ✗ (jaeger-span not found, no replacement)
  fix             = set TraceSummaryIndex: "acme-trace-summary" explicitly
  ```
- **Version bump triggers transform recreation.** `TraceSummaryVersion` defaults to `"v1"` when not set. Changing it causes the factory to delete and recreate the transform job on next startup, causing a temporary gap in the summary index until backfill completes.

### Mitigation

- Extract index name derivation into a shared helper called by both factory and reader — eliminates the divergence risk entirely. Deferred, not blocking this change.
- Add a startup warning when `UseTraceSummary=true`, `spanIndexPrefix` does not contain `jaeger-span`, and `TraceSummaryIndex` is not set explicitly.
- Document the `TraceSummaryVersion` behaviour in the operator guide to prevent unintended transform recreation.

## Alternatives Considered

**Always require explicit `TraceSummaryIndex`.** No naming coupling, but every operator must configure it even for the standard case. Rejected.

**Shared derivation function.** Both factory and reader call the same function — they can never silently diverge. Preferred long-term, not blocking this change.

**Discover index from cluster at startup.** Removes naming assumptions but adds a network call at startup that can fail. Rejected as over-engineered.

## Validation

`TestSpanReader_SummaryOptimization_DefaultIndexName` confirms that with `UseTraceSummary=true`, `TraceSummaryIndex=""`, and `spanIndexPrefix="jaeger-span"`, the reader queries `"trace-summary"` — matching what the factory writes by default.

## References

- `internal/storage/v1/elasticsearch/spanstore/reader.go` — derivation logic, `multiRead`
- `internal/storage/v1/elasticsearch/spanstore/factory.go` — `createTraceSummaryTransform`
- `internal/storage/elasticsearch/config/config.go` — `UseTraceSummary`, `TraceSummaryIndex`, `TraceSummaryVersion`
