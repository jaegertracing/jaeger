# External ACP Agents

Jaeger's AI gateway uses the [Agent Client Protocol (ACP)](https://agentclientprotocol.com/)
to connect the UI to an external agent. Jaeger does not ship an agent or select
a model provider. Operators run an ACP-compatible agent and configure Jaeger to
reach it over WebSocket.

[Goose](./goose/README.md) is a working configuration example. Any agent that
meets the contract below can be used instead.

## Configure Jaeger

Set the agent's WebSocket URL in the `jaeger_query` extension configuration.
Use `agent_headers` for any credentials that the agent expects during the
WebSocket upgrade:

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

`agent_headers` values are redacted from logs and configuration dumps. Prefer
environment-variable expansion to inline credentials. The agent URL and header
names are defined by the selected agent.

The gateway periodically calls ACP `initialize` at `agent_url` to determine
whether the agent is available. The AI assistant is shown in the UI only while
that health check succeeds, so the configured agent must accept a connection
and complete `initialize` without a chat request.

## Agent contract

An external agent must serve ACP JSON-RPC over the configured WebSocket URL.
For each chat turn, the gateway opens a connection and sends these requests in
order:

1. `initialize` — the agent responds with its protocol version, implementation
   details, and capabilities.
2. `session/new` — the agent allocates and returns a session ID.
3. `session/prompt` — the agent runs the turn and returns its stop reason.

The gateway advertises no filesystem or terminal capability. An agent must not
depend on either one and should expect `session/request_permission` to be
denied.

While handling `session/prompt`, the agent sends `session/update`
notifications for assistant text and tool-call progress. Jaeger converts these
updates into AG-UI server-sent events for the browser. Use the ACP SDK for the
agent's language when available; it supplies the required JSON-RPC framing and
message types.

## Use Jaeger's tools through MCP

When `ai.mcp` is configured, Jaeger advertises a turn-scoped Streamable HTTP
MCP server in `session/new`. An agent that supports this integration must
advertise the ACP HTTP MCP capability during `initialize`, read the supplied
MCP server from `session/new`, and use that URL for the lifetime of the turn.

The MCP server exposes Jaeger's telemetry tools and any UI tools supplied with
the chat request. Treat the announced URL as opaque and turn-scoped: do not
construct it from the Jaeger host or reuse it after the prompt completes.
The server's tool schemas are the source of truth; translate them into the
model provider's function-calling format without changing tool names or JSON
schemas.

## Verify an integration

1. Start the ACP agent with its own provider credentials and authentication.
2. Start Jaeger with `agent_url`, required `agent_headers`, and `mcp: {}`.
3. Open the Jaeger UI after the health check succeeds.
4. Ask a question that requires telemetry, such as `list the services you can
   see`, and confirm that the agent calls a tool advertised by Jaeger's MCP
   server.
5. Add an AG-UI contextual tool to a chat request and confirm that the agent
   receives it through the announced MCP server and emits tool-call progress.

Agent implementations should test the complete ACP lifecycle, MCP tool
discovery and invocation, authentication headers, and cleanup after successful
and failed prompts. Jaeger's gateway package has its own protocol tests; an
external agent should test its integration in its own repository.

## Related documentation

- [Goose configuration example](./goose/README.md)
- [AI gateway architecture](../../cmd/jaeger/internal/extension/jaegerquery/internal/jaegerai/README.md)
- [MCP endpoint documentation](../../cmd/jaeger/internal/extension/jaegerquery/internal/mcptools/README.md)
- [Agent Client Protocol](https://agentclientprotocol.com/)
