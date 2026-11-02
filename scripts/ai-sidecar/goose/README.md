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
./run.sh --release    # the released Jaeger image as is; the check passes once a release carries the identity headers
./run.sh --ci         # start, check, tear down, and exit with the check's result
./run.sh --down       # stop the stack
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

**An authentication error.** A chat turn crosses four authenticated hops, and
each fails with a different message. The message says which hop failed:

| Hop | What it looks like | Cause |
| --- | --- | --- |
| Browser → Jaeger `/api/ai/chat` | The request fails with an HTTP 401 or 403 before any chat event arrives | The proxy in front of Jaeger, or `ai.auth`, rejected the user |
| Jaeger → Goose (WebSocket dial) | `401 Unauthorized` on dial in the gateway log; the panel is missing | The `agent_headers` value is missing, misnamed, or does not match `GOOSE_SERVER__SECRET_KEY`. Verify the variable is exported in *both* processes: Goose reads it, and Jaeger expands it |
| Goose → model provider | A `RUN_ERROR` event `The agent failed the turn (ACP session/prompt): {"code":-32000,"message":"Authentication required",...}` | Goose has no usable credential for its provider: `OPENROUTER_API_KEY` is not in Goose's environment |
| Goose → model provider | The assistant answers with text such as `Authentication error: Authentication failed for https://openrouter.ai/... Status: 401` | The key reached Goose and the provider rejected it, or it is empty. Goose reports the provider's answer as its reply |
| Goose → Jaeger MCP endpoint | `401` lines in the proxy log, or `user=-` on them while the chat request carried a user | Goose did not send the announced headers back; see the agent contract in [../README.md](../README.md) |

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
