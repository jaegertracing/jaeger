# Goose (ACP Agent)

Jaeger's AI gateway is bring-your-own-agent: it dials an
[ACP](https://agentclientprotocol.com/) agent over WebSocket and speaks the
protocol to it. No agent code lives here.

[Goose](https://github.com/aaif-goose/goose) is used below as a worked example
because it serves ACP over WebSocket natively, so no bridge or wrapper is
needed. **Any agent that serves ACP over WebSocket works the same way** — only
`agent_url`, the auth header name, and the agent's own launch command change.

## Prerequisites

- A Jaeger build that includes [#9296](https://github.com/jaegertracing/jaeger/pull/9296)
  (WebSocket message framing) and [#9395](https://github.com/jaegertracing/jaeger/pull/9395)
  (`ai.agent_headers`). Both are on `main`.
- The `goose` binary from [Goose releases](https://github.com/aaif-goose/goose/releases).
- Credentials for a model provider. The zero-cost route is
  [OpenRouter](https://openrouter.ai)'s free tier — see below.

## 1. Configure the model provider

Goose reads these from its **environment**, not from `~/.config/goose/config.yaml`.
Setting the API key only in the config file leaves Goose reporting
`Provider not set`.

```bash
export GOOSE_PROVIDER=openrouter
export GOOSE_MODEL=nvidia/nemotron-3-super-120b-a12b:free
export OPENROUTER_API_KEY=...
```

The model **must support tool calling**, or it answers from prior knowledge and
never queries telemetry. To list free tool-calling models:

```bash
curl -s https://openrouter.ai/api/v1/models \
  -H "Authorization: Bearer $OPENROUTER_API_KEY" |
  jq -r '.data[] | select(.supported_parameters | index("tools"))
         | select((.pricing.prompt|tonumber) == 0) | .id'
```

## 2. Start Goose with authentication on

```bash
export GOOSE_SERVER__SECRET_KEY=$(openssl rand -hex 16)
goose serve --host 127.0.0.1 --port 16688
```

Goose then requires that secret as an `X-Secret-Key` header on the WebSocket
upgrade, and serves ACP at the **`/acp`** path. Its default port is 3284; the
`--port` above matches Jaeger's default `agent_url` port instead.

Goose also accepts `--dangerously-unauthenticated`, which disables this check.
It is not used here and should not be used with a Jaeger that can send headers.

## 3. Point Jaeger at it

In the `jaeger_query` extension config:

```yaml
    ai:
      agent_url: ws://127.0.0.1:16688/acp
      agent_headers:
        X-Secret-Key: ${env:GOOSE_SERVER__SECRET_KEY}
      # Present enables the telemetry MCP endpoint the agent's tools come from.
      mcp: {}
```

`agent_headers` values are `configopaque` strings, so they stay out of logs and
config dumps. Prefer `${env:VAR}` over an inline secret. Start Jaeger with
`GOOSE_SERVER__SECRET_KEY` set in its environment too, so the expansion resolves.

The header name belongs to the agent, not to Jaeger: Goose wants
`X-Secret-Key`, another agent may want `Authorization`. Set whatever it expects.

## 4. Verify

Open the Jaeger UI. If the gateway's health check reached the agent, the AI
assistant panel is available; if not, it is hidden. Ask it something that needs
telemetry, for example *"list the services you can see"*, and the streamed tool
calls should be prefixed `jaeger:` — that prefix means they came from Jaeger's
MCP endpoint rather than from a tool the agent brought itself.

## Troubleshooting

**Assistant panel missing / `"aiAssistant":false` in the page source.** The
health check cannot reach the agent. It dials separately from the chat endpoint
and sends the same headers, so a missing or wrong `agent_headers` value hides
the panel even though nothing else looks broken. Check the gateway log for
`AI health check failed`.

**`401 Unauthorized` on dial.** The header is missing, misnamed, or its value
does not match `GOOSE_SERVER__SECRET_KEY`. Verify the variable is exported in
*both* processes: Goose reads it, and Jaeger expands it.

**Connection hangs, then the health check times out.** The build predates
[#9296](https://github.com/jaegertracing/jaeger/pull/9296). ACP sends one
JSON-RPC object per WebSocket message with no trailing newline, which older
gateways waited for forever. Update Jaeger.

**`404 Not Found` on dial.** The path is wrong — Goose serves ACP at `/acp`, and
the bare host and port returns 404.

**Agent answers without any `jaeger:` tool calls.** It is using its own tools
instead of Jaeger's. Confirm the `mcp:` block is present; without it the gateway
announces no MCP endpoint and Goose falls back to its built-in shell extension,
which can still produce a plausible-looking answer.

## Note on agent-supplied tools

Goose ships general-purpose extensions, including shell access, and runs its own
agent loop. Anything it can reach on the host is reachable from a chat turn, so
run it with the same care as any other process holding your provider
credentials.
