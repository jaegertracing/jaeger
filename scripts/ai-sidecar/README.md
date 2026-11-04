# External ACP Agents

Jaeger's AI gateway connects the UI to an external agent over the
[Agent Client Protocol (ACP)](https://agentclientprotocol.com/). Jaeger ships no
agent and selects no model provider. The operator runs an ACP agent and points
Jaeger at it over WebSocket.

The website documents how to configure the gateway, the contract an agent must
meet, how to run [Goose](./goose/README.md) as the agent, and how to verify an
integration: see
[AI Assistant](https://www.jaegertracing.io/docs/latest/ai/assistant/). The
gateway code in
[`jaegerai`](../../cmd/jaeger/internal/extension/jaegerquery/internal/jaegerai/README.md)
is the reference for any detail that page leaves out.

## Exercising a UI tool without the browser

The website's verification steps send a chat request with no UI tools. To see
the gateway dispatch a UI tool, pass one with the request and ask for it:

```bash
curl -N -X POST http://127.0.0.1:16686/api/ai/chat \
  -H 'Content-Type: application/json' \
  -d '{
    "threadId": "t1", "runId": "r2",
    "messages": [{"role": "user", "content": "show the flamegraph for trace abc123"}],
    "tools": [{
      "name": "show_flamegraph",
      "description": "Open the flamegraph view for a trace_id.",
      "parameters": {"type":"object","properties":{"trace_id":{"type":"string"}},"required":["trace_id"]}
    }]
  }'
```

The stream carries `TOOL_CALL_START`, `TOOL_CALL_ARGS` and `TOOL_CALL_END` for
`show_flamegraph`. The agent sees the tool as `ui__show_flamegraph`, and the
gateway strips the prefix before the event reaches the browser.
