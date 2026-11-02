# Goose

[Goose](https://github.com/aaif-goose/goose) is a reference implementation of
an ACP agent, and this directory holds what is needed to run it as the agent
behind Jaeger's AI gateway. The steps to configure Goose, point Jaeger at it,
and verify the result are on the website on the
[AI Assistant page](https://www.jaegertracing.io/docs/latest/ai/assistant/).

## Smoke test

`docker-compose.yml` runs the setup above as containers, with an Apache httpd
reverse proxy between Goose and Jaeger's MCP endpoint whose access log prints
the headers of every proxied request. `smoke.sh` starts the stack, sends one
chat request that carries a caller identity, and checks the log: every MCP
request Goose made carries the turn token and the caller's identity headers,
and none carries a header the gateway was not told to forward. Goose lists the
MCP server's tools before it calls the model, so the checks run without a model
provider; with `OPENROUTER_API_KEY` set, the agent also calls a tool.

```bash
./smoke.sh                                   # released Jaeger image
CGO_ENABLED=0 GOOS=linux go build -o /tmp/jaeger-linux ../../../cmd/jaeger
JAEGER_BINARY=/tmp/jaeger-linux ./smoke.sh -f docker-compose.local.yml   # this working tree
```

The proxy logs the credentials on purpose, so the test can check them; a
deployment's proxy must not. The log looks like this (the token and route id
are per turn). Goose
sends one POST before `initialize` that Jaeger answers 400, and its `DELETE`
after the turn is answered 404 because the turn-scoped route is gone by then;
both are expected.

```text
POST /api/ai/mcp/<route>/ -> 200 turn=<token> user=alice auth=Bearer user-token unlisted=- ...
GET /api/ai/mcp/<route>/ -> 200 turn=<token> user=alice auth=Bearer user-token unlisted=- ...
DELETE /api/ai/mcp/<route>/ -> 404 turn=<token> user=alice auth=Bearer user-token unlisted=- ...
```

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
