# Investigation Report: Removing v1 QueryService

## Executive Summary

This document investigates the dual instantiation of v1 and v2 QueryService objects in `cmd/jaeger/internal/extension/jaegerquery/server.go` and provides a proposal for removing the v1 version.

**Current State:**
```go
qs := querysvc.NewQueryService(traceReader, depReader, opts)
v2qs := v2querysvc.NewQueryService(traceReader, depReader, v2opts)
```

Both services are created and used to serve different API versions, creating redundancy in the codebase.

## Current Architecture

### 1. V1 QueryService (`querysvc.QueryService`)

**Location:** `cmd/jaeger/internal/extension/jaegerquery/internal/querysvc/query_service.go`

**Purpose:** Serves the legacy API v2 (gRPC and HTTP) using the v1 data model.

**Key Characteristics:**
- Uses `spanstore.Reader` interface (v1 storage API)
- Works with `model.Trace` (Jaeger v1 data model)
- Wraps v2 `tracestore.Reader` using `v1adapter.GetV1Reader()`
- Returns `*model.Trace` objects
- Uses synchronous API patterns

**Storage Adapter:**
```go
spanReader := v1adapter.GetV1Reader(traceReader)
```

**Used By:**
1. **GRPCHandler** - Implements `api_v2.QueryServiceServer`
   - Location: `cmd/jaeger/internal/extension/jaegerquery/internal/grpc_handler.go`
   - Methods: `GetTrace()`, `FindTraces()`, `ArchiveTrace()`
   
2. **APIHandler** (HTTP) - Serves API v2 HTTP endpoints
   - Location: `cmd/jaeger/internal/extension/jaegerquery/internal/http_handler.go`
   - Methods: Various HTTP endpoints for traces, services, operations

3. **StaticHandler** - UI capabilities check
   - Uses `querySvc.GetCapabilities()` to determine archive storage availability

### 2. V2 QueryService (`v2querysvc.QueryService`)

**Location:** `cmd/jaeger/internal/extension/jaegerquery/internal/querysvc/v2/querysvc/service.go`

**Purpose:** Serves the modern API v3 (gRPC and HTTP Gateway) using OpenTelemetry data model.

**Key Characteristics:**
- Uses `tracestore.Reader` interface (v2 storage API) directly
- Works with `ptrace.Traces` (OpenTelemetry data model)
- No adapter needed - native v2 implementation
- Returns iterator-based results (`iter.Seq2`)
- Modern, streaming-friendly API design

**Used By:**
1. **apiv3.Handler** - Implements `api_v3.QueryServiceServer`
   - Location: `cmd/jaeger/internal/extension/jaegerquery/internal/apiv3/grpc_handler.go`
   - Methods: `GetTrace()`, `FindTraces()`, `GetServices()`, `GetOperations()`, `GetDependencies()`

2. **apiv3.HTTPGateway** - HTTP endpoints for API v3
   - Location: `cmd/jaeger/internal/extension/jaegerquery/internal/apiv3/http_gateway.go`
   - Routes: `/api/v3/traces`, `/api/v3/services`, `/api/v3/operations`

## Why Two QueryServices Exist

The dual QueryService setup exists because:

1. **API Version Support**: Jaeger needs to support both legacy API v2 and modern API v3 simultaneously
2. **Data Model Differences**: 
   - API v2 uses Jaeger's native v1 data model (`model.Trace`)
   - API v3 uses OpenTelemetry's data model (`ptrace.Traces`)
3. **Migration Path**: Allows gradual transition from v1 to v2 without breaking existing clients
4. **Interface Compatibility**: v1 QueryService implements patterns expected by legacy code

## Storage Layer Architecture

The storage layer uses a v2-first approach with adapters:

```
┌─────────────────────────┐
│   v2 tracestore.Reader  │ (Primary, native implementation)
└───────────┬─────────────┘
            │
            ├──────────────────────────────┐
            │                              │
            v                              v
    ┌───────────────┐            ┌─────────────────┐
    │ v2 QuerySvc   │            │ v1adapter       │
    │ (Direct use)  │            │ SpanReader      │
    └───────┬───────┘            └────────┬────────┘
            │                             │
            │                             v
            │                    ┌─────────────────┐
            │                    │ v1 QuerySvc     │
            │                    │ (Uses adapted)  │
            │                    └────────┬────────┘
            │                             │
            v                             v
    ┌───────────────┐            ┌─────────────────┐
    │   API v3      │            │    API v2       │
    │  (Modern)     │            │   (Legacy)      │
    └───────────────┘            └─────────────────┘
```

## Proposal: Remove v1 QueryService

### Approach: Migrate API v2 Handlers to Use v2 QueryService

Instead of removing v1 QueryService entirely, we should **migrate the API v2 handlers** to use v2 QueryService with appropriate data model conversions.

### Strategy

1. **Create API v2 Adapters for v2 QueryService**
   - Build conversion layer between v2 QueryService responses and API v2 requirements
   - Convert `ptrace.Traces` → `model.Trace` where needed
   - Handle iterator-based responses from v2 QueryService

2. **Update GRPCHandler to Use v2 QueryService**
   - Modify `GRPCHandler` to accept `v2querysvc.QueryService`
   - Convert iterator results to v1 model format
   - Maintain exact same API v2 contract

3. **Update APIHandler to Use v2 QueryService**
   - Modify `APIHandler` (HTTP) to accept `v2querysvc.QueryService`
   - Add conversion helpers for trace responses
   - Preserve existing API v2 HTTP behavior

4. **Remove v1 QueryService Instantiation**
   - Remove `querysvc.NewQueryService()` call from `server.go`
   - Pass only `v2qs` to handlers

5. **Maintain v1 QueryService Code (Temporarily)**
   - Keep the v1 QueryService implementation for reference
   - Mark as deprecated
   - Can be removed in future major version

### Benefits

✅ **Single Source of Truth**: Only one QueryService implementation to maintain
✅ **Reduced Code Duplication**: One less service layer to maintain
✅ **Simplified Testing**: Fewer test fixtures and mocks needed
✅ **Better Performance**: Native v2 storage access for all APIs
✅ **Clearer Architecture**: Data model conversion at API boundary, not storage boundary
✅ **Future-proof**: All code paths use modern v2 storage interfaces

### Risks & Mitigation

#### Risk 1: Breaking API v2 Behavior
- **Mitigation**: Comprehensive integration tests covering all API v2 endpoints
- **Mitigation**: Careful conversion logic with fallback behavior matching v1

#### Risk 2: Performance Changes
- **Mitigation**: Benchmark critical paths before/after
- **Mitigation**: Leverage v2's iterator-based approach for better streaming

#### Risk 3: Iterator Handling Complexity
- **Mitigation**: Create clear helper functions for iterator → slice conversions
- **Mitigation**: Reuse existing patterns from apiv3 handlers

#### Risk 4: Archive Storage Edge Cases
- **Mitigation**: Preserve exact fallback logic from v1 implementation
- **Mitigation**: Test archive storage scenarios explicitly

### Implementation Plan

#### Phase 1: Preparation (Low Risk)
1. Add conversion utilities:
   - `v2TracesToV1Trace([]ptrace.Traces) *model.Trace`
   - `v2IteratorToV1Traces(iter.Seq2) ([]*model.Trace, error)`
2. Create comprehensive tests for conversions
3. Ensure all existing tests pass

#### Phase 2: Migrate GRPCHandler (Medium Risk)
1. Update `GRPCHandler` constructor to accept `*v2querysvc.QueryService`
2. Modify `GetTrace()` to use v2 QueryService + conversion
3. Modify `FindTraces()` to use v2 QueryService + conversion
4. Modify `ArchiveTrace()` to use v2 QueryService
5. Run integration tests for API v2 gRPC

#### Phase 3: Migrate HTTP Handler (Medium Risk)
1. Update `APIHandler` constructor to accept `*v2querysvc.QueryService`
2. Update trace query methods with conversions
3. Update service/operation query methods (minimal changes needed)
4. Run integration tests for API v2 HTTP

#### Phase 4: Update Server (Low Risk)
1. Remove v1 QueryService instantiation from `server.go`
2. Remove v1 `QueryServiceOptions` and related archive setup
3. Update `NewServer()` signature to only take v2 QueryService
4. Update `registerGRPCHandlers()` and `initRouter()` calls
5. Run full integration test suite

#### Phase 5: Cleanup (Low Risk)
1. Mark v1 QueryService as deprecated
2. Update documentation
3. Remove v1adapter usage in query path (if no longer needed elsewhere)

### Alternative Approaches Considered

#### Alternative 1: Keep Both (Current State)
- ❌ Maintains technical debt
- ❌ Duplicate maintenance burden
- ❌ Confusing for new contributors

#### Alternative 2: Remove v1 QueryService Completely
- ❌ Too aggressive, removes useful reference implementation
- ❌ Makes rollback difficult if issues found

#### Alternative 3: Port v2 QueryService to v1 Interfaces
- ❌ Goes backward in architecture evolution
- ❌ Perpetuates v1 patterns

## Estimated Effort

- **Small Implementation**: 3-5 days for experienced contributor
  - Day 1: Conversion utilities and tests
  - Day 2: GRPCHandler migration
  - Day 3: APIHandler migration
  - Day 4: Integration and testing
  - Day 5: Documentation and cleanup

## Success Criteria

1. ✅ All API v2 tests pass without modification
2. ✅ All API v3 tests continue to pass
3. ✅ No performance regression in query operations
4. ✅ Archive storage functionality preserved
5. ✅ Only one QueryService instantiated in server.go
6. ✅ Code coverage maintained or improved

## Conclusion

The v1 QueryService can be safely removed from instantiation by migrating API v2 handlers to use the v2 QueryService with appropriate data model conversions. This approach:

- Reduces code duplication and maintenance burden
- Maintains backward compatibility for API v2 clients
- Improves architecture by consolidating on v2 storage interfaces
- Provides a clear migration path with manageable risk

**Recommendation**: Proceed with the proposed implementation plan.
