# AI Trace Analysis API (Level-1 PoC)

> **Status:** Prototype / Proof of Concept
> **Related issue:** [#7832 – AI-Powered Trace Analysis with Local LLM Support](https://github.com/jaegertracing/jaeger/issues/7832)
> **Roadmap reference:** [#7827 – GenAI integration with Jaeger (Level 1)](https://github.com/jaegertracing/jaeger/issues/7827)

## Overview

This endpoint provides Level-1 AI-powered trace analysis: a user asks a free-form question about a single trace, and the system returns a natural-language explanation.

The architecture follows the GenAI roadmap:
```
User Question + Trace ID
        |
        v
  jaegerquery         POST /api/ai/analyze
  (AI Handler)        POST /api/ai/search
        |  Fetches trace via TraceReader
        v
  AIService           Prunes trace for LLM context
        |  Pruned trace + prompt
        v
  Local LLM           e.g. Ollama / phi3 (port 11434)
        |  Natural language answer
        v
  JSON Response
```

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

| Field      | Type   | Required | Description                              |
|------------|--------|----------|------------------------------------------|
| `traceID`  | string | Yes      | The trace ID to analyze.                 |
| `question` | string | Yes      | A natural-language question about the trace. |

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

#### Response (Error)
```json
{
  "errors": [
    {
      "code": 400,
      "msg": "traceID is required"
    }
  ]
}
```

| HTTP Status | When                                          |
|-------------|-----------------------------------------------|
| 200         | Analysis completed successfully.               |
| 400         | Missing or invalid `traceID` or `question`.   |
| 501         | AI service is not configured.                  |
| 500         | LLM unavailable / internal error.              |

### `POST /api/ai/search`

Translate a natural language question into structured Jaeger search parameters using the local SLM.

#### Request
```json
{
  "question": "Find slow checkout requests in payment-service"
}
```

| Field      | Type   | Required | Description                                   |
|------------|--------|----------|-----------------------------------------------|
| `question` | string | Yes      | A natural-language search query to translate.  |

#### Response (Success)
```json
{
  "data": {
    "originalQuestion": "Find slow checkout requests in payment-service",
    "parameters": {
      "service": "payment-service",
      "operation": "checkout",
      "tags": { "error": "true" },
      "minDuration": "",
      "maxDuration": ""
    }
  },
  "total": 1
}
```

| HTTP Status | When                                          |
|-------------|-----------------------------------------------|
| 200         | Parameters extracted successfully.             |
| 400         | Missing or invalid `question`.                 |
| 501         | AI service is not configured.                  |
| 500         | SLM extraction failed / internal error.        |

## Current Status (PoC)

- **LLM client**: Ollama integration via LangChainGo (`phi3` model by default). Falls back to a stub client if Ollama is unavailable.
- **Trace pruning**: Error spans and their immediate parents are extracted and formatted for the LLM context window.
- **Search parameter extraction**: SLM translates natural language to structured Jaeger search JSON.

## Future Work

1. Real MCP integration via HTTP client on port 16687.
2. Streaming responses (SSE).
3. Evidence linking (span IDs in answers).
4. Benchmarks per #7827.
5. UI integration.
