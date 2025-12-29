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
- `jaeger-span-8.json` (merged with standard template)
- `jaeger-service-8.json`
- `jaeger-sampling-8.json`
- `jaeger-dependencies-8.json`

These templates include the `data_stream: {}` field and specify a default ingest pipeline for timestamp normalization (e.g., `jaeger-ds-span-timestamp`).

### Storage Logic
The storage stores (Span, Sampling, Dependency) are updated to:
1.  **Write** to the data stream name when `ES Version >= 8`.
2.  **Read** behavior is controlled by the `jaeger.es.readLegacyWithDataStream` feature gate:
    - When **enabled** (default during migration): Queries both data streams and legacy indices (e.g., `["jaeger-ds-span", "jaeger-span-*"]`)
    - When **disabled**: Queries only data streams (e.g., `["jaeger-ds-span"]`)
    
This feature gate allows operators to safely migrate from legacy indices to data streams and disable dual reads once migration is complete.

## Migration Strategy
Jaeger supports a seamless migration from traditional indices to data streams:
- **Phase 1**: Enable Data Streams (Default for ES 8+). Jaeger will start writing to the new `jaeger-ds-*` data streams.
- **Phase 2**: During read operations, the `jaeger.es.readLegacyWithDataStream` feature gate (enabled by default) ensures Jaeger queries both the new data streams and the legacy indices.
- **Phase 3**: Once legacy indices are aged out and deleted, disable the feature gate to query only data streams:
  ```bash
  --feature-gates=-jaeger.es.readLegacyWithDataStream
  ```

## Configuration
The following configuration options control data stream usage:
- `ES Version`: Data streams are automatically enabled for Elasticsearch 8+.
- `IndexPrefix`: Optional prefix for all indices and data streams.
- `jaeger.es.readLegacyWithDataStream`: Feature gate to control dual reading (enabled by default for migration support).

## ILM Policy Proposal
To manage the lifecycle of data streams effectively, we propose the following Index Lifecycle Management (ILM) policy, named `jaeger-ilm-policy`.

### Policy Definition
The policy defines three phases to optimize storage costs and performance:

1.  **Hot Phase**:
    -   **Rollover**: Occurs when the index reaches **50GB**, **200 million documents**, or **1 day** of age.
    -   **Priority**: Set to **100** (Highest).
2.  **Warm Phase**:
    -   **Transition**: Moves to warm phase **0ms** after rollover (effectively immediately upon leaving hot).
    -   **Forcemerge**: Reduces segment count to 1 for better search performance and reduced overhead.
    -   **Priority**: Set to **50**.
3.  **Delete Phase**:
    -   **Action**: Delete indices **7 days** after rollover.

### JSON Representation
```json
{
  "policy": {
    "phases": {
      "hot": {
        "min_age": "0ms",
        "actions": {
          "rollover": {
            "max_age": "1d",
            "max_primary_shard_size": "50gb",
            "max_docs": 200000000
          },
          "set_priority": {
            "priority": 100
          }
        }
      },
      "warm": {
        "min_age": "0ms",
        "actions": {
          "forcemerge": {
            "max_num_segments": 1
          },
          "set_priority": {
            "priority": 50
          }
        }
      },
      "delete": {
        "min_age": "7d",
        "actions": {
          "delete": {
            "delete_searchable_snapshot": true
          }
        }
      }
    }
  }
}
```

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
2. **Configuration**: Ensure `UseILM=true` (if applicable) or verify default templates.
3. **Verify Templates**: Ensure the `jaeger-span` template exists and has `"data_stream": {}`.
   ```bash
   curl -X GET "localhost:9200/_index_template/jaeger-span?pretty"
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

#### Verification
Confirmed that traces are queryable and data streams are created as expected.
