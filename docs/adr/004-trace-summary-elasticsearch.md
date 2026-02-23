# ADR: Trace Summary Index Optimization

## Status

Proposed

## Context

`FindTraceIDs` and single-trace lookups query `jaeger-span-*` indices, which are massive and write-heavy. Searching across a full 24-hour daily index just to find the exact start and end times of a specific trace is highly inefficient and creates significant CPU/IO load on the Elasticsearch cluster.

To solve this, we utilize **Elasticsearch Transforms**. A Transform is a continuous background job running directly inside the Elasticsearch cluster. It constantly monitors the raw span indices, groups incoming spans by `traceID`, and uses a scripted metric (Painless) to calculate the absolute `min_startTime` and `max_endTime` for each trace. 

This aggregated data is pivoted into a tiny, highly-indexed `trace-summary` footprint. By doing this, Jaeger can perform a lightning-fast O(1) lookup on the summary index to get the exact time bounds of a trace *before* querying the raw spans, drastically reducing the search window.

## How the Transform is Managed

On startup, when `UseTraceSummary=true`, the Jaeger factory orchestrates this background job:

1. Derives the summary index name — from `TraceSummaryIndex` if set, otherwise uses prefix-aware logic (`HasSuffix` + `TrimSuffix`) to replace `jaeger-span` with `trace-summary` (e.g. `prod-jaeger-span` → `prod-trace-summary`).
2. Checks if a transform job already exists and whether its description matches the configured version (e.g. `"Jaeger Trace Summary - v1"`).
3. If missing or version-mismatched, deletes the old job and creates a new one using the provided mapping templates.
4. Starts the transform, which continuously aggregates spans from `jaeger-span-*` into the summary index.

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

- [Elasticsearch Transforms Overview](https://www.elastic.co/guide/en/elasticsearch/reference/current/transforms.html)
- [Elasticsearch Scripted Metric Aggregations](https://www.elastic.co/guide/en/elasticsearch/reference/current/search-aggregations-metrics-scripted-metric-aggregation.html)
