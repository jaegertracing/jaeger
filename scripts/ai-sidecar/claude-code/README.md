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

## Prerequisites

- Node 20+.
- An Anthropic credential (see [Quick Start](#quick-start)).

## Quick Start

The simplest way to start the sidecar alongside a local Jaeger instance is using the top-level Makefile target. It automatically ensures dependencies are installed, checks for valid authentication, and launches both Jaeger and the bridge with interleaved, color-coded logs.

```bash
# In the repository root
make run-ai-claude ARGS="--mcp-server jaeger=http://127.0.0.1:16687/mcp"
```

> **Note:** The `--mcp-server` flag is what gives Claude access to Jaeger's
> tracing tools. Without it the bridge runs but the agent has no tools to
> query traces. See [MCP Servers](#mcp-servers) for details.

### Authentication
The launcher auto-detects two types of credentials:
1.  **API Key**: Set `ANTHROPIC_API_KEY=sk-...` in your environment.
2.  **Claude CLI**: If no API key is found, it checks for an existing session from `claude-agent-acp`. If you haven't logged in yet, run:
    ```bash
    cd scripts/ai-sidecar/claude-code && npm run auth:max
    ```

## Development

If you are developing features in a local checkout of the `claude-agent-acp` repository and want to verify them with this bridge:

1.  **In your local `claude-agent-acp` directory:**
    ```bash
    npm link
    ```
2.  **In this `scripts/ai-sidecar/claude-code` directory:**
    ```bash
    npm link @agentclientprotocol/claude-agent-acp
    ```

This configuration instructs Node.js to use your local development version of the agent, ensuring that any changes you make to the agent are immediately reflected when you start the bridge.

## Configuration

Optional environment variables:

| Var            | Default     | Purpose                                                                                                 |
| -------------- | ----------- | ------------------------------------------------------------------------------------------------------- |
| `HOST`         | `127.0.0.1` | Bind address. Non-loopback values require `ALLOW_REMOTE=1`.                                             |
| `PORT`         | `16688`     | Bind port.                                                                                              |
| `ALLOW_REMOTE` | unset       | Required to bind to a non-loopback `HOST`. The bridge has no auth; setting this opts into the exposure. |
| `DEBUG_BRIDGE` | unset       | Log every JSON-RPC frame in both directions.                                                            |

### MCP Servers

To give Claude access to Jaeger's tools, pass the `--mcp-server` flag via the `ARGS` variable:

```bash
# Via Makefile
make run-ai-claude ARGS="--mcp-server jaeger=http://127.0.0.1:16687/mcp"

# Or directly
./run.sh --mcp-server jaeger=http://127.0.0.1:16687/mcp
```

The flag is repeatable — pass multiple `--mcp-server` entries to connect additional MCP servers.

## Files

| File                   | Purpose                                                                |
| ---------------------- | ---------------------------------------------------------------------- |
| `run.sh`               | Standardized launcher: bootstraps Node and orchestrates Jaeger/Bridge. |
| `preflight.sh`         | Auth validation script called by the launcher.                         |
| `jaeger-ws-bridge.mjs` | Main entry point: handles the WS server and agent subprocesses.        |
| `config.mjs`           | Configuration management and security validation.                      |
| `utils.mjs`            | Payload normalization and JSON-RPC message injection (MCP).            |
| `logger.mjs`           | Structured logging (INFO, WARN, ERROR, DEBUG) with timestamps.         |
| `package.json`         | Declares dependencies (`claude-agent-acp`, `ws`).                      |
| `README.md`            | This file.                                                             |
