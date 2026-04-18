# Jaeger AI Gateway

This package implements the AI gateway component within Jaeger Query that bridges
the Jaeger UI with an external AI Agent Sidecar using the
[Agent Client Protocol (ACP)](https://agentclientprotocol.com/) and the
[AG-UI protocol](https://docs.ag-ui.com/) for frontend streaming.

## Architecture

```mermaid
flowchart TB
    UI["Jaeger UI (Browser)"]

    subgraph jaeger["Jaeger Process"]
        direction LR
        MCP["MCP Server<br>:16687/mcp"]
        CTX["ContextualToolsStore"]
        subgraph handler["ChatHandler"]
            direction TB
            ACPCONN["acp.ClientSideConnection"]
            SC["streamingClient<br>(acp.Client callbacks)"]
            WS["WsReadWriteCloser<br>(WebSocket adapter)"]
            ACPCONN -- "callbacks" --> SC
            ACPCONN -- "transport" --> WS
        end
        handler -- "SetForSession()" --> CTX
        MCP -- "GetContextualToolsForSession()" --> CTX
    end

    subgraph sidecar["Agent Sidecar"]
        AGENT["ACP Agent Handler"]
        MCPCLIENT["MCP Client"]
        LLM["LLM API<br>(e.g. Gemini)"]
        AGENT --> MCPCLIENT
        AGENT --> LLM
    end

    UI -- "POST /api/ai/chat<br>(AG-UI RunAgentInput)" --> handler
    SC -- "SSE AG-UI events" --> UI
    WS -- "WebSocket (ACP)" --> AGENT
    MCPCLIENT -- "HTTP (MCP)" --> MCP
```

## Components

### ChatHandler (`handler.go`)

HTTP handler registered at `POST /api/ai/chat`. When a request arrives:

1. Parses the AG-UI `RunAgentInput` JSON request body
2. Establishes a WebSocket connection to the Agent Sidecar
3. Creates a `WsReadWriteCloser` to adapt WebSocket to `io.ReadWriteCloser`
4. Creates a `streamingClient` that implements `acp.Client` interface
5. Uses `acp.ClientSideConnection` to speak ACP protocol with the sidecar
6. Executes ACP handshake: `Initialize` -> `NewSession` -> `Prompt`
7. Publishes frontend-provided AG-UI tools to `ContextualToolsStore`
8. Streams responses back as AG-UI Server-Sent Events (SSE)

### streamingClient (`streaming_client.go`)

Implements the `acp.Client` interface from `acp-go-sdk`. Translates ACP session
updates into AG-UI SSE events:

- **SessionUpdate**: Receives streamed content from the agent (text chunks, tool
  call notifications) and emits corresponding AG-UI events:
  - `RUN_STARTED` / `RUN_FINISHED` - lifecycle events (include `threadId`)
  - `TEXT_MESSAGE_START` / `TEXT_MESSAGE_CONTENT` / `TEXT_MESSAGE_END` - text streaming
  - `TOOL_CALL_START` / `TOOL_CALL_ARGS` / `TOOL_CALL_RESULT` / `TOOL_CALL_END` - tool progress
  - `RUN_ERROR` - error reporting
- **RequestPermission**: Always cancels/denies permission requests (gateway
  advertises no filesystem or terminal capabilities)

### WsReadWriteCloser (`ws_adapter.go`)

Adapts a gorilla WebSocket connection to the `io.ReadWriteCloser` interface
required by `acp.ClientSideConnection`. Handles:

- Reading WebSocket text/binary messages as a continuous byte stream
- Writing bytes as WebSocket text messages
- Proper connection lifecycle management

### ContextualToolsStore (`contextual_tools.go`)

Thread-safe store for AG-UI tools that the frontend provides in each chat
request. The `ChatHandler` writes the per-turn tools snapshot keyed by ACP
session ID, and the MCP `list_contextual_tools` tool reads the snapshot for
the corresponding session. This allows the agent to discover frontend-provided
actions (e.g. visualization tools) without the frontend and MCP server needing
a direct connection.

## Request Flow

```mermaid
sequenceDiagram
    participant UI
    participant ChatHandler
    participant CTX as ContextualToolsStore
    participant acpConn as acp.ClientSideConnection
    participant SC as streamingClient
    participant Sidecar as Agent Sidecar
    participant LLM

    UI->>ChatHandler: POST /api/ai/chat (AG-UI RunAgentInput)
    ChatHandler->>Sidecar: WS connect
    ChatHandler->>acpConn: Initialize
    acpConn->>Sidecar: ACP: Initialize
    Sidecar-->>acpConn: InitializeResponse
    ChatHandler->>acpConn: NewSession
    acpConn->>Sidecar: ACP: NewSession
    Sidecar-->>acpConn: NewSessionResponse(sessionId)
    ChatHandler->>CTX: SetForSession(sessionId, tools)
    ChatHandler->>SC: startRun()
    SC-->>UI: SSE: RUN_STARTED {threadId, runId}
    ChatHandler->>acpConn: Prompt (blocks)
    acpConn->>Sidecar: ACP: Prompt

    Sidecar->>LLM: send_message
    LLM-->>Sidecar: function_call
    Note over Sidecar: executes MCP tool via Jaeger MCP
    Sidecar->>acpConn: SessionUpdate (tool_call)
    acpConn->>SC: SessionUpdate (tool_call)
    SC-->>UI: SSE: TOOL_CALL_START, TOOL_CALL_ARGS
    Sidecar->>acpConn: SessionUpdate (tool_result)
    acpConn->>SC: SessionUpdate (tool_result)
    SC-->>UI: SSE: TOOL_CALL_RESULT, TOOL_CALL_END
    Sidecar->>LLM: function_response
    LLM-->>Sidecar: final text

    Sidecar->>acpConn: SessionUpdate (text chunk)
    acpConn->>SC: SessionUpdate (text chunk)
    SC-->>UI: SSE: TEXT_MESSAGE_START, TEXT_MESSAGE_CONTENT

    Sidecar-->>acpConn: PromptResponse (StopReason)
    acpConn-->>ChatHandler: Prompt() returns
    ChatHandler->>SC: finishRun(stop_reason)
    SC-->>UI: SSE: TEXT_MESSAGE_END, RUN_FINISHED
    ChatHandler->>Sidecar: WS close
```

## Configuration

The AI gateway is configured via the `extensions.jaeger_query.ai` section:

```yaml
extensions:
  jaeger_query:
    ai:
      agent_url: "ws://localhost:16688"     # WebSocket URL of Agent Sidecar
      max_request_body_size: 1048576        # Max request body size in bytes
```

The endpoint is only registered when `ai.agent_url` is configured and non-empty.

## ACP Client Interface

The `acp.Client` interface has two types of methods:

**Request/Response methods** (agent calls client, blocks waiting for response):
- `ReadTextFile`, `WriteTextFile` - file operations (unsupported, returns error)
- `RequestPermission` - permission dialogs (always cancelled; gateway advertises no fs/terminal capabilities)
- `CreateTerminal`, `TerminalOutput`, etc. - terminal operations (unsupported, returns error)

**Notification method** (one-way, fire-and-forget):
- `SessionUpdate` - streams real-time progress and results during prompt processing

`SessionUpdate` carries only **informational** content for UI streaming:
- `AgentMessageChunk` - streamed text from the agent
- `AgentThoughtChunk` - agent's internal reasoning
- `ToolCall` / `ToolCallUpdate` - notifications that the agent has initiated/completed
  a tool call (for UI progress display, not execution requests)
- `Plan` - execution plan for complex tasks

In Jaeger's architecture, the sidecar executes MCP tools by calling Jaeger's MCP
server directly over HTTP. The `SessionUpdate(ToolCall)` notifications merely
inform the UI that a tool is running - they do not ask the client to execute
anything.

### End-of-Turn Handling

`Prompt()` blocks until the sidecar completes the ACP turn, including all tool
executions. When it returns, the `streamingClient` emits `TEXT_MESSAGE_END` (if
a text message was open) followed by `RUN_FINISHED` with the stop reason from
`PromptResponse.StopReason`, then the HTTP response is closed.

## Related Components

- **Agent Sidecar**: See `scripts/ai-sidecar/` for reference implementations
  (e.g., Gemini-based Python sidecar)
- **MCP Server**: Jaeger's MCP server exposes trace query tools at `/mcp`,
  including `list_contextual_tools` which reads from the `ContextualToolsStore`
- **ACP Protocol**: See https://agentclientprotocol.com/
- **AG-UI Protocol**: See https://docs.ag-ui.com/
