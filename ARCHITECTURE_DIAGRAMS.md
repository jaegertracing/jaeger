# Architecture Diagrams

## Current Architecture (Before Changes)

```
┌─────────────────────────────────────────────────────────┐
│                     server.go                           │
│                                                         │
│  traceReader (v2) ──┬──> v2 QueryService ──> API v3    │
│                     │                                   │
│                     └──> v1adapter.GetV1Reader()       │
│                           │                             │
│                           v                             │
│                        spanReader (v1 interface)        │
│                           │                             │
│                           v                             │
│                        v1 QueryService ──> API v2       │
│                                                         │
└─────────────────────────────────────────────────────────┘

PROBLEM: Two QueryService instances, duplicate logic
```

## Current Request Flow

```
┌─────────────┐
│ API v2      │
│ gRPC Client │
└──────┬──────┘
       │
       v
┌─────────────────────────┐
│   GRPCHandler           │
│   (api_v2 server)       │
└──────┬──────────────────┘
       │
       v
┌─────────────────────────┐         ┌──────────────────┐
│   v1 QueryService       │────────>│  spanReader      │
│   (querysvc)            │         │  (v1 interface)  │
└─────────────────────────┘         └────────┬─────────┘
                                             │
                                             v
                                    ┌─────────────────┐
                                    │  v1adapter      │
                                    │  SpanReader     │
                                    └────────┬────────┘
                                             │
                                             v
                                    ┌─────────────────┐
                                    │ tracestore      │
                                    │ Reader (v2)     │
                                    └─────────────────┘

vs.

┌─────────────┐
│ API v3      │
│ gRPC Client │
└──────┬──────┘
       │
       v
┌─────────────────────────┐
│   apiv3.Handler         │
│   (api_v3 server)       │
└──────┬──────────────────┘
       │
       v
┌─────────────────────────┐         ┌──────────────────┐
│   v2 QueryService       │────────>│ tracestore       │
│   (v2querysvc)          │         │ Reader (v2)      │
└─────────────────────────┘         └──────────────────┘

INEFFICIENCY: Extra adapter layer and service for API v2
```

## Proposed Architecture (After Changes)

```
┌─────────────────────────────────────────────────────────┐
│                     server.go                           │
│                                                         │
│  v2 QueryService ──┬──> API v3 ──> Client (v3)         │
│         ↑          │                                    │
│         │          └──> API v2 ──> Client (v2)         │
│         │               (with conversion)               │
│  traceReader (v2)                                       │
└─────────────────────────────────────────────────────────┘

BENEFIT: Single QueryService instance, simpler architecture
```

## Proposed Request Flow

```
┌─────────────┐
│ API v2      │
│ gRPC Client │
└──────┬──────┘
       │
       v
┌─────────────────────────┐
│   GRPCHandler           │
│   (api_v2 server)       │
└──────┬──────────────────┘
       │
       v
┌─────────────────────────┐
│   v2 QueryService       │
│   (v2querysvc)          │
└──────┬──────────────────┘
       │
       v
┌─────────────────────────┐         ┌──────────────────┐
│  tracestore.Reader (v2) │◄────────│ Storage Backend  │
└──────┬──────────────────┘         └──────────────────┘
       │
       │ Returns: iter.Seq2[[]ptrace.Traces, error]
       v
┌─────────────────────────┐
│  v2adapter helpers      │
│  (NEW)                  │
│  - IteratorToV1Traces() │
│  - V2TracesToV1Trace()  │
└──────┬──────────────────┘
       │
       │ Returns: []*model.Trace
       v
┌─────────────────────────┐
│  GRPCHandler            │
│  Sends API v2 response  │
└─────────────────────────┘

BENEFIT: Conversion happens at API boundary, not storage boundary
```

## Layer Responsibilities (After)

```
┌──────────────────────────────────────────────────────┐
│                  API Layer                           │
│  - GRPCHandler (API v2)                              │
│  - APIHandler (API v2 HTTP)                          │
│  - apiv3.Handler (API v3)                            │
│  - apiv3.HTTPGateway (API v3 HTTP)                   │
│                                                      │
│  RESPONSIBILITY: API contract, auth, validation,     │
│                  data model conversion               │
└──────────────────┬───────────────────────────────────┘
                   │
                   v
┌──────────────────────────────────────────────────────┐
│             Query Service Layer                      │
│  - v2 QueryService (ONLY ONE!)                       │
│                                                      │
│  RESPONSIBILITY: Business logic, trace aggregation,  │
│                  archive fallback, adjusters         │
└──────────────────┬───────────────────────────────────┘
                   │
                   v
┌──────────────────────────────────────────────────────┐
│              Storage Layer                           │
│  - tracestore.Reader (v2)                            │
│  - depstore.Reader (v2)                              │
│                                                      │
│  RESPONSIBILITY: Data persistence and retrieval      │
└──────────────────────────────────────────────────────┘

CLARITY: Clear separation of concerns
```

## Data Flow Comparison

### Current (Before)
```
Client (v2) → API v2 Handler → v1 QueryService → v1adapter → Storage v2
Client (v3) → API v3 Handler → v2 QueryService ───────────> Storage v2

TWO QueryService instances from same storage!
```

### Proposed (After)
```
Client (v2) → API v2 Handler → v1adapter conversion ┐
                                                     ├─> v2 QueryService → Storage v2
Client (v3) → API v3 Handler ────────────────────────┘

ONE QueryService instance with conversion at API edge!
```

## Conversion Points

### Current Architecture
```
┌─────────────┐     ┌──────────────┐     ┌─────────────┐
│  Storage    │────>│  v1adapter   │────>│ v1 QuerySvc │
│  (v2 data)  │     │  (convert)   │     │ (v1 data)   │
└─────────────┘     └──────────────┘     └─────────────┘

Conversion happens at STORAGE layer (always, even if not needed)
```

### Proposed Architecture
```
┌─────────────┐     ┌──────────────┐     ┌─────────────┐
│  Storage    │────>│ v2 QuerySvc  │────>│ v2adapter   │
│  (v2 data)  │     │ (v2 data)    │     │ (convert)   │
└─────────────┘     └──────────────┘     └─────────────┘
                                                │
                                                v
                                         ┌─────────────┐
                                         │ API v2      │
                                         │ (v1 data)   │
                                         └─────────────┘

Conversion happens at API layer (only when needed for API v2)
API v3 gets native v2 data with no conversion!
```

## Key Advantages

1. **Single QueryService**: One instance handles all APIs
2. **Conversion at Edge**: Data model conversion only when/where needed
3. **Better Performance**: API v3 never pays conversion cost
4. **Clearer Architecture**: Storage → Business Logic → API → Client
5. **Easier Maintenance**: One less service layer to maintain
6. **Future Ready**: Easy to deprecate API v2 in the future

## Migration Safety

The v1 QueryService code remains in the repository (just not instantiated):
```
BEFORE:
v1 QueryService ✓ (code exists, instantiated)
v2 QueryService ✓ (code exists, instantiated)

DURING MIGRATION:
v1 QueryService ✓ (code exists, NOT instantiated)
v2 QueryService ✓ (code exists, instantiated)

AFTER (FUTURE):
v1 QueryService ✗ (can be deleted in major version bump)
v2 QueryService ✓ (code exists, instantiated)
```

This allows easy rollback if issues are discovered.
