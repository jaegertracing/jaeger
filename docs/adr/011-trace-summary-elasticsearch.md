# ADR-011: Trace Summary Index Optimization

## Status

Accepted

## Context

Single-trace lookups (`GetTrace`) query `jaeger-span-*` indices, which are massive and write-heavy. Searching across a full 24-hour daily index just to find the exact start and end times of a specific trace is highly inefficient and creates significant CPU/IO load on the Elasticsearch cluster.

To solve this, we utilize **Elasticsearch Transforms**. A Transform is a continuous background job running directly inside the cluster. It constantly monitors the raw span indices, groups incoming spans by `traceID`, and uses a scripted metric (Painless) to calculate the absolute `min_startTime` and `max_endTime` for each trace.

This aggregated data is pivoted into a tiny, highly-indexed `trace-summary` footprint. By doing this, Jaeger can perform a lightning-fast O(1) lookup on the summary index to get the exact time bounds of a trace *before* querying the raw spans, drastically reducing the search window.

Furthermore, this index schema is designed as the foundational backend for the API v3 `FindTraceSummaries` method. The scripted metric can be extended to aggregate `span_count`, `distinct_services`, and `error_count` into the pivot table, allowing Jaeger to instantly serve trace summaries without returning full traces.

*Note: OpenSearch handles index transforms under `/_plugins/_transform`, while Elasticsearch uses `/_transform`. The Jaeger storage factory dynamically routes to the correct API path based on the detected backend.*

## How the Transform is Managed

On startup, the Jaeger factory orchestrates this background job:

1. Derives the summary index name safely by appending `jaeger-trace-summary` to the configured index prefix (e.g., a `prod-` prefix cleanly becomes `prod-jaeger-trace-summary`).
2. Checks if a transform job already exists and whether its description matches the expected version (`"Jaeger Trace Summary - v1"`).
3. If missing or version-mismatched, deletes the old job and creates a new one using the provided mapping templates.
4. Starts the transform, which continuously aggregates spans from `jaeger-span-*` into the summary index.

The reader then queries this index in `multiRead` instead of the full span index.

### Reader Optimization: How the Reader Consumes Transforms

With the `trace-summary` transform in place, the `SpanReader` utilizes a precise, bounded lookup model.

When a user requests a trace by ID, the reader executes the following workflow:
1. **O(1) Boundary Lookup:** The reader queries the `trace-summary` index using a `TermsQuery` for the requested `traceID`s.
2. **Pruning the Search Space:** The transform returns the exact `min_startTime` and `max_endTime` for those traces. The reader uses these exact timestamps to calculate the precise daily/rollover indices that contain the trace.
3. **Precision Buffering:** The reader pads these exact timestamps with a tight ±10-minute safety margin (to account for clock skew and transform sync delay), drastically reducing the number of shards Elasticsearch must search.
4. **Graceful Fallback:** If a trace is absent from the summary index (e.g., it is too new and falls within the transform's sync delay interval), the reader gracefully falls back to a wider ±1-hour expansion buffer to ensure no data is lost.

## Decision

The Jaeger factory automatically provisions and manages an Elasticsearch Transform job on startup. The transform continuously aggregates spans into a lightweight `{prefix}jaeger-trace-summary` index, storing `min_startTime` and `max_endTime` per `traceID`.

The `SpanReader` queries this summary index before every `multiRead` to obtain precise time bounds. This architectural shift significantly limits the search space required for trace retrieval, falling back to wider temporal queries only when summary data is not yet available.

## Consequences

### Positive

- `GetTrace` / `multiRead` queries a purpose-built summary index rather than the full span index, reducing query latency and cluster load.
- Default derivation safely handles custom prefixes out-of-the-box, meaning no extra configuration is needed for standard or custom deployments.

### Negative

- **Background Overhead:** The continuous transform job requires a small amount of dedicated CPU/Memory overhead on the Elasticsearch/OpenSearch cluster to perform the background aggregations.

## Alternatives Considered

**Shared derivation function.** Both factory and reader use parallel string manipulation to derive the index name. A shared helper is the preferred long-term solution to ensure they never silently diverge, but is deferred to avoid blocking this architectural change.

**Discover index from cluster at startup.** Removes naming assumptions but adds a network call at startup that can fail. Rejected as over-engineered.

## Validation

`TestSpanReader_SummaryOptimization_DefaultIndexName` confirms that the reader derives and queries the correct `"trace-summary"` index — matching what the factory writes by default.

### Known Limitations
- Index name derivation is duplicated between the factory and reader. A shared helper
  is the preferred long-term solution and is deferred as a follow-up; operators can
  work around divergence by setting `TraceSummaryIndex` explicitly.
- `loadMapping` silently returns an empty string on file read errors. This is a
  pre-existing limitation shared across all mappings and is tracked for cleanup separately.

## References

- [Elasticsearch Transforms Overview](https://www.elastic.co/guide/en/elasticsearch/reference/current/transforms.html)
- [Elasticsearch Scripted Metric Aggregations](https://www.elastic.co/guide/en/elasticsearch/reference/current/search-aggregations-metrics-scripted-metric-aggregation.html)
