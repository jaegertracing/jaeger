# Cassandra FindTraceIDs Duration Query Behavior

## Status

Accepted

## Context

The Cassandra spanstore implementation in Jaeger handles trace queries with duration filters (DurationMin/DurationMax) through a separate code path that cannot efficiently intersect with other query parameters like tags or general operation name filters. This behavior differs from other storage backends like Badger and may seem counterintuitive to users.

### Data Model and Cassandra Constraints

Cassandra's data model imposes specific constraints on query patterns. The `duration_index` table is defined with the following schema structure (as referenced in the CQL insertion query in [`internal/storage/v1/cassandra/spanstore/writer.go`](../../internal/storage/v1/cassandra/spanstore/writer.go)):

```cql
INSERT INTO duration_index(service_name, operation_name, bucket, duration, start_time, trace_id)
VALUES (?, ?, ?, ?, ?, ?)
```

This schema uses a composite partition key consisting of `service_name`, `operation_name`, and `bucket` (an hourly time bucket), with `duration` as a clustering column. In Cassandra, **partition keys require equality constraints** in WHERE clauses - you cannot perform range queries or arbitrary intersections across different partition keys efficiently.

### Duration Index Structure

The duration index is bucketed by hour to limit partition size and improve query performance. From [`internal/storage/v1/cassandra/spanstore/writer.go`](../../internal/storage/v1/cassandra/spanstore/writer.go) (line 57):

```go
durationBucketSize = time.Hour
```

When a span is indexed, its start time is rounded to the nearest hour bucket (line 231 in writer.go):

```go
timeBucket := startTime.Round(durationBucketSize)
```

The indexing function in `indexByDuration` (lines 229-243) creates two index entries per span:
1. One indexed by service name alone (with empty operation name)
2. One indexed by both service name and operation name

```go
indexByOperationName("")                 // index by service name alone
indexByOperationName(span.OperationName) // index by service name and operation name
```

### Query Path Implementation

In [`internal/storage/v1/cassandra/spanstore/reader.go`](../../internal/storage/v1/cassandra/spanstore/reader.go), the `findTraceIDs` method (lines 275-301) performs an early return when duration parameters are present:

```go
func (s *SpanReader) findTraceIDs(ctx context.Context, traceQuery *spanstore.TraceQueryParameters) (dbmodel.UniqueTraceIDs, error) {
	if traceQuery.DurationMin != 0 || traceQuery.DurationMax != 0 {
		return s.queryByDuration(ctx, traceQuery)
	}
	// ... other query paths
}
```

This early return means that when a duration query is detected, **all other query parameters except ServiceName and OperationName are effectively ignored** (tags, for instance, are not processed).

The `queryByDuration` method (lines 333-375) iterates over hourly buckets within the query time range and issues a Cassandra query for each bucket:

```go
startTimeByHour := traceQuery.StartTimeMin.Round(durationBucketSize)
endTimeByHour := traceQuery.StartTimeMax.Round(durationBucketSize)

for timeBucket := endTimeByHour; timeBucket.After(startTimeByHour) || timeBucket.Equal(startTimeByHour); timeBucket = timeBucket.Add(-1 * durationBucketSize) {
	query := s.session.Query(
		queryByDuration,
		timeBucket,
		traceQuery.ServiceName,
		traceQuery.OperationName,
		minDurationMicros,
		maxDurationMicros,
		traceQuery.NumTraces*limitMultiple)
	// execute query...
}
```

Each query specifies exact values for `bucket`, `service_name`, and `operation_name` (the partition key components), along with a range filter on `duration` (the clustering column). The query definition (lines 51-55) is:

```cql
SELECT trace_id
FROM duration_index
WHERE bucket = ? AND service_name = ? AND operation_name = ? AND duration > ? AND duration < ?
LIMIT ?
```

### Why Not Intersect with Other Indices?

Unlike storage backends such as Badger (which can perform hash-joins and arbitrary index intersections), Cassandra's partition-based architecture makes cross-index intersections expensive and impractical:

1. **Partition key constraints**: The duration index requires equality on `(service_name, operation_name, bucket)`. You cannot efficiently query across multiple operations or join with the tag index without scanning many partitions.
   
2. **No server-side joins**: Cassandra does not support server-side joins. To intersect duration results with tag results, the client would need to:
   - Query the duration index for all matching trace IDs
   - Query the tag index for all matching trace IDs
   - Perform a client-side intersection
   
   This would be inefficient for large result sets and would require fetching potentially many trace IDs over the network.

3. **Hourly bucket iteration**: The duration query already iterates over hourly buckets. Adding tag intersections would multiply the number of queries and result sets to merge.

### Comparison with Badger

The Badger storage backend handles duration queries differently. In [`internal/storage/v1/badger/spanstore/reader.go`](../../internal/storage/v1/badger/spanstore/reader.go) (around line 486), the `FindTraceIDs` method performs duration queries and then uses the results as a filter (`hashOuter`) that can be intersected with other index results:

```go
if query.DurationMax != 0 || query.DurationMin != 0 {
	plan.hashOuter = r.durationQueries(plan, query)
}
```

Badger uses an embedded key-value store where range scans and in-memory filtering are efficient, allowing it to merge results from multiple indices. This is a fundamental difference from Cassandra's distributed, partition-oriented design.

## Decision

**The Cassandra spanstore will continue to treat duration queries as a separate query path that does not intersect with tag indices or other non-service/operation filters.**

When a `TraceQueryParameters` contains `DurationMin` or `DurationMax`:
- The query will use the `duration_index` table exclusively
- Only `ServiceName` and `OperationName` parameters will be respected (used as partition key components)
- Tag filters and other parameters will be ignored
- The code will iterate over hourly time buckets within the query time range

This approach is documented in code comments and in this ADR to set proper expectations.

## Consequences

### Positive

1. **Performance**: Duration queries execute efficiently by scanning only relevant Cassandra partitions (scoped to service, operation, and hourly bucket).
2. **Scalability**: The bucketed partition strategy prevents hot partitions and distributes load across the cluster.
3. **Simplicity**: The implementation is straightforward and leverages Cassandra's strengths (partition-scoped queries with range filtering on clustering columns).

### Negative

1. **Limited query expressiveness**: Users cannot combine duration filters with tag filters in a single query. They must choose one or the other.
2. **Expectation mismatch**: Users familiar with other backends (like Badger) may expect duration and tags to be combinable.
3. **Workarounds required**: Applications that need both duration and tag filtering must:
   - Issue separate queries (one with duration, one with tags)
   - Perform client-side intersection of results
   - Or use a different storage backend that supports combined queries

### Guidance for Users

- **When using Cassandra spanstore**: Be aware that specifying `DurationMin` or `DurationMax` will cause tag filters to be ignored. Validate that `ErrDurationAndTagQueryNotSupported` is returned if both are specified (enforced in `validateQuery` at line 227-229 in reader.go).
  
- **For combined filtering needs**: Consider using the Badger backend, or implement client-side filtering by:
  1. Querying with duration filters to get a candidate set of trace IDs
  2. Fetching those traces
  3. Filtering the results by tag values in your application code

- **Query design**: Structure queries to leverage the indices available. Use `ServiceName` and `OperationName` in conjunction with duration queries for best results.

## References

- Implementation files:
  - [`internal/storage/v1/cassandra/spanstore/reader.go`](../../internal/storage/v1/cassandra/spanstore/reader.go) - Query logic and duration query path
  - [`internal/storage/v1/cassandra/spanstore/writer.go`](../../internal/storage/v1/cassandra/spanstore/writer.go) - Duration index schema and insertion logic
  - [`internal/storage/v1/badger/spanstore/reader.go`](../../internal/storage/v1/badger/spanstore/reader.go) - Badger implementation for comparison

- Cassandra documentation:
  - [Cassandra Data Modeling](https://cassandra.apache.org/doc/latest/data_modeling/index.html)
  - [CQL Partition Keys and Clustering Columns](https://cassandra.apache.org/doc/latest/cql/ddl.html#partition-key)

- Related code:
  - `durationIndex` constant (writer.go line 47-50): CQL insert statement
  - `queryByDuration` constant (reader.go line 51-55): CQL select statement
  - `durationBucketSize` constant (writer.go line 57): Hourly bucketing
  - Error `ErrDurationAndTagQueryNotSupported` (reader.go line 77): Validation that prevents combining duration and tag queries
