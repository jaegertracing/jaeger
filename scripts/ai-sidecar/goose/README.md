# Goose

[Goose](https://github.com/aaif-goose/goose) is a reference implementation of
an ACP agent, and this directory holds what is needed to run it as the agent
behind Jaeger's AI gateway. The steps to configure Goose, point Jaeger at it,
and verify the result are on the website on the
[AI Assistant page](https://www.jaegertracing.io/docs/latest/ai/assistant/).

## Run the stack

`docker-compose.yml` runs the setup above as containers, plus two things that
make it usable on its own: microsim, the trace generator of the SPM example,
sending a steady trickle of traces, and an Apache httpd reverse proxy between
Goose and Jaeger's MCP endpoint whose access log prints the headers of every
proxied request. `run.sh` starts the stack and leaves it up, so the assistant
can be tried in the Jaeger UI at <http://localhost:16686>, where the AI panel
appears once Jaeger's health probe reaches Goose. Chatting needs a model
provider: set `OPENROUTER_API_KEY` as in step 1 before starting.

```bash
./run.sh              # a Linux build of this working tree, inside the released image
./run.sh --release    # the released Jaeger image as is
./run.sh --down       # tear the stack down after the check, for CI
```

Before it hands the stack over, the script sends one chat request that carries
a caller identity and checks the proxy log: every MCP request Goose made
carries the turn token and the caller's identity headers, and none carries a
header the gateway was not told to forward. Goose lists the MCP server's tools
before it calls the model, so the check passes without a model provider; with
one, the agent also calls a tool.

The proxy logs the credentials on purpose, so the check can read them; a
deployment's proxy must not. The log looks like this (the token and route id
are per turn). Three answers are expected and depend on no header: Goose sends
one POST before `initialize` that Jaeger answers 400; the `GET` that would open
a standing event stream is answered 405, because the turn-scoped endpoint keeps
no session, and Goose continues without it; a `DELETE` after the turn is
answered 404 because the turn-scoped route is gone by then.

```text
POST /api/ai/mcp/<route>/ -> 400 turn=<token> user=alice auth=Bearer user-token unlisted=- ...
POST /api/ai/mcp/<route>/ -> 200 turn=<token> user=alice auth=Bearer user-token unlisted=- ...
GET /api/ai/mcp/<route>/ -> 405 turn=<token> user=alice auth=Bearer user-token unlisted=- ...
POST /api/ai/mcp/<route>/ -> 200 turn=<token> user=alice auth=Bearer user-token unlisted=- ...
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
