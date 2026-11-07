# Goose

[Goose](https://github.com/aaif-goose/goose) is a reference implementation of
an ACP agent, and this directory holds what is needed to run it as the agent
behind Jaeger's AI gateway. The steps to configure Goose, point Jaeger at it,
and verify the result are on the website on the
[AI Assistant page](https://www.jaegertracing.io/docs/latest/ai/assistant/).

## Run the stack

`docker-compose.yml` runs that setup as containers, plus two things that make
it usable on its own: microsim, the trace generator of the SPM example, sending
a steady trickle of traces, and an Apache httpd reverse proxy between Goose and
Jaeger's MCP endpoint whose access log prints the headers of every proxied
request. `run.sh` starts the stack and leaves it up, so the assistant can be
tried in the Jaeger UI at <http://localhost:16686>, where the AI panel appears
once Jaeger's health probe reaches Goose. Chatting needs a model provider: set
`OPENROUTER_API_KEY` before starting, or set `SECRET_RESOLVER_CMD` to a command
that prints it, as the script's header comment describes.

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
are per turn). Goose and Jaeger speak MCP revision 2026-07-28, which has no
`initialize` handshake and no session, so every request is a `POST` that
carries the protocol version in its `MCP-Protocol-Version` header and no
`Mcp-Session-Id`. The three requests of a turn are `server/discover`,
`tools/list`, and the `tools/call` the model asks for; without a model
provider the third is missing.

```text
POST /api/ai/mcp/<route>/ -> 200 turn="<token>" user="alice" auth="Bearer user-token" unlisted="-" accept="text/event-stream, application/json" proto="2026-07-28" sid="-"
POST /api/ai/mcp/<route>/ -> 200 turn="<token>" user="alice" auth="Bearer user-token" unlisted="-" accept="text/event-stream, application/json" proto="2026-07-28" sid="-"
POST /api/ai/mcp/<route>/ -> 200 turn="<token>" user="alice" auth="Bearer user-token" unlisted="-" accept="text/event-stream, application/json" proto="2026-07-28" sid="-"
```

## Reading a failure of the stack

The website's troubleshooting section covers the agent and the gateway. The
stack adds two hops of its own, the model provider and the proxied MCP
endpoint, and each failure shows up in a different place:

| Hop | What it looks like | Cause |
| --- | --- | --- |
| Goose → model provider | A `RUN_ERROR` event `The agent failed the turn (ACP session/prompt): {"code":-32000,"message":"Authentication required",...}` | Goose has no usable credential for its provider: `OPENROUTER_API_KEY` is not in Goose's environment |
| Goose → model provider | The assistant answers with text such as `Authentication error: Authentication failed for https://openrouter.ai/... Status: 401` | The key reached Goose and the provider rejected it, or it is empty. Goose reports the provider's answer as its reply |
| Goose → model provider | Every prompt fails with a 404 that names a paid model slug | OpenRouter's free tier changed and the default `GOOSE_MODEL` is no longer free. Set `GOOSE_MODEL` to another free tool-calling model; the website shows how to list them |
| Goose → Jaeger MCP endpoint | `401` lines in the proxy log, or `user="-"` on them while the chat request carried a user | Goose did not send the announced headers back, which the agent contract on the website requires |

## Agent-supplied tools

Goose ships general-purpose extensions, including shell access, and with them
on a question about services may come back with a clock widget or a delegated
"developer" call beside the Jaeger tools. Goose has no switch that turns them
all off, only a per-extension `enabled: false` in its config file, so the image
in `docker-compose.yml` generates that file at build time from the extension
list its own `goose info -v` prints, every entry disabled; the agent then has
only the tools Jaeger announces for the turn, and a new Goose version brings its
own list. Goose started by hand uses whatever `~/.config/goose/config.yaml`
enables; `goose info -v` lists the extensions and their state.
