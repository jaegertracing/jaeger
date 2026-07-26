# MCP Server for Jaeger

* **Status**: Implemented ([#8423](https://github.com/jaegertracing/jaeger/pull/8423)); graduated from [RFC 0012](../rfc/0012-mcp-server-extension.md). Decisions 2 and 3 are superseded — see below
* **Decided**: 2026-01-23
* **Describes the implementation as of**: 2026-07-25

> **Graduation note:** The proposal behind this decision — the full tool specification, sample tool outputs, the configuration and directory layout, the phased roadmap and the testing strategy — lives in [RFC 0012](../rfc/0012-mcp-server-extension.md). This ADR records only the decisions and which of them still hold. [RFC 0008](../rfc/0008-ai-gateway-mcp-tool-routing.md) is the source of truth for how tool calls are routed today.

## Context

Jaeger holds the telemetry an LLM agent needs to diagnose a distributed system, but a single trace can carry thousands of spans — far more than is useful to put in a context window, and dumping one wholesale produces worse answers, not better ones. The [Model Context Protocol](https://modelcontextprotocol.io/) gives agents a structured way to discover and invoke tools, so Jaeger can expose its query capabilities as a set of narrow tools instead of a firehose.

## Decision

### 1. Expose Jaeger telemetry to agents as MCP tools ✅ stands

Jaeger serves an MCP endpoint whose tools wrap the existing `QueryService`, rather than expecting agents to drive the HTTP query API directly. The tool set has grown past what [RFC 0012](../rfc/0012-mcp-server-extension.md) specified and now covers `get_services`, `get_span_names`, `search_traces`, `get_span_details`, `get_trace_errors`, `get_trace_topology`, `get_critical_path`, `get_service_dependencies` and `read_skill`, the last serving built-in playbooks from an embedded filesystem.

### 2. Run as a standalone `jaeger_mcp` extension on its own port ⚠️ superseded

Shipped that way in [#8423](https://github.com/jaegertracing/jaeger/pull/8423), serving `:16687`, then retired by [RFC 0008](../rfc/0008-ai-gateway-mcp-tool-routing.md) M1 ([#8894](https://github.com/jaegertracing/jaeger/pull/8894)). The tools are now a library — [`mcptools`](../../cmd/jaeger/internal/extension/jaegerquery/internal/mcptools/) — inside the query extension, served under `<base_path>/api/ai/mcp/` on the **query port** and gated by `enable_mcp` (off by default). One MCP implementation now backs both that endpoint and the turn-scoped one the AI gateway announces.

### 3. Obtain `QueryService` from the `jaegerquery` extension ⚠️ superseded

This followed from decision 2 and went away with it. Living inside the query extension, `mcptools` receives `QueryService` directly; there is no inter-extension lookup and no runtime coupling between two extensions to keep working.

### 4. Implement the critical-path algorithm in Go ✅ stands

The algorithm existed only in the UI's TypeScript. It was ported to [`mcptools/internal/criticalpath/`](../../cmd/jaeger/internal/extension/jaegerquery/internal/mcptools/internal/criticalpath/), which is what makes `get_critical_path` possible server-side.

### 5. Use progressive disclosure to keep responses small ✅ stands

The tools are shaped for a drill-down workflow — search for candidate traces, map a trace's topology without attributes, find the critical path, then inspect only the suspicious spans — so an agent narrows down to the relevant span without ever loading a whole trace. This is the design's central premise and the reason the tool set is shaped the way it is rather than mirroring the query API.

## Consequences

- Agents reach Jaeger through a narrow, purpose-built surface, and each tool can shape its response for an LLM rather than for a UI.
- The critical-path algorithm now exists twice, in Go and in the UI's TypeScript, and the two can drift.
- Because the tools are a library rather than an extension, adding one is a local change with no configuration surface of its own; the trade-off is that MCP availability is tied to the query extension being enabled.
- Serving MCP on the query port removes a listener and its TLS/auth configuration, at the cost of the isolation a separate port gave.

## References

* [RFC 0012: MCP Server Extension for Jaeger](../rfc/0012-mcp-server-extension.md) — the original proposal: tool specifications, sample outputs, roadmap and testing strategy
* [RFC 0008: AI Gateway — Unified MCP Tool Routing](../rfc/0008-ai-gateway-mcp-tool-routing.md) — current tool routing, and the reason decisions 2 and 3 were reversed
* Tools and server: [`cmd/jaeger/internal/extension/jaegerquery/internal/mcptools/`](../../cmd/jaeger/internal/extension/jaegerquery/internal/mcptools/)
* [Model Context Protocol Specification](https://modelcontextprotocol.io/)
