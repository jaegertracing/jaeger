# Investigation: Path to Replace olivere/elastic Driver

**Issue:** [#7612](https://github.com/jaegertracing/jaeger/issues/7612)

**Date:** 2025-12-30

**Status:** Research Complete

## Executive Summary

This document investigates the path to replace the deprecated `olivere/elastic` driver with official Elasticsearch/OpenSearch clients. The olivere/elastic library is officially deprecated, and users are advised to migrate to `github.com/elastic/go-elasticsearch`.

## Current State Analysis

### Dependencies in go.mod

The Jaeger codebase currently uses **both** drivers:

1. **`github.com/olivere/elastic/v7`** (v7.0.32) - Primary driver for ES operations
2. **`github.com/elastic/go-elasticsearch/v9`** (v9.2.1) - Used for ES v8+ template creation

### Code Distribution

Files using `olivere/elastic/v7`: **30 files**

| Component | Files Affected |
|-----------|---------------|
| Span Store (v1) | 4 files (reader.go, writer.go, service_operation.go, reader_test.go) |
| Span Store (v2) | 2 files (depstore/storage.go, storage_test.go) |
| Sampling Store | 2 files (storage.go, storage_test.go) |
| Metric Store | 7 files (reader.go, query_builder.go, to_domain.go, etc.) |
| Core ES Client | 5 files (client.go, config.go, wrapper.go, errors.go, mocks.go) |
| Integration Tests | 3 files |
| Query Package | 1 file (range_query.go - already abstracted) |

### Current Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                         Application Layer                           │
│  (spanstore, depstore, samplingstore, metricstore)                 │
├─────────────────────────────────────────────────────────────────────┤
│                     ES Abstraction Layer                            │
│  internal/storage/elasticsearch/client.go (interfaces)             │
├─────────────────────────────────────────────────────────────────────┤
│                       Wrapper Layer                                 │
│  internal/storage/elasticsearch/wrapper/wrapper.go                 │
├──────────────────────────────┬──────────────────────────────────────┤
│   olivere/elastic/v7         │   go-elasticsearch/v9 (ES v8+)      │
│   (ES v6, v7, OpenSearch)    │   (Template creation only)          │
└──────────────────────────────┴──────────────────────────────────────┘
```

## Migration Options

### Option 1: Migrate to `elastic/go-elasticsearch` (Recommended)

**Official Elasticsearch Go Client**
- Repository: https://github.com/elastic/go-elasticsearch
- Current Version: v9.x (supports ES 8.x and 9.x)
- Maintained by: Elastic (official)

**Pros:**
- Official client, actively maintained by Elastic
- Long-term support guaranteed
- Better ES 8+ and ES 9+ compatibility
- OpenSearch compatibility via shared API

**Cons:**
- Lower-level API (no Query DSL builder)
- Requires building queries as JSON/maps
- Significant refactoring needed
- Loss of fluent API convenience

### Option 2: Add `opensearch-project/opensearch-go` for OpenSearch

**Official OpenSearch Go Client**
- Repository: https://github.com/opensearch-project/opensearch-go
- Current Version: v4.4.0
- Based on: Fork of go-elasticsearch

**Pros:**
- Official OpenSearch support
- Better OpenSearch-specific features
- Active development and releases

**Cons:**
- Would require maintaining two drivers
- More complex codebase
- API differences from go-elasticsearch

### Option 3: Hybrid Approach (Currently in Use)

Continue using both drivers with abstraction layer:
- `olivere/elastic/v7` for general operations (deprecated)
- `go-elasticsearch/v9` for ES v8+ specific operations

**Note:** This is a temporary solution; `olivere/elastic` is deprecated.

## Detailed Impact Analysis

### High-Impact Components

#### 1. Query Building (`elastic.Query` interface)
Files using `elastic.Query`:
- `internal/storage/v1/elasticsearch/spanstore/reader.go`
- `internal/storage/metricstore/elasticsearch/query_builder.go`

**Migration Effort:** HIGH - All query builders use olivere's fluent DSL:
```go
// Current olivere/elastic approach
query := elastic.NewBoolQuery().
    Must(elastic.NewTermQuery("field", value)).
    Filter(elastic.NewRangeQuery("timestamp").Gte(start))

// go-elasticsearch approach (raw JSON)
query := map[string]any{
    "bool": map[string]any{
        "must": []map[string]any{
            {"term": map[string]any{"field": value}},
        },
        "filter": []map[string]any{
            {"range": map[string]any{"timestamp": map[string]any{"gte": start}}},
        },
    },
}
```

#### 2. Aggregations (`elastic.Aggregation` interface)
Files using aggregations:
- `internal/storage/v1/elasticsearch/spanstore/reader.go` (trace ID aggregations)
- `internal/storage/metricstore/elasticsearch/query_builder.go`

**Migration Effort:** HIGH - Complex aggregation pipelines

#### 3. Bulk Processing (`elastic.BulkProcessor`)
Files using bulk processing:
- `internal/storage/elasticsearch/config/config.go`
- `internal/storage/elasticsearch/wrapper/wrapper.go`

**Migration Effort:** MEDIUM - Both drivers support bulk operations

#### 4. Search Results (`elastic.SearchResult`)
Files parsing search results:
- All reader implementations
- Multiple test files

**Migration Effort:** MEDIUM - Response structures differ

### Already Abstracted Components

**Good News:** The codebase already has abstractions in place:

1. **Client Interface** (`internal/storage/elasticsearch/client.go`):
   - Defines `Client`, `SearchService`, `IndexService`, etc.
   - Migration only needs to update the wrapper implementation

2. **Range Query** (`internal/storage/elasticsearch/query/range_query.go`):
   - Already rewritten to be driver-agnostic
   - Can be used as a template for other queries

## Recommended Migration Path

### Phase 1: Expand Abstraction Layer (Low Risk)
1. Create driver-agnostic query builders similar to `RangeQuery`
2. Add aggregation abstractions
3. Abstract search result parsing

**Files to Create/Modify:**
- `internal/storage/elasticsearch/query/bool_query.go`
- `internal/storage/elasticsearch/query/term_query.go`
- `internal/storage/elasticsearch/query/aggregations.go`
- `internal/storage/elasticsearch/result/search_result.go`

### Phase 2: Update Wrapper Layer (Medium Risk)
1. Implement abstractions using `go-elasticsearch/v9`
2. Maintain backward compatibility via feature flags
3. Add comprehensive tests

**Files to Modify:**
- `internal/storage/elasticsearch/wrapper/wrapper.go`
- `internal/storage/elasticsearch/config/config.go`

### Phase 3: Update Storage Implementations (Higher Risk)
1. Migrate span store readers/writers
2. Migrate dependency store
3. Migrate sampling store
4. Migrate metric store

### Phase 4: Cleanup
1. Remove `olivere/elastic` dependency
2. Update documentation
3. Announce breaking changes in CHANGELOG

## Effort Estimation

| Phase | Components | Complexity | Files |
|-------|------------|------------|-------|
| Phase 1 | Query abstractions | Medium | ~8 new files |
| Phase 2 | Wrapper implementation | High | ~5 files |
| Phase 3 | Storage implementations | High | ~15 files |
| Phase 4 | Cleanup & documentation | Low | ~5 files |

**Total Estimated Files:** ~33 files modified/created

## Testing Strategy

1. **Unit Tests:** Update all existing tests with new abstractions
2. **Integration Tests:** Run against ES v7, v8, v9 and OpenSearch v1, v2, v3
3. **Feature Gates:** Use OTel Collector's feature gate pattern for gradual rollout
4. **Backward Compatibility:** Ensure existing deployments continue working

## Risks and Mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Breaking changes in API | High | High | Use feature gates, deprecation warnings |
| Performance regression | Medium | Medium | Benchmark before/after |
| OpenSearch incompatibility | Low | High | Dedicated testing matrix |
| Test coverage gaps | Medium | Medium | Integration test suite |

## Conclusion

The migration from `olivere/elastic` to `go-elasticsearch` is necessary due to deprecation but requires significant effort. The existing abstraction layer (`client.go`, `wrapper.go`) provides a good foundation.

**Recommended Approach:**
1. Incrementally build driver-agnostic abstractions
2. Use feature gates for gradual migration
3. Maintain comprehensive integration tests
4. Target completion across multiple releases

## References

- [olivere/elastic (Deprecated)](https://github.com/olivere/elastic)
- [elastic/go-elasticsearch (Official)](https://github.com/elastic/go-elasticsearch)
- [opensearch-project/opensearch-go](https://github.com/opensearch-project/opensearch-go)
- [Elastic Discuss: go-elasticsearch vs olivere](https://discuss.elastic.co/t/go-elasticsearch-versus-olivere-golang-client/252248)
- [OpenSearch Forum: Golang Client Libraries](https://forum.opensearch.org/t/golang-client-libraries-olivere-elastic-elastic-go-elasticsearch/5174)
- [Jaeger CHANGELOG - PR #7244](https://github.com/jaegertracing/jaeger/pull/7244)
- [Jaeger CHANGELOG - PR #2448](https://github.com/jaegertracing/jaeger/pull/2448)
