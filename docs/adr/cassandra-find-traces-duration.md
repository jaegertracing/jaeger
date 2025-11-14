# ADR 0001: Cassandra duration queries for FindTraceIDs

Status: Accepted

Date: 2025-11-13

## Context

Jaeger's v1 span query API (spanstore.TraceQueryParameters) supports specifying a time window, optional tags, operation name, and optional duration range (DurationMin/DurationMax). Different storage backends index spans differently.

In the Cassandra v1 spanstore implementation, `FindTraceIDs` currently treats queries that include duration bounds specially: if duration is specified, the implementation follows a dedicated code path that queries the duration index and does not combine (intersect) results with other non-duration indices in the same way the other backends do.

This ADR explains why the Cassandra backend behaves this way and documents the design rationale and consequences.

## Decision

We document and codify the rationale: Cassandra's duration index is modeled in CQL with partition keys and clustering columns such that efficient duration-range scans are only possible when the partition key values are provided (service_name, operation_name, bucket). Because of Cassandra's query model (equality required for partition keys), the duration index cannot be used as a generic inverted index to efficiently intersect with other indices (e.g. tag_index) across arbitrary partitions. Therefore, the Cassandra spanstore handles duration queries using a dedicated, partition-scoped scan of the duration index (per service/operation/bucket), and the FindTraceIDs code path for duration queries returns results produced by that index rather than attempting an arbitrary index intersection.

We also record the alternative (supported by other backends like Badger) where duration-index results are used as a hash filter to intersect with other index results — that approach is not practical for the Cassandra schema.

## Consequences

- When a v1 TraceQueryParameters includes DurationMin or DurationMax, the Cassandra backend will execute a duration-index query path (it will iterate time buckets and query the duration_index partition(s)) and this path will effectively be the driver of results. Other parameters may not be intersected in the same way they are in the other backend implementations.
- For accurate filtering, callers should ensure serviceName and (optionally) operationName are provided with duration queries. The duration index is partitioned by (service_name, operation_name, bucket), and the reader queries per bucket and per (service, operation).
- Users should be aware that behavior can differ across storage backends: e.g., Badger supports combining duration results with other indices via additional scan/join logic, while Cassandra uses a partition-scoped duration index scan.
- If future schema/design changes are made to Cassandra storage (e.g. different indices, materialized views, or secondary indexing strategies), the query design here can be revisited.

## References and Evidence

1. Cassandra reader early-return for duration queries (FindTraceIDs)
   - Path: internal/storage/v1/cassandra/spanstore/reader.go
   - Key snippet:
   ```go
   func (s *SpanReader) findTraceIDs(ctx context.Context, traceQuery *spanstore.TraceQueryParameters) (dbmodel.UniqueTraceIDs, error) {
       if traceQuery.DurationMin != 0 || traceQuery.DurationMax != 0 {
           return s.queryByDuration(ctx, traceQuery)
       }
       ...
   }
   ```
   This shows duration queries take a separate path and return results from `queryByDuration`.

2. Duration index writer and schema shape
   - Path: internal/storage/v1/cassandra/spanstore/writer.go
   - CQL insert statement used by writer:
   ```sql
   INSERT INTO duration_index(service_name, operation_name, bucket, duration, start_time, trace_id)
     VALUES (?, ?, ?, ?, ?, ?)
   ```
   - The writer stores two rows per span: one with operation_name set to `""` (service-only) and one with the actual operation name. Time bucketing is used:
   ```go
   timeBucket := startTime.Round(durationBucketSize) // durationBucketSize == time.Hour
   q1 := query.Bind(span.Process.ServiceName, operationName, timeBucket, span.Duration, span.StartTime, span.TraceID)
   ```
   The writer uses hourly buckets for the index.

3. Schema partitioning constraint (duration_index)
   - The CQL schema for duration_index uses a composite partition key ((service_name, operation_name, bucket)) with clustering columns (duration, start_time, trace_id). Example CQL (in project schema):
   ```
   PRIMARY KEY ((service_name, operation_name, bucket), duration, start_time, trace_id)
   WITH CLUSTERING ORDER BY (duration DESC, start_time DESC)
   ```
   Because partition key columns must be specified (equality) for efficient queries, the duration index is only practically queryable within a given (service, operation, bucket) partition.

4. Reader queries per bucket
   - Path: internal/storage/v1/cassandra/spanstore/reader.go (queryByDuration)
   - The reader iterates over the hourly buckets in the requested time range and issues a `queryByDuration` for each bucket:
   ```go
   // startTimeByHour := traceQuery.StartTimeMin.Round(durationBucketSize)
   // endTimeByHour := traceQuery.StartTimeMax.Round(durationBucketSize)
   for timeBucket := endTimeByHour; timeBucket.After(startTimeByHour) || timeBucket.Equal(startTimeByHour); timeBucket = timeBucket.Add(-1 * durationBucketSize) {
       query := s.session.Query(
           queryByDuration,
           timeBucket,
           traceQuery.ServiceName,
           traceQuery.OperationName,
           minDurationMicros,
           maxDurationMicros,
           traceQuery.NumTraces*limitMultiple)
       t, err := s.executeQuery(childSpan, query, s.metrics.queryDurationIndex)
       ...
   }
   ```
   This shows the scan is scoped by the time bucket and service/operation.

5. Contrasting Badger implementation
   - Path: internal/storage/v1/badger/spanstore/reader.go
   - The Badger backend can run a duration index range scan and then use the returned TraceIDs as a hash filter to intersect with other index scans; this is possible because Badger is a KV store under our control and supports arbitrary scans and custom in-process joins.

## Suggested guidance for callers

- Prefer providing serviceName (and operationName if applicable) with duration-range queries against the Cassandra backend.
- Expect slight behavioral differences between backends for complex queries that include duration + tags; be explicit in queries to reduce ambiguity.
- If unified behavior across backends is required, consider a server-side coordination that explicitly combines indices in application code (but note cost implications).

## Related code paths

- reader: internal/storage/v1/cassandra/spanstore/reader.go (findTraceIDs, queryByDuration)
- writer: internal/storage/v1/cassandra/spanstore/writer.go (indexByDuration, durationIndex)
- contrast: internal/storage/v1/badger/spanstore/reader.go (durationQueries, indexSeeksToTraceIDs)
