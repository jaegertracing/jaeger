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

`uv sync` is the only supported dependency install method for this sidecar.

## Run The Sidecar Server

You can start the same server using either entrypoint:

```bash
uv run python main.py
```

Expected startup log:

```text
Jaeger ACP Sidecar listening on ws://localhost:9000
```

## Architecture

```mermaid
sequenceDiagram
    participant GW as Gateway
    participant HW as handle_websocket
    participant ACP as ACP run_agent
    participant AG as JaegerSidecarAgent
    participant GL as gemini_loop
    participant MCP as JaegerMCPBridge
    participant JMCP as Jaeger MCP
    participant GEM as Gemini

    GW->>HW: WebSocket connect
    HW->>ACP: forward incoming ACP messages
    ACP->>AG: initialize/new_session/prompt
    AG->>GL: run loop

    GL->>MCP: get_gemini_tools()
    MCP->>JMCP: discover tools
    JMCP-->>MCP: tool metadata
    MCP-->>GL: declarations

    GL->>GEM: send user prompt
    GEM-->>GL: function_calls or final text

    loop For each function call
        GL->>MCP: call_tool(name,args)
        MCP->>JMCP: execute tool
        JMCP-->>MCP: tool output
        MCP-->>GL: tool result
        GL->>GEM: send function response
        GEM-->>GL: next function_calls or final text
    end

    GL-->>AG: final text
    AG-->>ACP: session_update + end_turn
    ACP-->>GW: streamed updates + response

    GW->>HW: close
    HW->>HW: cancel tasks + close streams/sockets
```


## End-to-End Test

1. Start Jaeger CMD in another terminal.
2. Start this sidecar.
3. Run the pytest workflow test, which monkeypatches the agent and drives the ACP prompt flow end to end:

```bash
uv run pytest -q test_sidecar_workflow.py
```

The test connects to the sidecar over WebSocket, sends `initialize`, `session/new`, and `session/prompt`, and verifies the streamed ACP updates and end-of-turn marker.