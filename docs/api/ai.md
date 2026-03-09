# AI Trace Analysis API (Level-1 PoC)

> **Status:** Prototype / Proof of Concept
> **Related issue:** [#7832 – AI-Powered Trace Analysis with Local LLM Support](https://github.com/jaegertracing/jaeger/issues/7832)
> **Roadmap reference:** [#7827 – GenAI integration with Jaeger (Level 1)](https://github.com/jaegertracing/jaeger/issues/7827)

## Overview

This API provides Level-1 AI-powered trace analysis via two endpoints:

- **`POST /api/ai/analyze`** – Ask a free-form question about a single trace.
- **`POST /api/ai/search`** – Translate a natural-language query into Jaeger search parameters.

### Architecture

```
User Question
      |
      v
jaegerquery                POST /api/ai/analyze
(AIHandler + AIService)    POST /api/ai/search
      |                          |
      |  /api/ai/analyze         |  /api/ai/search
      v                          v
 MCPClient (stub)          buildSearchAnalysisPrompt
 - get_trace_topology      (JSON-schema constrained)
 - get_critical_path             |
      |                          v
      +-------> LLMClient <------+
                (Ollama / Stub)
                    |
                    v
              JSON Response
```

**LLM backend:** When Ollama is reachable (default model: `phi3`), requests are
routed through `OllamaLLMClient` via LangChainGo. JSON mode is enabled
automatically for `/api/ai/search` prompts. If Ollama is unavailable,
the service falls back to `StubLLMClient`, which returns fixed responses.

## Endpoints

### `POST /api/ai/analyze`

Analyze a trace using AI. Accepts a trace ID and a natural-language question.

#### Request

```json
{
  "traceID": "abc123def456789",
  "question": "Why is this trace slow?"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `traceID` | string | Yes | The trace ID to analyze. |
| `question` | string | Yes | A natural-language question about the trace. |

#### Response (Success)

```json
{
  "data": {
    "traceID": "abc123def456789",
    "answer": "The payment-service is the primary bottleneck, consuming 120ms of the 177ms critical path."
  },
  "total": 1
}
```

### `POST /api/ai/search`

Translate a natural-language question into structured Jaeger search parameters.

#### Request

```json
{
  "question": "Show me slow traces from the payment service over 2 seconds"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `question` | string | Yes | A natural-language search query. |

#### Response (Success)

```json
{
  "data": {
    "originalQuestion": "Show me slow traces from the payment service over 2 seconds",
    "parameters": {
      "service": "payment-service",
      "operation": "",
      "tags": {},
      "minDuration": "2s",
      "maxDuration": ""
    }
  },
  "total": 1
}
```

### Error Responses

```json
{
  "errors": [
    {
      "code": 400,
      "msg": "question is required"
    }
  ]
}
```

| HTTP Status | When |
|-------------|------|
| 200 | Request completed successfully. |
| 400 | Missing or invalid request fields. |
| 501 | AI service is not configured. |
| 500 | LLM unavailable or internal error. |

## Current Status

- **MCP client:** Stub returning fixed topology and critical path strings.
- **LLM client:** Ollama integration via LangChainGo (`phi3` model). Falls back to stub when Ollama is unavailable.
- **Search extraction:** Uses JSON-schema constrained prompting with `DisallowUnknownFields` validation.

## Future Work

1. Real MCP integration via HTTP client on port 16687.
2. Streaming responses (SSE).
3. Evidence linking (span IDs in answers).
4. Benchmarks per #7827.
5. UI integration.
