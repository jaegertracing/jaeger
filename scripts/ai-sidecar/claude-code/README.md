# Claude Code Sidecar (ACP Agent)

This folder is a thin **WebSocket → stdio bridge** that lets Jaeger's AI
gateway use [`claude-agent-acp`](https://github.com/agentclientprotocol/claude-agent-acp)
(the Claude Agent SDK's ACP adapter) as its sidecar.

The bridge is intentionally minimal:

- Listens on `ws://127.0.0.1:16688` by default (matches `extensions.jaeger_query.ai.agent_url`).
- For each incoming WS connection, spawns one `claude-agent-acp` subprocess.
- Shuttles newline-delimited JSON-RPC frames between the WS and the child's
  stdin/stdout.
- Tears the child down when the connection closes.

The agent itself is `claude-agent-acp`; the bridge does no LLM or ACP work
beyond byte shuttling. See [the parent README](../README.md) for a
walk-through of the protocol and what a from-scratch sidecar implements.

## Prerequisites

- Node 20+.
- An Anthropic credential the Claude Agent SDK can use. Two paths:
  - **Claude Max subscription / claude.ai login** — interactive browser
    flow handled by the agent's bundled Claude CLI. See
    [Authenticate](#authenticate) below. No env var needed.
  - **API key** — set `ANTHROPIC_API_KEY=sk-…` in the bridge's
    environment. Skip the auth step.

## Install

```bash
cd scripts/ai-sidecar/claude-code
npm install
```

If you're hacking on a local checkout of `claude-agent-acp`, point at it
with `npm link` from the agent repo, then `npm link
@agentclientprotocol/claude-agent-acp` here.

## Authenticate

Only if you're using Max / claude.ai login — skip if you set
`ANTHROPIC_API_KEY` instead.

```bash
npm run auth:max
```

That just invokes `claude-agent-acp --cli auth login --claudeai`, which
runs the agent's bundled Claude CLI in auth mode. Follow the browser flow
it opens; credentials persist to the standard Claude CLI config
(`~/.claude/…`), so you only need to do this once per machine. Subsequent
`npm start` runs pick them up automatically — no env var required.

Check the current auth state any time:

```bash
npm run auth:status
```

## Run

```bash
# Max-subscription auth: just start it
npm start

# API-key auth: set the var in the same shell
ANTHROPIC_API_KEY=sk-…  npm start

# [bridge] listening on ws://127.0.0.1:16688
# [bridge] agent entry: …/dist/index.js
```

Optional env knobs:

| Var    | Default     | Purpose      |
| ------ | ----------- | ------------ |
| `HOST` | `127.0.0.1` | Bind address |
| `PORT` | `16688`     | Bind port    |

Then start Jaeger pointing at the same URL:

```yaml
# config.yaml
extensions:
  jaeger_query:
    ai:
      agent_url: "ws://127.0.0.1:16688"
```

## How Jaeger MCP gets to Claude

The gateway forwards whatever the operator configures under
`extensions.jaeger_query.ai.mcp_servers` to the agent via
`NewSessionRequest.mcpServers` on every session/new. The Claude Agent SDK
consumes those entries automatically and exposes their tools to the model
as `mcp__<name>__<tool>`. No MCP wiring is needed in this bridge.

Minimal config for plugging Claude into Jaeger's built-in MCP server:

```yaml
extensions:
  jaeger_query:
    ai:
      agent_url: ws://127.0.0.1:16688
      mcp_servers:
        - name: jaeger
          url: http://127.0.0.1:16687/mcp
```

> Only HTTP transport is exposed today. Stdio-only agents ignore the
> entry; `claude-agent-acp` advertises HTTP MCP capability and honors it.
> Agents that have their own MCP config (e.g. the Gemini sidecar reading
> `JAEGER_MCP_URL`) silently drop wire-pushed entries — both paths stay
> valid.

## Known limitations

- **Contextual / UI tools are not dispatched.** Tools the frontend declares
  on the AG-UI request arrive in `NewSessionRequest._meta` under the
  `jaegertracing.io/contextual-tools` key. `claude-agent-acp` does not
  consume that key today, so the bridge passes the request through as-is
  and the tools are silently dropped. UI features like `ui_highlight_span`
  therefore don't work via this sidecar yet. The Gemini reference
  implementation in [`../gemini/`](../gemini/) does handle them — track
  upstream support in `claude-agent-acp` for parity.
- **No streaming-of-streams optimisation.** Each WS frame is one ACP
  message; the bridge does no buffering or coalescing. Fine for chat
  traffic; not designed for high-throughput protocols.
- **Per-request subprocess cost.** Each new chat spawns a fresh Node
  process (~200–400 ms cold start). Trivial next to LLM latency; if it
  ever isn't, an in-process embed is straightforward to write.

## How it differs from the Gemini sidecar

|                    | `gemini/`                                  | `claude-code/`                                  |
| ------------------ | ------------------------------------------ | ----------------------------------------------- |
| Language           | Python                                     | Node (just the bridge)                          |
| Agent              | In-process, custom                         | External process: `claude-agent-acp`            |
| MCP                | Built-in bridge reads `JAEGER_MCP_URL` env | Gateway pushes via `mcpServers`; SDK handles it |
| Contextual tools   | Supported via ACP ext_method               | Not yet (dropped silently)                      |
| LOC in this folder | ~600 (full agent)                          | ~100 (just the bridge)                          |

## Files

| File                   | Purpose                                                   |
| ---------------------- | --------------------------------------------------------- |
| `package.json`         | Declares deps (`claude-agent-acp`, `ws`) and `npm start`. |
| `jaeger-ws-bridge.mjs` | The WS server + per-connection subprocess shuttle.        |
| `README.md`            | This file.                                                |
