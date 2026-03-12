# jaeger_mcp

This extension implements a Model Context Protocol (MCP) server for Jaeger, enabling LLM-based assistants to query and analyze distributed traces efficiently.

## Overview

The MCP server provides a structured way for AI agents to interact with Jaeger's trace data using progressive disclosure:
- **Search** → Find traces matching specific criteria
- **Map** → Visualize trace structure without loading full attribute data  
- **Diagnose** → Identify critical execution paths that contributed to latency or errors
- **Inspect** → Load full details only for specific, suspicious spans

This approach prevents context-window exhaustion in LLMs and enables more efficient trace analysis.

**Note:** The current implementation uses Streamable HTTP transport only. MCP `stdio` transport is not supported.

See [ADR-002](../../../../docs/adr/002-mcp-server.md) for full design details.

## Available Endpoints

* `/health`
* `/mcp`

## Available Tools

* `get_services`
* `get_span_names`
* `search_traces`
* `get_trace_topology`
* `get_trace_errors`
* `get_span_details`
* `get_critical_path`
* `health`

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

## Sample usage

Establish session:
```
curl -v -X POST http://localhost:16687/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "initialize",
    "params": {
      "protocolVersion": "2024-11-05",
      "capabilities": {},
      "clientInfo": { "name": "curl-client", "version": "1.0.0" }
    }
  }'
```

Look for session ID header like this:
```
  < Mcp-Session-Id: SAWYSMIJP3CA6P6PONC4QB3QLT
```

Get list of tools (use session ID from output above):
```
curl -X POST http://localhost:16687/mcp \
  -H "Content-Type: application/json" \
  -H "Mcp-Session-Id: SAWYSMIJP3CA6P6PONC4QB3QLT" \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}'
```
