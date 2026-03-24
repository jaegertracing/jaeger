# Python Sidecar (ACP Agent)

This folder contains the Python ACP sidecar used by the Jaeger AI gateway.

The sidecar:
- Listens on `ws://localhost:9000`
- Runs a Gemini-backed ACP agent
- Uses Jaeger MCP tools from `http://127.0.0.1:16687/mcp`

## Prerequisites

- Python 3.14+
- `uv` installed
- A Gemini API key

## Required Environment Variable

Set your Gemini API key before starting the server:

```bash
export GEMINI_API_KEY="your_api_key_here"
```

Without this key, the sidecar cannot create the Gemini client.

Optional MCP endpoint override:

```bash
export JAEGER_MCP_URL="http://127.0.0.1:16687/mcp"
```

If unset, the sidecar defaults to `http://127.0.0.1:16687/mcp`.

## Install Dependencies

From this directory:

```bash
uv sync
```

## Run The Sidecar Server

You can start the same server using either entrypoint:

```bash
uv run python main.py
```

Expected startup log:

```text
Jaeger ACP Sidecar listening on ws://localhost:9000
```

## End-to-End Test

1. Start Jaeger CMD in another terminal.
2. Start this sidecar.
3. Send a chat prompt:

```bash
curl -N -X POST http://localhost:16686/api/ai/chat \
	-H "Content-Type: application/json" \
	-d '{"prompt":"can you search for trace with id 1"}'
```

You should see streamed output and tool lifecycle updates (`tool_call` / `tool_result`) when Gemini decides to use tools.
