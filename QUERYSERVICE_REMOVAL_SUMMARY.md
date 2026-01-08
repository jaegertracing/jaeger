# V1 QueryService Removal - Investigation Summary

## Quick Links

📋 **[Full Investigation Report](INVESTIGATION_REMOVE_V1_QUERYSERVICE.md)** - Detailed analysis and proposal  
🔧 **[Implementation Guide](IMPLEMENTATION_DETAILS.md)** - Step-by-step implementation with code examples

## Problem Statement

The query service in `cmd/jaeger/internal/extension/jaegerquery/server.go` currently instantiates both v1 and v2 QueryService objects:

```go
qs := querysvc.NewQueryService(traceReader, depReader, opts)
v2qs := v2querysvc.NewQueryService(traceReader, depReader, v2opts)
```

This creates redundancy and maintenance burden.

## Key Findings

### Current Architecture
- **V1 QueryService** serves API v2 (legacy Jaeger API) using Jaeger v1 data model
- **V2 QueryService** serves API v3 (modern OTLP-based API) using OpenTelemetry data model
- Both services wrap the same underlying v2 storage layer
- V1 uses an adapter (`v1adapter.GetV1Reader()`) to convert v2 storage to v1 interface

### Usage Breakdown
| Component | Current QueryService | API Version | Status |
|-----------|---------------------|-------------|--------|
| GRPCHandler | v1 QueryService | API v2 (gRPC) | Can migrate |
| APIHandler (HTTP) | v1 QueryService | API v2 (HTTP) | Can migrate |
| apiv3.Handler | v2 QueryService | API v3 (gRPC) | Already using v2 ✓ |
| apiv3.HTTPGateway | v2 QueryService | API v3 (HTTP) | Already using v2 ✓ |

## Recommendation

**Migrate API v2 handlers to use v2 QueryService** with data model conversion at the API boundary.

### Benefits
✅ Single QueryService instantiation  
✅ Reduced code duplication  
✅ Simplified architecture  
✅ Better performance (native v2 storage access)  
✅ Maintains backward compatibility  

### Approach
1. Create conversion utilities to translate v2 QueryService responses (OpenTelemetry format) to API v2 format (Jaeger v1 model)
2. Update GRPCHandler to use v2 QueryService with conversions
3. Update APIHandler to use v2 QueryService with conversions
4. Remove v1 QueryService instantiation from server.go
5. Keep v1 QueryService implementation as deprecated (can be removed in future major version)

## Implementation Effort

**Estimated Time:** 3-5 days for experienced contributor

**Risk Level:** Medium (requires careful testing but clear rollback path)

## Files Requiring Changes

### Core Changes
- `cmd/jaeger/internal/extension/jaegerquery/server.go` - Remove v1 instantiation
- `cmd/jaeger/internal/extension/jaegerquery/internal/server.go` - Update signatures
- `cmd/jaeger/internal/extension/jaegerquery/internal/grpc_handler.go` - Use v2 QueryService
- `cmd/jaeger/internal/extension/jaegerquery/internal/http_handler.go` - Use v2 QueryService

### New Files
- `cmd/jaeger/internal/extension/jaegerquery/internal/querysvc/v2adapter/converter.go` - Conversion utilities
- `cmd/jaeger/internal/extension/jaegerquery/internal/querysvc/v2adapter/converter_test.go` - Tests

### Test Updates
- `*_test.go` files in the same directories - Update to use v2 QueryService

## Success Criteria

- [ ] All API v2 gRPC endpoints work correctly
- [ ] All API v2 HTTP endpoints work correctly
- [ ] All API v3 endpoints continue to work
- [ ] Archive storage functionality preserved
- [ ] No performance regression (< 5% slowdown acceptable)
- [ ] All tests pass
- [ ] Code coverage maintained

## Next Steps

1. **Review & Approve** - Maintainers review the proposal
2. **Implement** - Follow the implementation guide
3. **Test** - Comprehensive testing including E2E
4. **Document** - Update architecture documentation
5. **Deploy** - Merge and monitor

## Questions or Concerns?

See the detailed documents for more information:
- Technical deep-dive: [INVESTIGATION_REMOVE_V1_QUERYSERVICE.md](INVESTIGATION_REMOVE_V1_QUERYSERVICE.md)
- Code-level changes: [IMPLEMENTATION_DETAILS.md](IMPLEMENTATION_DETAILS.md)

---

**Investigation Date:** January 8, 2026  
**Status:** Awaiting review and approval
