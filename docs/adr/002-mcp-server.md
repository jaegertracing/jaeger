# MCP Server Extension for Jaeger

## Status

Proposed

## Context

Large Language Models (LLMs) are increasingly being used as assistants for debugging and analyzing distributed systems. Jaeger, as a distributed tracing platform, contains rich observability data that could help LLMs diagnose issues in microservice architectures. However, distributed traces can be massive—a single trace might contain hundreds or thousands of spans—and loading full trace data directly into an LLM's context window is impractical and often counterproductive.

The [Model Context Protocol (MCP)](https://modelcontextprotocol.io/) is an open standard that facilitates integration between LLM applications and external data sources. MCP defines a structured way for AI agents to discover and invoke tools, access resources, and receive responses in a format optimized for LLM consumption.

### Progressive Disclosure Architecture

The key insight driving this design is **progressive disclosure**: rather than dumping entire traces into an LLM context, we provide tools that allow the LLM to follow a guided "drill-down" workflow:

1. **Search** → Find candidate traces matching specific criteria (service, time range, attributes, duration)
2. **Map** → Visualize trace structure (topology) without loading attribute data
3. **Diagnose** → Identify the critical execution path that contributed to latency
4. **Inspect** → Load full details only for specific, suspicious spans

This approach prevents context-window exhaustion and forces structured reasoning.

### Dependencies

The official MCP Go SDK is available at [`github.com/modelcontextprotocol/go-sdk`](https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk/mcp), maintained in collaboration with Google. This SDK supports:
- Tool registration with JSON schema validation
- StdIO and HTTP transports (including Streamable HTTP)
- Server-Sent Events (SSE) for real-time updates
- Concurrent session management

### Existing Infrastructure

Jaeger already provides most of the backend functionality needed:

| MCP Tool | Existing Jaeger Component | Notes |
|----------|---------------------------|-------|
| `get_services` | `QueryService.GetServices()` | Direct mapping |
| `search_traces` | `QueryService.FindTraces()` | Returns metadata; needs filtering |
| `get_trace_topology` | `QueryService.GetTrace()` | Needs post-processing to strip attributes |
| `get_span_details` | `QueryService.GetTrace()` | Needs span-level filtering |
| `get_trace_errors` | `QueryService.GetTrace()` | Needs error status filtering |
| `get_critical_path` | **Not available in backend** | Only exists in UI (TypeScript) |

> [!IMPORTANT]
> The critical path algorithm currently exists only in the Jaeger UI codebase ([`jaeger-ui/packages/jaeger-ui/src/components/TracePage/CriticalPath/index.tsx`](../../jaeger-ui/packages/jaeger-ui/src/components/TracePage/CriticalPath/index.tsx)). This algorithm must be re-implemented in Go for the MCP server.

### Extension Architecture

Following the pattern established by `jaegerquery`, the MCP server will be implemented as an OpenTelemetry Collector extension. This provides:
- Lifecycle management (Start/Shutdown)
- Configuration validation
- Dependency injection via `jaegerquery` extension
- Separate HTTP/SSE endpoint for MCP protocol

> [!NOTE]
> **Phase 2 Requirement**: The MCP extension will need to retrieve the `QueryService` instance from the `jaegerquery` extension. This will require `jaegerquery` to expose `QueryService` through an Extension interface, similar to how `jaegerstorage` exposes storage factories via the `jaegerstorage.Extension` interface and `GetTraceStoreFactory()` helper function. See `cmd/jaeger/internal/exporters/storageexporter/exporter.go:35` for reference implementation pattern.

## Decision

Implement an MCP server as a new extension under `cmd/jaeger/internal/extension/mcpserver/` that:

1. **Exposes MCP tools** for trace search, topology viewing, critical path analysis, and span inspection
2. **Runs on a separate HTTP port** (default: 4320) with Streamable HTTP transport
3. **Depends on `jaegerstorage`** for trace data access, similar to `jaegerquery`
4. **Implements critical path algorithm** in Go, ported from the UI's TypeScript implementation
5. **Uses progressive disclosure** to minimize token consumption in LLM contexts

### MCP Tools Specification

```yaml
tools:
  - name: get_services
    description: List available service names. Use this first to discover valid service names for search_traces.
    input_schema:
      pattern: string (optional) - Filter services by pattern (substring match). Future: may support regex or semantic search.
      limit: integer (optional, default: 100) - Maximum number of services to return
    output: List of service names (strings)

  - name: get_span_names
    description: List available span names for a service. Useful for discovering valid span names before using search_traces.
    input_schema:
      service_name: string (required) - Filter by service name. Use get_services to discover valid names.
      pattern: string (optional) - Optional regex pattern to filter span names
      span_kind: string (optional) - Optional span kind filter (e.g., SERVER, CLIENT, PRODUCER, CONSUMER, INTERNAL)
      limit: integer (optional, default: 100) - Maximum number of span names to return
    output: List of span names with span kind information

  - name: search_traces
    description: Find traces matching service, time, attributes, and duration criteria. Returns metadata only.
    input_schema:
      start_time_min: string (optional, default: "-1h") - Start of time interval. Supports RFC3339 or relative (e.g., "-1h", "-30m")
      start_time_max: string (optional) - End of time interval. Supports RFC3339 or relative (e.g., "now", "-1m"). Default: now
      service_name: string (required) - Filter by service name. Use get_services to discover valid names.
      span_name: string (optional) - Filter by span name. Use get_span_names to discover valid names.
      attributes: object (optional) - Key-value pairs to match against span/resource attributes (e.g., {"http.status_code": "500"})
      with_errors: boolean (optional) - If true, only return traces containing error spans
      duration_min: duration string (optional, e.g., "2s", "100ms")
      duration_max: duration string (optional)
      limit: integer (default: 10, max: 100)
    output: List of trace summaries (trace_id, service_count, span_count, duration, has_errors)

  - name: get_trace_topology
    description: Get the structural tree of a trace showing parent-child relationships, timing, and error locations. Does NOT return attributes or logs.
    input_schema:
      trace_id: string (required)
      depth: integer (optional, default: 3) - Maximum depth of the tree. 0 for full tree.
    output: Tree structure with span metadata (id, service, operation, duration, error flag, children[])

  - name: get_critical_path
    description: Identify the sequence of spans forming the critical latency path (the blocking execution path).
    input_schema:
      trace_id: string (required)
    output: Ordered list of spans on the critical path with timing information

  - name: get_span_details
    description: Fetch full details (attributes, events, links, status) for specific spans.
    input_schema:
      trace_id: string (required)
      span_ids: string[] (required, max 20)
    output: Full OTLP span data for requested spans

  - name: get_trace_errors
    description: Get full details for all spans with error status.
    input_schema:
      trace_id: string (required)
    output: Full OTLP span data for error spans only
```

### Sample Tool Outputs

#### get_services

Returns available service names for use in `search_traces`.

**Input:**
```json
{
  "pattern": "payment",        // optional: substring filter
  "limit": 100                 // optional: max results (default: 100)
}
```

**Output:**
```json
{
  "services": ["payment-service", "payment-gateway", "payment-processor"]
}
```

---

#### search_traces

Find traces matching criteria. Returns lightweight metadata only (no attributes/events).

**Input:**
```json
{
  "start_time_min": "-1h",           // required: RFC3339 or relative
  "start_time_max": "now",           // optional: default "now"
  "service_name": "frontend",        // required
  "operation_name": "/api/checkout", // optional
  "attributes": {                    // optional: match span/resource attributes
    "http.status_code": "500",
    "user.id": "12345"
  },
  "with_errors": true,               // optional: filter to error traces
  "duration_min": "2s",              // optional
  "duration_max": "10s",             // optional
  "limit": 10                        // optional: default 10, max 100
}
```

**Output:**
```json
{
  "traces": [
    {
      "trace_id": "1a2b3c4d5e6f7890",
      "root_service": "frontend",
      "root_operation": "/api/checkout",
      "start_time": "2024-01-15T10:30:00Z",
      "duration_ms": 2450,
      "span_count": 47,
      "service_count": 8,
      "has_errors": true
    }
  ]
}
```

---

#### get_trace_topology

Returns the structural skeleton of a trace—parent-child relationships, timing, and error locations—**without** loading attributes or events. This keeps the response small for LLM context.

**Input:**
```json
{
  "trace_id": "1a2b3c4d5e6f7890"
}
```

**Output:**
```json
{
  "trace_id": "1a2b3c4d5e6f7890",
  "root": {
    "span_id": "span_A",
    "service": "frontend",
    "operation": "/api/checkout",
    "start_time": "2024-01-15T10:30:00Z",
    "duration_ms": 2450,
    "status": "OK",
    "children": [
      {
        "span_id": "span_B",
        "service": "cart-service",
        "operation": "getCart",
        "start_time": "2024-01-15T10:30:00.050Z",
        "duration_ms": 120,
        "status": "OK",
        "children": []
      },
      {
        "span_id": "span_C",
        "service": "payment-service",
        "operation": "processPayment",
        "start_time": "2024-01-15T10:30:00.200Z",
        "duration_ms": 2200,
        "status": "ERROR",
        "children": [
          {
            "span_id": "span_D",
            "service": "payment-gateway",
            "operation": "chargeCard",
            "start_time": "2024-01-15T10:30:00.250Z",
            "duration_ms": 2100,
            "status": "ERROR",
            "children": []
          }
        ]
      }
    ]
  }
}
```

---

#### get_critical_path

Returns the sequence of spans that form the critical latency path—the "blocking" execution path that directly contributed to total trace duration.

**Input:**
```json
{
  "trace_id": "1a2b3c4d5e6f7890"
}
```

**Output:**
```json
{
  "trace_id": "1a2b3c4d5e6f7890",
  "total_duration_ms": 2450,
  "critical_path_duration_ms": 2400,
  "path": [
    {
      "span_id": "span_A",
      "service": "frontend",
      "operation": "/api/checkout",
      "self_time_ms": 50,
      "section_start_ms": 0,
      "section_end_ms": 50
    },
    {
      "span_id": "span_C",
      "service": "payment-service",
      "operation": "processPayment",
      "self_time_ms": 100,
      "section_start_ms": 50,
      "section_end_ms": 150
    },
    {
      "span_id": "span_D",
      "service": "payment-gateway",
      "operation": "chargeCard",
      "self_time_ms": 2100,
      "section_start_ms": 150,
      "section_end_ms": 2250
    },
    {
      "span_id": "span_A",
      "service": "frontend",
      "operation": "/api/checkout",
      "self_time_ms": 200,
      "section_start_ms": 2250,
      "section_end_ms": 2450
    }
  ]
}
```

> [!NOTE]
> A span may appear multiple times on the critical path (e.g., `span_A` above) if it has work both before and after its children execute.

---

#### get_span_details

Fetch full OTLP span data for specific spans. Use this only after identifying suspicious spans via topology or critical path.

**Input:**
```json
{
  "trace_id": "1a2b3c4d5e6f7890",
  "span_ids": ["span_C", "span_D"]   // max 20 spans
}
```

**Output:**
```json
{
  "trace_id": "1a2b3c4d5e6f7890",
  "spans": [
    {
      "span_id": "span_C",
      "trace_id": "1a2b3c4d5e6f7890",
      "parent_span_id": "span_A",
      "service": "payment-service",
      "operation": "processPayment",
      "start_time": "2024-01-15T10:30:00.200Z",
      "duration_ms": 2200,
      "status": {
        "code": "ERROR",
        "message": "Upstream service timeout"
      },
      "attributes": {
        "http.method": "POST",
        "http.url": "http://payment-gateway/charge",
        "http.status_code": "504",
        "retry.count": "3"
      },
      "events": [
        {
          "name": "retry_attempt",
          "timestamp": "2024-01-15T10:30:00.700Z",
          "attributes": {"attempt": "1"}
        },
        {
          "name": "retry_attempt",
          "timestamp": "2024-01-15T10:30:01.200Z",
          "attributes": {"attempt": "2"}
        }
      ],
      "links": []
    },
    {
      "span_id": "span_D",
      "trace_id": "1a2b3c4d5e6f7890",
      "parent_span_id": "span_C",
      "service": "payment-gateway",
      "operation": "chargeCard",
      "start_time": "2024-01-15T10:30:00.250Z",
      "duration_ms": 2100,
      "status": {
        "code": "ERROR",
        "message": "Connection timeout to payment processor"
      },
      "attributes": {
        "db.system": "postgresql",
        "db.statement": "SELECT * FROM transactions WHERE...",
        "net.peer.name": "payment-db.internal",
        "net.peer.port": "5432"
      },
      "events": [],
      "links": []
    }
  ]
}
```

---

#### get_trace_errors

Shortcut to get full details for all error spans in a trace.

**Input:**
```json
{
  "trace_id": "1a2b3c4d5e6f7890"
}
```

**Output:**
```json
{
  "trace_id": "1a2b3c4d5e6f7890",
  "error_count": 2,
  "spans": [
    // Same format as get_span_details output
    // Contains only spans where status.code == "ERROR"
  ]
}
```

### Configuration

```yaml
extensions:
  jaeger_mcp:
    # HTTP endpoint for MCP protocol (Streamable HTTP transport)
    http:
      endpoint: "0.0.0.0:4320"
    
    # Storage configuration (references jaegerstorage extension)
    storage:
      traces: "some_storage"
    
    # Server identification for MCP protocol
    server_name: "jaeger"
    server_version: "${version}"
    
    # Limits
    max_span_details_per_request: 20
    max_search_results: 100
```

### Extension Directory Structure

```
cmd/jaeger/internal/extension/jaegermcp/
├── README.md
├── config.go            # Configuration struct and validation
├── config_test.go
├── factory.go           # Extension factory (NewFactory, createDefaultConfig)
├── factory_test.go
├── server.go            # Extension lifecycle (Start, Shutdown, Dependencies)
├── server_test.go
└── internal/
    ├── criticalpath/    # Critical path algorithm (ported from UI)
    │   ├── criticalpath.go
    │   └── criticalpath_test.go
    ├── handlers/        # MCP tool handlers
    │   ├── search_traces.go
    │   ├── search_traces_test.go
    │   ├── get_trace_topology.go
    │   ├── get_critical_path.go
    │   ├── get_span_details.go
    │   ├── get_span_details_test.go
    │   ├── get_trace_errors.go
    │   └── get_trace_errors_test.go
    └── types/           # Response types for MCP tools (one file per handler)
        ├── search_traces.go
        ├── get_span_details.go
        └── get_trace_errors.go
```

## Consequences

### Positive

1. **AI Integration**: Enables LLM-based assistants to query and analyze Jaeger traces efficiently
2. **Token Optimization**: Progressive disclosure architecture prevents context exhaustion
3. **Standards Compliance**: Uses official MCP protocol, compatible with Claude, GPT, and other MCP-enabled agents
4. **Reusable Algorithm**: Go implementation of critical path can be used for API responses, not just MCP
5. **Clean Separation**: Runs on separate port, doesn't affect existing query service

### Negative

1. **Algorithm Duplication**: Critical path algorithm exists in both TypeScript (UI) and Go (MCP server)
2. **New Dependency**: Adds `github.com/modelcontextprotocol/go-sdk` to dependencies
3. **Maintenance Overhead**: Additional extension to maintain and test

### Mitigation

- Consider eventually exposing critical path via the gRPC query API for UI consumption, eliminating the TypeScript implementation
- The MCP SDK is official and well-maintained; dependency risk is low
- Extension follows established patterns, reducing maintenance burden

---

## Implementation Roadmap

### Phase 1: Foundation

1. **Extension Scaffold** ✅
   - Create `jaegermcp` extension directory structure
   - Implement `config.go` with configuration validation
   - Implement `factory.go` following `jaegerquery` pattern
   - Implement `server.go` with lifecycle management
   - Wire extension into component registration

2. **MCP Server Setup** ✅
   - Add `github.com/modelcontextprotocol/go-sdk` dependency
   - Initialize MCP server with Streamable HTTP transport
   - Implement server start/shutdown with graceful cleanup

---

### Phase 2: Basic Tools

3. **Storage Integration** ✅
   - Connect to `jaegerstorage` extension for trace reader access
   - Create internal service layer for trace operations

4. **Implement `get_services` Tool** ✅
   - Wrap `QueryService.GetServices()`
   - Support optional regex pattern filtering
   - Apply configurable limit (default: 100)
   - Return list of service names

4b. **Implement `get_span_names` Tool** ✅
   - Wrap `QueryService.GetOperations()`
   - Support optional regex pattern filtering
   - Support optional span kind filtering
   - Apply configurable limit (default: 100)
   - Return list of span names with span kind information

5. **Implement `search_traces` Tool** ✅
   - Wrap `QueryService.FindTraces()`
   - Transform response to MCP-optimized format (metadata only)
   - Add input validation and error handling

6. **Implement `get_span_details` Tool** ✅
   - Wrap `QueryService.GetTrace()`
   - Filter to requested span IDs only
   - Return full OTLP attribute data

7. **Implement `get_trace_errors` Tool** ✅
   - Wrap `QueryService.GetTrace()`
   - Filter to spans with error status
   - Return full OTLP attribute data

---

### Phase 3: Advanced Tools

7. **Implement `get_trace_topology` Tool** ✅
   - Fetch trace via `QueryService.GetTrace()`
   - Build tree structure from flat span list
   - **Strip attributes and events** before response
   - Include timing and error flags

8. **Port Critical Path Algorithm**
   - Study TypeScript implementation in `jaeger-ui/packages/jaeger-ui/src/components/TracePage/CriticalPath/`
   - Implement equivalent Go algorithm in `internal/criticalpath/`
   - Key components:
     - `findLastFinishingChildSpan()` - Find LFC for a span
     - `sanitizeOverFlowingChildren()` - Handle child spans that exceed parent duration
     - `computeCriticalPath()` - Main recursive algorithm
   - Add comprehensive unit tests with same test cases as UI

9. **Implement `get_critical_path` Tool**
   - Use critical path algorithm from step 8
   - Return ordered list of spans on critical path
   - Include timing breakdown

---

### Phase 4: Polish and Extend

10. **Configuration and Observability**
    - Add OpenTelemetry metrics for MCP tool invocations
    - Add structured logging for debugging
    - Implement rate limiting if needed

11. **Documentation**
    - Write `README.md` for the extension
    - Document MCP server instructions (system prompt) for LLM configuration
    - Add example configurations

12. **Integration Testing**
    - End-to-end tests with mock storage
    - Test MCP protocol compliance
    - Performance testing with large traces

---

## Testing Strategy

### Unit Tests

| Component | Testing Approach |
|-----------|------------------|
| `config.go` | Test validation with valid/invalid configs |
| `factory.go` | Test factory creation and default config |
| `server.go` | Test lifecycle with mock storage extension |
| Critical path algorithm | Port test cases from TypeScript tests; use same expected results |
| Tool handlers | Mock `QueryService`; test input validation, response format, error handling |

### Integration Tests

1. **Extension Lifecycle**
   - Test extension starts with valid configuration
   - Test graceful shutdown
   - Test dependency resolution with `jaegerstorage`

2. **MCP Protocol Compliance**
   - Use MCP SDK client to connect to server
   - Verify tool discovery (`tools/list`)
   - Verify tool invocation and response format
   - Test error handling (invalid inputs, missing traces)

3. **End-to-End Scenarios**
   - Use memory storage with sample traces
   - Execute progressive disclosure workflow (search → topology → critical path → details)
   - Verify token efficiency (topology response is smaller than full trace)

### Test Fixtures

Reuse existing test fixtures from:
- `cmd/jaeger/internal/extension/jaegerquery/internal/fixture/` - Sample traces
- `jaeger-ui/packages/jaeger-ui/src/components/TracePage/CriticalPath/testCases/` - Critical path test cases

### CI Integration

- Add to existing CI workflow
- Include in `make test` target
- Add to code coverage requirements

---

## References

- [Model Context Protocol Specification](https://modelcontextprotocol.io/)
- [MCP Go SDK Documentation](https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk/mcp)
- Jaeger Extension Pattern: [`cmd/jaeger/internal/extension/jaegerquery/`](../../cmd/jaeger/internal/extension/jaegerquery/)
- Critical Path UI Implementation: [`jaeger-ui/packages/jaeger-ui/src/components/TracePage/CriticalPath/index.tsx`](../../jaeger-ui/packages/jaeger-ui/src/components/TracePage/CriticalPath/index.tsx)
- Original Design Document: [`design.md`](../../design.md)
