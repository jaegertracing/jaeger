# Migration Plan: cmd/query/app → cmd/jaeger/internal/extension/jaegerquery

## Overview
This document outlines an incremental migration strategy to move all code from `cmd/query/app` into `cmd/jaeger/internal/extension/jaegerquery`, making jaegerquery the primary home for query service functionality in Jaeger v2.

## Current State

### cmd/query/app Structure (86 Go files)
```
cmd/query/app/
├── *.go (root level handlers, server, flags)
├── apiv3/          # API v3 gRPC + HTTP gateway
├── ddg/            # Deep dependency graph
├── fixture/        # Test fixtures
├── qualitymetrics/ # Quality metrics data structures  
├── querysvc/       # Core query service logic
│   ├── internal/   # v1 adjusters
│   └── v2/         # v2 query service + adjusters
└── ui/             # Static UI assets handling
```

### cmd/jaeger/internal/extension/jaegerquery Structure (8 Go files)
```
cmd/jaeger/internal/extension/jaegerquery/
├── config.go       # Extension configuration
├── config_test.go
├── factory.go      # Extension factory
├── factory_test.go
├── server.go       # Server wrapper
├── server_test.go
└── README.md
```

### External Dependencies
- **Primary consumer**: `cmd/jaeger/internal/extension/jaegerquery` (imports most of cmd/query/app)
- **Test dependency**: `cmd/anonymizer/app/query/query_test.go` (legacy, minimal impact)

## Proposed Directory Structure (Target)

```
cmd/jaeger/internal/extension/jaegerquery/
├── config.go               # Configuration (QueryOptions, UIConfig, Storage)
├── config_test.go
├── factory.go              # Extension factory
├── factory_test.go
├── server.go               # Main server orchestration
├── server_test.go
├── README.md
│
├── handlers/               # HTTP/gRPC request handlers (NEW)
│   ├── grpc_handler.go          # From cmd/query/app/grpc_handler.go
│   ├── grpc_handler_test.go
│   ├── http_handler.go          # From cmd/query/app/http_handler.go
│   ├── http_handler_test.go
│   ├── handler_options.go       # From cmd/query/app/handler_options.go
│   ├── query_parser.go          # From cmd/query/app/query_parser.go
│   ├── query_parser_test.go
│   └── trace_response_handler.go
│
├── apiv3/                  # API v3 implementation (MOVED)
│   ├── grpc_handler.go          # From cmd/query/app/apiv3/
│   ├── http_gateway.go
│   └── *_test.go
│
├── querysvc/               # Core query service logic (MOVED)
│   ├── query_service.go         # From cmd/query/app/querysvc/
│   ├── adjusters.go
│   ├── adjuster/                # From internal/adjuster
│   └── v2/                      # v2 query service
│       ├── querysvc/
│       └── adjuster/
│
├── ui/                     # UI asset handling (MOVED)
│   ├── static_handler.go        # From cmd/query/app/static_handler.go
│   ├── static_handler_test.go
│   ├── assets.go                # From cmd/query/app/ui/
│   └── placeholder/
│
├── utils/                  # Shared utilities (NEW)
│   ├── default_params.go        # From cmd/query/app/default_params.go
│   ├── util.go                  # From cmd/query/app/util.go
│   ├── util_test.go
│   ├── json_marshaler.go
│   ├── otlp_translator.go
│   └── otlp_translator_test.go
│
└── qualitymetrics/         # Quality metrics (MOVED)
    ├── data.go                  # From cmd/query/app/qualitymetrics/
    ├── data_test.go
    └── data_types.go
```

## Migration Phases

### Phase 1: Configuration Consolidation
**Goal**: Move configuration types to jaegerquery as the single source of truth

**Steps**:
1. Move `QueryOptions` and `UIConfig` types to `cmd/jaeger/internal/extension/jaegerquery/config.go`
2. Update `cmd/query/app/flags.go` to become a thin re-export layer using type aliases
3. Move `DefaultQueryOptions()` function
4. Move and adapt `flags_test.go`

**Benefits**:
- Establishes jaegerquery as the config owner
- Maintains backward compatibility via type aliases
- ~100 lines migrated

**Risks**: Low - only config types, no behavioral logic

---

### Phase 2: Querysvc Core Logic
**Goal**: Move the core query service implementation

**Steps**:
1. Create `cmd/jaeger/internal/extension/jaegerquery/querysvc/` directory
2. Move `cmd/query/app/querysvc/query_service.go` → `querysvc/query_service.go`
3. Move `cmd/query/app/querysvc/adjusters.go` → `querysvc/adjusters.go`
4. Move adjuster implementations:
   - `cmd/query/app/querysvc/internal/adjuster/` → `querysvc/adjuster/`
5. Move v2 query service:
   - `cmd/query/app/querysvc/v2/` → `querysvc/v2/`
6. Update import paths in moved files
7. Add re-exports in `cmd/query/app/querysvc/` for backward compatibility

**Benefits**:
- Centralizes query business logic
- ~2000 lines migrated
- Enables independent evolution of query service

**Risks**: Medium - core functionality, needs careful testing

**Testing Strategy**:
- Run all querysvc tests after migration
- Verify no behavioral changes via integration tests

---

### Phase 3: API v3 Implementation
**Goal**: Move API v3 gRPC and HTTP gateway

**Steps**:
1. Create `cmd/jaeger/internal/extension/jaegerquery/apiv3/` directory
2. Move all files from `cmd/query/app/apiv3/`:
   - `grpc_handler.go` → `apiv3/grpc_handler.go`
   - `http_gateway.go` → `apiv3/http_gateway.go`
   - All test files and snapshots
3. Update import paths
4. Add re-exports in `cmd/query/app/apiv3/` for backward compatibility

**Benefits**:
- ~500 lines migrated
- API v3 becomes part of jaegerquery extension

**Risks**: Low-Medium - well-isolated API layer

---

### Phase 4: UI Assets Handling
**Goal**: Move static UI serving logic

**Steps**:
1. Create `cmd/jaeger/internal/extension/jaegerquery/ui/` directory
2. Move `cmd/query/app/static_handler.go` → `ui/static_handler.go`
3. Move `cmd/query/app/ui/` contents → `ui/`
4. Move UI fixtures
5. Update import paths
6. Add re-exports in `cmd/query/app/` for backward compatibility

**Benefits**:
- ~800 lines migrated
- UI handling colocated with query extension

**Risks**: Low - mostly file serving logic

---

### Phase 5: HTTP/gRPC Handlers
**Goal**: Move request handlers

**Steps**:
1. Create `cmd/jaeger/internal/extension/jaegerquery/handlers/` directory
2. Move handler files:
   - `grpc_handler.go` → `handlers/grpc_handler.go`
   - `http_handler.go` → `handlers/http_handler.go`
   - `handler_options.go` → `handlers/handler_options.go`
   - `query_parser.go` → `handlers/query_parser.go`
   - `trace_response_handler.go` → `handlers/trace_response_handler.go`
3. Move all associated test files
4. Update import paths
5. Add re-exports in `cmd/query/app/` for backward compatibility

**Benefits**:
- ~1500 lines migrated
- Request handling logic centralized

**Risks**: Medium - HTTP/gRPC layer, needs API compatibility testing

---

### Phase 6: Utilities and Support Code
**Goal**: Move utility functions and helpers

**Steps**:
1. Create `cmd/jaeger/internal/extension/jaegerquery/utils/` directory
2. Move utility files:
   - `default_params.go` → `utils/default_params.go`
   - `util.go` → `utils/util.go`
   - `json_marshaler.go` → `utils/json_marshaler.go`
   - `otlp_translator.go` → `utils/otlp_translator.go`
3. Move `qualitymetrics/` → `qualitymetrics/`
4. Move `ddg/` → `ddg/` (deep dependency graph)
5. Update import paths
6. Add re-exports in `cmd/query/app/` for backward compatibility

**Benefits**:
- ~600 lines migrated
- Support utilities colocated

**Risks**: Low - utility functions

---

### Phase 7: Server Integration
**Goal**: Consolidate server initialization

**Steps**:
1. Review `cmd/query/app/server.go` logic
2. Integrate relevant parts into `cmd/jaeger/internal/extension/jaegerquery/server.go`
3. Remove dependencies on `cmd/query/app` package from jaegerquery
4. Update tests

**Benefits**:
- ~800 lines migrated
- jaegerquery becomes fully self-contained

**Risks**: High - server orchestration, needs extensive testing

**Testing Strategy**:
- Full integration tests
- Manual testing of query API
- Performance testing

---

### Phase 8: Deprecation and Cleanup
**Goal**: Mark cmd/query/app as deprecated

**Steps**:
1. Add deprecation notices to all files in `cmd/query/app/`
2. Update documentation to point to jaegerquery
3. Keep `cmd/query/app/` as re-export layer for one release cycle
4. In subsequent release, consider removing or archiving

**Benefits**:
- Clear migration path for external users
- Maintains backward compatibility temporarily

**Risks**: Low - just documentation

---

## Testing Strategy

### Per-Phase Testing
- **Unit tests**: Run all tests in migrated package after each phase
- **Integration tests**: Verify query service functionality end-to-end
- **API compatibility**: Ensure no breaking changes to public APIs

### Final Validation
- Run full test suite: `make test`
- Manual testing of query API (HTTP and gRPC)
- UI testing with real Jaeger deployment
- Performance comparison: before vs after migration

## Backward Compatibility Strategy

For each migrated component:
1. Keep original file in `cmd/query/app/` as thin wrapper
2. Use type aliases and re-exports to maintain API compatibility
3. Add deprecation comments pointing to new location
4. Maintain for at least one minor release before removal

Example re-export pattern:
```go
// cmd/query/app/querysvc/query_service.go
package querysvc

import jaegerquery "github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"

// Deprecated: Use github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc instead
type QueryService = jaegerquery.QueryService
```

## Rollback Strategy

Each phase is independent and can be rolled back:
1. Revert the specific commit for that phase
2. Re-run tests to verify system stability
3. No cascading failures due to incremental approach

## Timeline Estimate

- Phase 1 (Config): 1-2 days
- Phase 2 (Querysvc): 3-4 days  
- Phase 3 (API v3): 2 days
- Phase 4 (UI): 2 days
- Phase 5 (Handlers): 3-4 days
- Phase 6 (Utils): 2 days
- Phase 7 (Server): 3-4 days
- Phase 8 (Deprecation): 1 day

**Total**: ~17-23 days of development + testing

## Success Criteria

- [ ] All code from `cmd/query/app/` successfully migrated
- [ ] Zero test failures after migration
- [ ] No performance regression
- [ ] API compatibility maintained
- [ ] Documentation updated
- [ ] Backward compatibility layer in place
- [ ] `cmd/jaeger/internal/extension/jaegerquery` is fully self-contained

## Open Questions

1. Should `cmd/anonymizer/app/query/` be migrated or kept as-is?
   - **Recommendation**: Keep as-is initially, it's legacy tooling
   
2. Should we maintain `cmd/query/app/` indefinitely or sunset it?
   - **Recommendation**: Deprecate after one release cycle, remove after two

3. How do we handle external projects that import `cmd/query/app/`?
   - **Recommendation**: Document migration in release notes, provide re-export layer

4. Should we rename packages during migration (e.g., querysvc → service)?
   - **Recommendation**: Keep names consistent initially for easier review
