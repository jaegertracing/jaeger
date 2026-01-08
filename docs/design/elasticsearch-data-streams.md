# Elasticsearch/OpenSearch Data Streams in Jaeger

This document describes the implementation of Elasticsearch and OpenSearch Data Streams in Jaeger, focusing on the motivation, design decisions, and migration strategy.

## Motivation
Elasticsearch/OpenSearch Data Streams provide a more efficient and manageable way to handle time-series data compared to traditional indices. Benefits include:
- **Automatic Rollover**: Data streams automatically manage the creation of new backing indices based on size or age.
- **Simplified Indexing**: Clients write to a single data stream name instead of multiple date-based indices.
- **Better Performance**: Optimized for append-only time-series data.

## Implementation Details

### Data Stream Naming Convention
To ensure consistency and isolation, we use dot-notation for data streams, which is a common convention in the Elastic ecosystem.
The default names are:
- `jaeger.span`: Spans data stream.
- `jaeger.service`: Service/Operation data stream.
- `jaeger.sampling`: Sampling probabilities data stream.
- `jaeger.dependencies`: Dependency data stream.

This naming strategy avoids conflicts with legacy indices (which typically use `jaeger-span-*` patterns) and adheres to modern practices.

### Index Templates
Data stream-specific index templates are provided in `internal/storage/v1/elasticsearch/mappings/`:
- `jaeger.span-8.json` (merged with standard template)
- `jaeger.service-8.json`
- `jaeger.sampling-8.json`
- `jaeger.dependencies-8.json`

These templates include the `data_stream: {}` field and specify a default ingest pipeline for timestamp normalization (e.g., `jaeger-ds-span-timestamp`).

### Storage Logic
The storage stores (Span, Sampling, Dependency) are updated to support a new configuration flag:
- `UseDataStream`: When set to `true`, Jaeger writes to the data stream alias (e.g., `jaeger.span`) instead of date-based indices.

## Lifecycle Management (ILM vs. ISM)
To manage the lifecycle of data streams effectively, we support both Elasticsearch ILM and OpenSearch ISM.

### Elasticsearch: ILM (Index Lifecycle Management)
When `UseILM=true` (and `UseDataStream=true`), Jaeger applies an ILM policy (default: `jaeger-ilm-policy`) that manages rollover and retention.

### OpenSearch: ISM (Index State Management)
When `UseISM=true` (and `UseDataStream=true`), Jaeger applies an ISM policy (default: `jaeger-ism-policy`) compatible with OpenSearch.

**Note**: `UseILM` and `UseISM` are mutually exclusive.

### Default Policy Definition
The default policy (for both ILM and ISM) targets optimized storage costs:
1.  **Hot Phase**: Rollover at 50GB or 200M docs.
2.  **Delete Phase**: Delete indices 7 days after rollover.

## Configuration
The following configuration options control data stream usage:
- `UseDataStream`: Explicit toggle to enable data stream support (default: `false`).
- `UseILM`: Enable Elasticsearch Index Lifecycle Management (default: `false`).
- `UseISM`: Enable OpenSearch Index State Management (default: `false`).
- `IndexPrefix`: Optional prefix for all indices and data streams.
- `jaeger.es.readLegacyWithDataStream`: Feature gate to control dual reading (enabled by default for migration support).

## Migration Strategy
Jaeger supports a seamless migration from traditional indices to data streams:
- **Phase 1**: Enable Data Streams (`UseDataStream=true`). Jaeger will start writing to the new `jaeger.*` data streams.
- **Phase 2**: During read operations, the `jaeger.es.readLegacyWithDataStream` feature gate (enabled by default) ensures Jaeger queries both the new data streams and the legacy indices.
- **Phase 3**: Once legacy indices are aged out and deleted, disable the feature gate to query only data streams.

## Verification & Evidence

### 1. Ingest Pipeline Setup
Data streams require a `@timestamp` field. Jaeger spans use `startTime` (microseconds). An ingest pipeline `jaeger-ds-span-timestamp` is used to copy `startTime` to `@timestamp`.

### 2. Testing Steps
1. **Prerequisites**: Elasticsearch 8.x or OpenSearch 2.x+, and Jaeger built from the data stream support branch.
2. **Configuration**: Set `UseDataStream=true`.
3. **Verify Templates**: Ensure the `jaeger.span` template exists and has `"data_stream": {}`.
   ```bash
   curl -X GET "localhost:9200/_index_template/jaeger.span?pretty"
   ```
4. **Generate Data**: Use `tracegen` or a sample app like HotROD to send spans.
5. **Verify Data Stream**:
   ```bash
   curl -X GET "localhost:9200/_data_stream/jaeger.span?pretty"
   ```
