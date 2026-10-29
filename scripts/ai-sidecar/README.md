# External ACP Agents

Jaeger's AI gateway connects the UI to an external agent over the
[Agent Client Protocol (ACP)](https://agentclientprotocol.com/). Jaeger ships no
agent and selects no model provider. The operator runs an ACP agent and points
Jaeger at it over WebSocket.

[Goose](./goose/README.md) is the worked example: an off-the-shelf ACP agent
that meets the contract below with no Jaeger-specific code. Any other agent that
meets the same contract works the same way.

## Configure Jaeger

Set the agent's WebSocket URL in the `jaeger_query` extension configuration.
Use `agent_headers` for any credentials the agent expects on the WebSocket
upgrade:

```yaml
extensions:
  jaeger_query:
    ai:
      agent_url: ws://127.0.0.1:16688/acp
      agent_headers:
        Authorization: Bearer ${env:MY_AGENT_TOKEN}
      # Enables Jaeger's MCP endpoint for the agent.
      mcp: {}
```

The gateway redacts `agent_headers` values from logs and configuration dumps.
The URL path and the header names are whatever the chosen agent requires.

## Agent contract

This is what the gateway requires of an agent, as the gateway implements it
today. The gateway code in
[`jaegerai`](../../cmd/jaeger/internal/extension/jaegerquery/internal/jaegerai/README.md)
is the reference for any detail this page leaves out.

**Transport.** The agent serves ACP JSON-RPC over a WebSocket at `agent_url`.
Each WebSocket message carries one JSON-RPC object with no newline framing.
The gateway sends `agent_headers` on every dial, so an agent may require an
authentication header of its choosing.

**Health.** The gateway periodically dials the agent and sends `initialize`
and nothing else. The agent must answer it on a fresh connection. The UI shows
the chat surface only while that check succeeds, and
`ai.health_check_interval` controls how often it runs.

**Capabilities.** The agent's `initialize` response must advertise
`agentCapabilities.mcpCapabilities.http`. Without it the gateway announces no
MCP server, and the agent runs every turn with zero Jaeger tools. The gateway
advertises no `fs` and no `terminal` client capability, so the agent must not
depend on either.

**Sessions and turns.** Each chat request is one `initialize`, one
`session/new` and one `session/prompt` on a fresh connection. Session ids are
never reused. `session/new` carries `mcpServers` with one Streamable HTTP
server entry, the turn-scoped endpoint URL plus any headers. The agent must
connect to that URL for this turn only, treat it as opaque, and not reuse it
after the prompt completes.

**Tools.** Jaeger's telemetry tools and the UI tools that the browser supplied
with the chat request arrive together in one `tools/list` from that endpoint.
UI tools are namespaced `ui__<name>` so they cannot shadow a telemetry tool.
The agent calls whatever it was offered, using the names and JSON schemas as
served. A UI tool returns an acknowledgement, not data, and the browser performs
the effect. Caching tools across turns is wrong, because the UI tool set differs
per request.

**Streaming.** The gateway translates `session/update` notifications into
AG-UI events for the browser: `agent_message_chunk` carries text, and
`tool_call` and `tool_call_update` with `rawInput` and `rawOutput` carry tool
progress. The gateway accepts thought chunks and plans and drops them. The
gateway answers `session/request_permission` by selecting the first option of
kind `allow_once` or `allow_always`, so an agent may ask but must not depend on
a human.

**Tracing.** `session/prompt` carries `traceparent` and `tracestate` in `_meta`,
using the keys that
[SEP-414](https://modelcontextprotocol.io/seps/414-request-meta) defines for MCP.
An agent that reads them and re-injects them into `_meta` on each MCP
`tools/call` joins its spans to the chat request's trace. This is optional, but
it is what makes the agent visible in Jaeger's own traces.

**Finish.** The agent returns `session/prompt` with a `stopReason`, which ends
the AG-UI run.

## Verify an integration

1. Start the agent with its own provider credentials and authentication.
2. Start Jaeger with `agent_url`, the required `agent_headers`, and `mcp: {}`.
3. Confirm the health check passes:
   ```bash
   curl -s http://127.0.0.1:16686/api/v3/capabilities | jq .aiAssistant
   ```
   The output is `true` once the agent answers `initialize`.
4. Send a chat request without the browser:
   ```bash
   curl -N -X POST http://127.0.0.1:16686/api/ai/chat \
     -H 'Content-Type: application/json' \
     -d '{
       "threadId": "t1",
       "runId": "r1",
       "messages": [{"role": "user", "content": "what services are running?"}],
       "tools": []
     }'
   ```
   The response is a stream of AG-UI SSE frames: `RUN_STARTED`,
   `TEXT_MESSAGE_START`, one or more `TEXT_MESSAGE_CONTENT`, `TOOL_CALL_*`
   frames for each Jaeger tool the agent called, then `TEXT_MESSAGE_END` and
   `RUN_FINISHED`.
5. Pass a UI tool with the request and ask for it:
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
   The stream carries `TOOL_CALL_START`, `TOOL_CALL_ARGS` and `TOOL_CALL_END`
   for `show_flamegraph`. The agent sees the tool as `ui__show_flamegraph`, and
   the gateway strips the prefix before the event reaches the browser.

## Related documentation

- [Goose configuration example](./goose/README.md)
- [AI gateway architecture](../../cmd/jaeger/internal/extension/jaegerquery/internal/jaegerai/README.md)
- [MCP endpoint documentation](../../cmd/jaeger/internal/extension/jaegerquery/internal/mcptools/README.md)
- [Agent Client Protocol](https://agentclientprotocol.com/)
