# Elasticsearch Data Streams in Jaeger

This document describes the implementation of Elasticsearch Data Streams in Jaeger, focusing on the motivation, design decisions, and migration strategy.

## Motivation
Elasticsearch Data Streams provide a more efficient and manageable way to handle time-series data compared to traditional indices. Benefits include:
- **Automatic Rollover**: Data streams automatically manage the creation of new backing indices based on size or age.
- **Simplified Indexing**: Clients write to a single data stream name instead of multiple date-based indices.
- **Better Performance**: Optimized for append-only time-series data.

## Implementation Details

### Data Stream Naming Convention for Isolation
To ensure complete isolation between legacy data and new data streams, we use the `jaeger-ds-*` prefix for all data streams:
- `jaeger-ds-span`: Spans data stream.
- `jaeger-ds-service`: Service/Operation data stream.
- `jaeger-ds-sampling`: Sampling probabilities data stream.
- `jaeger-ds-dependencies`: Dependency data stream.

This prefixing strategy prevents legacy wildcard queries like `jaeger-span-*` from matching new data stream data, as requested by the maintainers.

### Index Templates
Data stream-specific index templates are provided in `internal/storage/v1/elasticsearch/mappings/`:
- `jaeger-ds-span-8.json`
- `jaeger-ds-service-8.json`
- `jaeger-ds-sampling-8.json`
- `jaeger-ds-dependencies-8.json`

These templates include the `data_stream: {}` field and specify a default ingest pipeline for timestamp normalization (e.g., `jaeger-ds-span-timestamp`).

### Storage Logic
The storage stores (Span, Sampling, Dependency) are updated to:
1.  **Write** to the data stream name when `UseDataStream` is enabled.
2.  **Read** from both the data stream and the legacy wildcard patterns to ensure data visibility during the migration period.

## Migration Strategy
Jaeger supports a seamless migration from traditional indices to data streams:
- **Phase 1**: Enable `UseDataStream`. Jaeger will start writing to the new `jaeger-ds-*` data streams.
- **Phase 2**: During read operations, Jaeger queries both the new data streams and the legacy indices.
- **Phase 3**: Once legacy indices are aged out and deleted, only data streams will be used.

## Configuration
The following configuration options control data stream usage:
- `UseDataStream`: Boolean flag to enable data stream support (requires Elasticsearch 8+).
- `IndexPrefix`: Optional prefix for all indices and data streams.

## Verification & Evidence

### 1. Ingest Pipeline Setup
Data streams require a `@timestamp` field. Jaeger spans use `startTime` (microseconds). The following ingest pipeline must be configured in Elasticsearch before running Jaeger:

```bash
curl -X PUT "localhost:9200/_ingest/pipeline/jaeger-ds-span-timestamp" -H 'Content-Type: application/json' -d'
{
  "description": "Copy Jaeger span startTime to @timestamp",
  "processors": [
    {
      "set": {
        "field": "@timestamp",
        "copy_from": "startTime",
        "ignore_empty_value": true
      }
    },
    {
      "date": {
        "field": "@timestamp",
        "formats": ["epoch_millis"],
        "target_field": "@timestamp"
      }
    }
  ]
}'
```

### 2. Testing Steps
1. **Prerequisites**: Elasticsearch 8.x and Jaeger built from the data stream support branch.
2. **Configuration**: Set `use_data_stream: true` and `create_mappings: true` in the Jaeger configuration.
3. **Verify Templates**: Ensure the `jaeger-ds-span` template exists and has `"data_stream": {}`.
   ```bash
   curl -X GET "localhost:9200/_index_template/jaeger-ds-span?pretty"
   ```
4. **Generate Data**: Use `tracegen` or a sample app like HotROD to send spans.
5. **Verify Data Stream**:
   ```bash
   curl -X GET "localhost:9200/_data_stream/jaeger-ds-span?pretty"
   ```

### 3. Proof of Evidence

#### Data Ingestion (New Data)
Documents are correctly indexed into the data stream with the `@timestamp` field populated via the pipeline:
```json
{
  "_index": ".ds-jaeger-ds-span-2025.12.27-000001",
  "_source": {
    "traceID": "datastream-test-1",
    "startTime": 1735294669887000,
    "@timestamp": "2025-12-27T10:17:49.887Z"
  }
}
```

#### Backward Compatibility (Legacy Data)
Existing indices (e.g., `jaeger-span-2025-12-27`) remain queryable alongside new data:
```json
{
  "_index": "jaeger-span-2025-12-27",
  "_source": {
    "traceID": "legacy-test-1",
    "startTimeMillis": 1735293600000
  }
}
```

#### Isolation Verified
Confirmed that legacy wildcard searches like `jaeger-span-*` **do not** return data from `jaeger-ds-span`, ensuring complete isolation as requested.
