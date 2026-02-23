# ADR: Trace Summary Index Optimization

## Status

Proposed

## Context

`FindTraceIDs` and single-trace lookups query `jaeger-span-*` indices, which are massive and write-heavy. Searching across a full 24-hour daily index just to find the exact start and end times of a specific trace is highly inefficient and creates significant CPU/IO load on the Elasticsearch cluster.

To solve this, we utilize **Elasticsearch Transforms**. A Transform is a continuous background job running directly inside the Elasticsearch cluster. It constantly monitors the raw span indices, groups incoming spans by `traceID`, and uses a scripted metric (Painless) to calculate the absolute `min_startTime` and `max_endTime` for each trace.

This aggregated data is pivoted into a tiny, highly-indexed `trace-summary` footprint. By doing this, Jaeger can perform a lightning-fast O(1) lookup on the summary index to get the exact time bounds of a trace *before* querying the raw spans, drastically reducing the search window.

## How the Transform is Managed

On startup, the Jaeger factory orchestrates this background job:

1. Derives the summary index name by replacing the first occurrence of `jaeger-span` with `jaeger-trace-summary` (e.g., `prod-jaeger-span` → `prod-jaeger-trace-summary`).
2. Checks if a transform job already exists and whether its description matches the expected version (`"Jaeger Trace Summary - v1"`).
3. If missing or version-mismatched, deletes the old job and creates a new one using the provided mapping templates.
4. Starts the transform, which continuously aggregates spans from `jaeger-span-*` into the summary index.

The reader then queries this index in `multiRead` instead of the full span index.

### Reader Optimization: How the Reader Consumes Transforms

With the `trace-summary` transform in place, the `SpanReader` fundamentally shifts from a "guess-and-check" querying model to a precise, bounded lookup.

When a user requests a trace by ID, the reader executes the following workflow:
1. **O(1) Boundary Lookup:** The reader first queries the `trace-summary` index using a `TermsQuery` for the requested `traceID`s.
2. **Pruning the Search Space:** The transform returns the exact `min_startTime` and `max_endTime` for those traces. The reader uses these exact timestamps to calculate the precise daily/rollover indices that contain the trace.
3. **Eliminating the Buffer:** Previously, the reader had to pad the search window by +/- 1 hour to account for traces straddling index boundaries. By knowing the exact timestamps, this massive 2-hour buffer is completely eliminated, drastically reducing the number of shards Elasticsearch must search.
4. **Graceful Fallback:** If a trace is too new (within the transform's sync interval) and is not yet in the summary index, the reader gracefully falls back to the legacy behavior, applying the +/- 1 hour buffer to ensure no data is lost.

## Decision

The Jaeger factory automatically provisions and manages an Elasticsearch Transform job on startup.
The transform continuously aggregates spans from `*{prefix}jaeger-span-*` into a lightweight
`{prefix}jaeger-trace-summary` index, storing `min_startTime` and `max_endTime` per `traceID`.

The `SpanReader` queries this summary index before every `multiRead` to obtain precise time bounds,
eliminating the ±1h search buffer. If a trace is absent from the summary index (e.g. too new),
the reader falls back to the legacy ±1h expansion automatically.

## Consequences

### Positive

- `FindTraceIDs` queries a purpose-built summary index rather than the full span index, reducing query latency and cluster load.
- Default derivation means no extra configuration needed for standard setups.

### Negative

- **Silent failure on custom prefixes.** If the user configures a `spanIndexPrefix` that does not contain the string `jaeger-span`, the derivation will return the prefix unchanged. The reader and transform will still function, but the index won't be clearly labeled as a summary index.

## Alternatives Considered

**Shared derivation function.** Both factory and reader call the same function — they can never silently diverge. Preferred long-term, not blocking this change.

**Discover index from cluster at startup.** Removes naming assumptions but adds a network call at startup that can fail. Rejected as over-engineered.

## Validation

`TestSpanReader_SummaryOptimization_DefaultIndexName` confirms that the reader derives and queries the correct `"trace-summary"` index — matching what the factory writes by default.

## References

- [Elasticsearch Transforms Overview](https://www.elastic.co/guide/en/elasticsearch/reference/current/transforms.html)
- [Elasticsearch Scripted Metric Aggregations](https://www.elastic.co/guide/en/elasticsearch/reference/current/search-aggregations-metrics-scripted-metric-aggregation.html)
