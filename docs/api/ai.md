# AI Trace Analysis API (Level-1 PoC)

> **Status:** Prototype / Proof of Concept
> **Related issue:** [#7832 – AI-Powered Trace Analysis with Local LLM Support](https://github.com/jaegertracing/jaeger/issues/7832)
> **Roadmap reference:** [#7827 – GenAI integration with Jaeger (Level 1)](https://github.com/jaegertracing/jaeger/issues/7827)

## Overview

This endpoint provides Level-1 AI-powered trace analysis: a user asks a free-form question about a single trace, and the system returns a natural-language explanation.

The architecture follows ADR-002 (MCP Server) and the GenAI roadmap:
```
User Question + Trace ID
        |
        v
  jaegerquery         POST /api/ai/analyze
  (AI Handler)
        |  Fetches trace context
        v
  jaegermcp           MCP tools (port 16687)
  (MCP Server)        - get_trace_topology
                      - get_critical_path
        |  Trace context returned
        v
  Local LLM           e.g. Ollama (port 11434)
        |  Natural language answer
        v
  JSON Response
```

## Endpoint

### `POST /api/ai/analyze`

Analyze a trace using AI. Accepts a trace ID and a natural-language question.

#### Request
```json
{
  "trace_id": "abc123def456789",
  "question": "Why is this trace slow?"
}
```

| Field      | Type   | Required | Description                              |
|------------|--------|----------|------------------------------------------|
| `trace_id` | string | Yes      | The trace ID to analyze.                 |
| `question` | string | Yes      | A natural-language question about the trace. |

#### Response (Success)
```json
{
  "data": {
    "trace_id": "abc123def456789",
    "answer": "The payment-service is the primary bottleneck, consuming 120ms of the 177ms critical path."
  },
  "total": 1
}
```

#### Response (Error)
```json
{
  "errors": [
    {
      "code": 400,
      "msg": "trace_id is required"
    }
  ]
}
```

| HTTP Status | When                                          |
|-------------|-----------------------------------------------|
| 200         | Analysis completed successfully.               |
| 400         | Missing or invalid `trace_id` or `question`.   |
| 501         | AI service is not configured.                  |
| 500         | MCP server or LLM unavailable / internal error.|

## Current Status (PoC)

- **MCP client**: Stub returning fixed topology and critical path strings.
- **LLM client**: Stub returning a fixed analysis.

## Future Work

1. Real MCP integration via HTTP client on port 16687.
2. Real LLM integration via Ollama/LangChainGo on port 11434.
3. Streaming responses (SSE).
4. Evidence linking (span IDs in answers).
5. Benchmarks per #7827.
6. UI integration.
