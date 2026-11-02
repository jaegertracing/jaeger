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
- The `goose` binary from [Goose releases](https://github.com/aaif-goose/goose/releases),
  v1.55.0 or later. That release includes
  [aaif-goose/goose#12697](https://github.com/aaif-goose/goose/pull/12697), which
  propagates the chat request's trace context into Goose's MCP tool calls so the
  agent's spans appear in the request trace.
- Credentials for a model provider. The zero-cost route is
  [OpenRouter](https://openrouter.ai)'s free tier — see below.

## 1. Configure the model provider

Goose reads these from its **environment**, not from `~/.config/goose/config.yaml`.
Setting the API key only in the config file leaves Goose reporting
`Provider not set`.

```bash
export GOOSE_PROVIDER=openrouter
export GOOSE_MODEL=nvidia/nemotron-3.5-lightning:free
export OPENROUTER_API_KEY=...
```

`run.sh` (see "Run the stack") can read the key from a secret store instead,
so it never sits in the shell's environment or history. Set
`SECRET_RESOLVER_CMD` to a command that prints the secret named by its last
argument; the script runs it with `OPENROUTER_API_KEY` appended when the
variable itself is not set:

```bash
# macOS Keychain: a generic password with service "api-tokens" and account "OPENROUTER_API_KEY"
export SECRET_RESOLVER_CMD="security find-generic-password -s api-tokens -w -a"
# Linux, pass: an entry named OPENROUTER_API_KEY
export SECRET_RESOLVER_CMD="pass show"
# Linux, libsecret: a secret with attributes service=api-tokens key=OPENROUTER_API_KEY
export SECRET_RESOLVER_CMD="secret-tool lookup service api-tokens key"
```

The model **must support tool calling**, or it answers from prior knowledge and
never queries telemetry. Free slugs come and go: a model that leaves the free
tier answers every prompt with a 404 that names the paid slug. To list the free
tool-calling models of the day:

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

## Run the stack

`docker-compose.yml` runs the setup above as containers, plus two things that
make it usable on its own: microsim, the trace generator of the SPM example,
sending a steady trickle of traces, and an Apache httpd reverse proxy between
Goose and Jaeger's MCP endpoint whose access log prints the headers of every
proxied request. `run.sh` starts the stack and leaves it up, so the assistant
can be tried in the Jaeger UI at <http://localhost:16686>, where the AI panel
appears once Jaeger's health probe reaches Goose. Chatting needs a model
provider: set `OPENROUTER_API_KEY`, or `SECRET_RESOLVER_CMD` to fetch it, as
in step 1 before starting.

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
are per turn). Three answers are expected and depend on no header. Goose first
sends `server/discover`, a request from a newer MCP revision than the Go SDK
implements, which Jaeger answers 400 with `"server/discover" unsupported`, and
Goose then proceeds to `initialize`. The `GET` that would open a standing event
stream is answered 405, because the turn-scoped endpoint keeps no session, and
Goose continues without it. A `DELETE` after the turn is answered 404 because
the turn-scoped route is gone by then.

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
credentials. Those extensions also show up in answers: with them on, a question
about services may come back with a clock widget or a delegated "developer"
call beside the Jaeger tools. Goose has no switch that turns them all off, only a
per-extension `enabled: false` in its config file, so the image in
`docker-compose.yml` generates that file at build time from the extension list
its own `goose info -v` prints, every entry disabled; the agent then has only
the tools Jaeger announces for the turn, and a new Goose version brings its own
list. Running Goose by hand, as in step 2, uses whatever
`~/.config/goose/config.yaml` enables; `goose info -v` lists the extensions and
their state.
