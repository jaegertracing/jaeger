# jaeger_mcp

This extension implements a Model Context Protocol (MCP) server for Jaeger, enabling LLM-based assistants to query and analyze distributed traces efficiently.

## Overview

The MCP server provides a structured way for AI agents to interact with Jaeger's trace data using progressive disclosure:
- **Search** → Find traces matching specific criteria
- **Map** → Visualize trace structure without loading full attribute data  
- **Diagnose** → Identify critical execution paths that contributed to latency or errors
- **Inspect** → Load full details only for specific, suspicious spans

This approach prevents context-window exhaustion in LLMs and enables more efficient trace analysis.

**Note:** The current implementation uses Streamable HTTP transport only. MCP stdio transport is not supported.

## Status

✅ **Phase 1: Foundation (Complete)** - Extension scaffold, lifecycle management, and MCP SDK integration

✅ **Phase 2: Storage Integration (Complete)** - Connection to jaegerquery extension for trace access

🚧 **Phase 3: Advanced Tools (In Progress)** - Critical path analysis

Future phases will add:
- Phase 2: Remaining basic MCP tools (search, span details, errors, get_services)
- Phase 3: Remaining advanced tools (topology)
- Phase 4: Documentation and observability

See [ADR-002](../../../../docs/adr/002-mcp-server.md) for full design details.

## Available Tools

### Phase 1
- ✅ `health` - Check server health and status

### Phase 3
- ✅ `get_critical_path` - Identify the sequence of spans forming the critical latency path

## Configuration

```yaml
extensions:
  jaeger_mcp:
    # HTTP endpoint for MCP protocol (Streamable HTTP transport)
    http:
      endpoint: "0.0.0.0:16687"
    
    # Server identification for MCP protocol
    server_name: "jaeger"
    # server_version will default to the build version
    
    # Limits
    max_span_details_per_request: 20
    max_search_results: 100
```

## Dependencies

This extension depends on the [jaeger_query](../jaegerquery/) extension to access trace data. The `jaeger_query` extension must be configured in the service extensions list.

## Development Status

### Phase 1 (Complete)
- ✅ Extension directory structure
- ✅ Configuration validation
- ✅ Factory implementation
- ✅ Server lifecycle management
- ✅ MCP SDK integration
- ✅ Streamable HTTP transport
- ✅ Basic health tool

### Phase 2 (Partial)
- ✅ Storage integration with jaegerquery extension

### Phase 3 (Partial)
- ✅ Critical path algorithm ported from UI
- ✅ `get_critical_path` tool implementation
