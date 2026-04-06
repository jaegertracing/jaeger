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

See [ADR-002](/docs/adr/002-mcp-server.md) for full design details.

## Available Endpoints

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
```shell
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
```shell
  < Mcp-Session-Id: SAWYSMIJP3CA6P6PONC4QB3QLT
```

Get list of tools (use session ID from output above):
```shell
curl -X POST http://localhost:16687/mcp \
  -H "Content-Type: application/json" \
  -H "Mcp-Session-Id: SAWYSMIJP3CA6P6PONC4QB3QLT" \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}'
```

## Sample usage with Claude Code

```shell
claude mcp add -t http jaeger-mcp http://127.0.0.1:16687/mcp
```

```
❯ which services are in my Jaeger instance

⏺ Your Jaeger instance has the following services:

  1. customer
  2. driver
  3. frontend
  4. jaeger
  5. mysql
  6. redis-manual
  7. route

  This looks like the HotROD demo application (a ride-sharing example), plus the jaeger service itself for internal tracing.

❯ how many traces  for frontend service

⏺ In the last hour, there are 6 traces for the frontend service:

  ┌───────────┬──────────┬────────┬──────────┬───────┐
  │ Span Name │ Duration │ Errors │ Services │ Spans │
  ├───────────┼──────────┼────────┼──────────┼───────┤
  │ /dispatch │ 1,114 ms │ Yes    │ 6        │ 40    │
  ├───────────┼──────────┼────────┼──────────┼───────┤
  │ /config   │ <1 ms    │ No     │ 1        │ 1     │
  ├───────────┼──────────┼────────┼──────────┼───────┤
  │ /config   │ <1 ms    │ No     │ 1        │ 1     │
  ├───────────┼──────────┼────────┼──────────┼───────┤
  │ /config   │ <1 ms    │ No     │ 1        │ 1     │
  ├───────────┼──────────┼────────┼──────────┼───────┤
  │ /dispatch │ 901 ms   │ Yes    │ 6        │ 39    │
  ├───────────┼──────────┼────────┼──────────┼───────┤
  │ /dispatch │ 760 ms   │ Yes    │ 6        │ 40    │
  └───────────┴──────────┴────────┴──────────┴───────┘

  Notable: all 3 /dispatch traces have errors, while the /config traces are clean.

❯ how many errors in the longest trace?

⏺ The longest trace has 3 errors, all of the same type:

  - Service: redis-manual
  - Span: GetDriver
  - Error: redis timeout
  - Affected drivers: T717023C, T732667C, T781220C

  All three errors occurred within ~170ms of each other, suggesting a Redis connectivity issue that caused timeouts when looking up
  driver information.

❯ find the longest trace from frontend service, list spans that have errors and show their parent spans as "service/span_name"

⏺ Here are the results for the longest frontend trace (e07fc217a00814b4ef552a10466da417, 908 ms, 39 spans, 6 services):

  ---
  Error Spans (2 errors)

  Both errors are redis timeout exceptions in the redis-manual service:

  ┌─────┬──────────────┬───────────────────────────┬─────────────────────────────────────┬─────────────────────────────────────────┐
  │  #  │   Service    │           Span            │                Error                │               Parent Span               │
  ├─────┼──────────────┼───────────────────────────┼─────────────────────────────────────┼─────────────────────────────────────────┤
  │ 1   │ redis-manual │ GetDriver (span 68a98f61) │ redis timeout for driverID T772373C │ driver/driver.DriverService/FindNearest │
  ├─────┼──────────────┼───────────────────────────┼─────────────────────────────────────┼─────────────────────────────────────────┤
  │ 2   │ redis-manual │ GetDriver (span 0d18a2ce) │ redis timeout for driverID T710636C │ driver/driver.DriverService/FindNearest │
  └─────┴──────────────┴───────────────────────────┴─────────────────────────────────────┴─────────────────────────────────────────┘

  ---
  Both error spans share the same parent: driver/driver.DriverService/FindNearest (span 42117b7675f3f8f8). The driver service was
  looking up nearby drivers in Redis, and 2 out of the ~13 GetDriver lookups timed out.
```

## With MCP Inspector

```shell
npx @modelcontextprotocol/inspector
```

* Transport Type: Streamable HTTP
* URL: http://localhost:16687/mcp
