# Elasticsearch Data Streams for Span Storage

## Status

Proposed

## Context

Jaeger's Elasticsearch storage backend currently uses time-based indices with manual rollover aliases (`jaeger-span-write`, `jaeger-span-read`) for span storage. While functional, this approach has operational challenges.

### Current Behavior

The existing implementation in [`internal/storage/v2/elasticsearch/`](../../internal/storage/v2/elasticsearch/) manages span indices through:

1. **Manual Rollover Aliases**: Requires explicit alias configuration and rollover triggers
2. **Explicit Index Naming**: Index names follow `jaeger-span-YYYY-MM-DD` pattern
3. **Separate ILM Configuration**: Users must configure Index Lifecycle Management policies independently

### Problems

1. **Operational Overhead**: Managing rollover aliases and ILM policies requires significant operational knowledge.
2. **Configuration Complexity**: Multiple interdependent flags (`UseILM`, `CreateAliases`, `IndexRolloverFrequencySpans`) create potential for misconfiguration.
3. **Modern ES Features Unused**: Elasticsearch 7.9+ and OpenSearch 2.0+ natively support Data Streams, which simplify time-series data management.

### Data Streams Overview

[Elasticsearch Data Streams](https://www.elastic.co/guide/en/elasticsearch/reference/current/data-streams.html) are the native solution for append-only time-series data:

- **Automatic Rollover**: Built-in index lifecycle management
- **Simplified Writes**: Single endpoint for all writes (`POST /<data-stream>/_doc`)
- **Integrated ILM/ISM**: Lifecycle policies referenced directly in index templates

Data Streams only support `create` operations (append-only). Documents cannot be updated or deleted by ID, which makes them ideal for immutable trace data.

## Decision

Implement a **Hybrid Indexing Model** for Jaeger's Elasticsearch storage:

| Data Type | Storage Strategy | Rationale |
|-----------|-----------------|-----------|
| **Spans** | Data Streams (`jaeger-span-ds`) | High-volume, append-only, immutable time-series |
| **Services** | Standard Indices | Requires deduplication and updates |
| **Dependencies** | Standard Indices | Batch-computed, requires updates |

### Why Hybrid?

Spans are ideal for Data Streams because they are append-only and never updated. Services and Dependencies require document updates for deduplication, making them incompatible with Data Streams.

### Configuration

```yaml
es:
  use_data_stream: true  # Default: false, opt-in for initial release
```

**Automatic Backend Detection**: Jaeger automatically detects whether it's connected to Elasticsearch or OpenSearch and applies the appropriate lifecycle policy:
- **Elasticsearch**: Uses ILM via `index.lifecycle.name`
- **OpenSearch**: Uses ISM via `plugins.index_state_management.policy_id`

No separate `use_ism` flag is required.

### Index Template Structure

Data Streams use composable index templates:

```json
{
  "index_patterns": ["jaeger-span-ds"],
  "data_stream": {},
  "composed_of": ["jaeger-span-mappings"],
  "template": {
    "settings": {
      "index.lifecycle.name": "jaeger-ilm-policy"
    }
  }
}
```

### Handling @timestamp

Data Streams require a `@timestamp` field. An ingest pipeline copies `startTime` to `@timestamp`:

```json
{
  "description": "Copy startTime to @timestamp for Data Stream compatibility",
  "processors": [
    { "set": { "field": "@timestamp", "copy_from": "startTime" } }
  ]
}
```

### Backward Compatibility

| User Scenario | Behavior |
|---------------|----------|
| **New Users** | Use Data Streams when enabled |
| **Existing Users** | Dual-lookup reads from both Data Stream and legacy indices |
| **Custom Templates** | Component templates allow customization |

**No re-indexing required**. Legacy indices age out naturally per retention settings while new data flows to Data Streams.

### Minimum Version Requirement

**Elasticsearch 7.9** or **OpenSearch 2.0** is required for Data Stream support.

## Consequences

### Positive

1. **Simplified Operations**: No manual rollover configuration for spans
2. **Automatic ILM/ISM**: Lifecycle policies integrated into template
3. **Modern Architecture**: Aligns with Elasticsearch best practices
4. **Performance**: Native optimizations for append-only workloads

### Negative

1. **Version Requirement**: ES 7.9+/OpenSearch 2.0+ required
2. **Learning Curve**: Users familiar with legacy aliases need to understand new model
3. **Template Migration**: Custom template users need one-time migration

### Neutral

1. **Hybrid Model**: Two storage strategies may initially seem complex but reflects the different nature of spans vs. metadata

## Implementation Phases

### Phase 1: Core Support
- Add `use_data_stream` configuration flag
- Implement `jaeger-span-ds` index template
- Create timestamp ingest pipeline
- Auto-detect ES vs OpenSearch for policy selection

### Phase 2: Migration Support  
- Implement dual-lookup reader for backward compatibility
- Documentation for custom template migration

### Phase 3: Default Transition
- Make Data Streams the default for new installations
- Deprecate `UseILM` flag (subsumed by Data Stream behavior)

## References

- Implementation: PR [#7768](https://github.com/jaegertracing/jaeger/pull/7768)
- Design Document: [DataStream Proposal](https://docs.google.com/document/d/1WDQJmHGjnyck5h1DDvZf4ZJnTaXEF-wUvR6k_lipWJ4)
- [Elasticsearch Data Streams Documentation](https://www.elastic.co/guide/en/elasticsearch/reference/current/data-streams.html)
- [OpenSearch Data Streams Documentation](https://opensearch.org/docs/latest/opensearch/data-streams/)
