# jaeger_mcp

This extension implements a Model Context Protocol (MCP) server for Jaeger, enabling LLM-based assistants to query and analyze distributed traces efficiently.

## Overview

The MCP server provides a structured way for AI agents to interact with Jaeger's trace data using progressive disclosure:
- **Search** → Find traces matching specific criteria
- **Map** → Visualize trace structure without loading full attribute data  
- **Diagnose** → Identify critical execution paths that contributed to latency or errors
- **Inspect** → Load full details only for specific, suspicious spans

This approach prevents context-window exhaustion in LLMs and enables more efficient trace analysis.

## Status

🚧 **Phase 1: Foundation (In Progress)** - Extension scaffold and lifecycle management

Future phases will add:
- Phase 2: Basic MCP tools (search, span details, errors)
- Phase 3: Advanced tools (topology, critical path)
- Phase 4: Documentation and observability

See [ADR-002](../../../../docs/adr/002-mcp-server.md) for full design details.

## Configuration

```yaml
extensions:
  jaeger_mcp:
    # HTTP endpoint for MCP protocol (Streamable HTTP transport)
    http:
      endpoint: "0.0.0.0:4320"
    
    # Server identification for MCP protocol
    server_name: "jaeger"
    server_version: "dev"
    
    # Limits
    max_span_details_per_request: 20
    max_search_results: 100
```

## Dependencies

This extension depends on the [jaeger_query](../jaegerquery/) extension to access trace data.

## Development Status

Phase 1 implements:
- ✅ Extension directory structure
- ✅ Configuration validation
- ✅ Factory implementation
- ✅ Server lifecycle management
- ✅ Basic health endpoint
- 🚧 MCP SDK integration (coming in Phase 2)
