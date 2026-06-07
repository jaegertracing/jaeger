# Build Your Own Jaeger AI Sidecar

So you want to plug a different LLM into Jaeger's chat experience. Good news:
the gateway doesn't care which model is on the other end. It just speaks
[ACP](https://agentclientprotocol.com/) over a WebSocket, and whatever process
answers is the sidecar.

This guide walks through how to build that process. There's a working
reference implementation in [`gemini/`](./gemini) — most readers will want to
fork it. If you'd rather build from scratch in another language, there's a
checklist for that too.

> Already know the contract and want the deep-dive? See
> [`docs/rfc/0002-ai-gateway-contextual-tools.md`](../../docs/rfc/0002-ai-gateway-contextual-tools.md)
> and the [gateway README](../../cmd/jaeger/internal/extension/jaegerquery/internal/jaegerai/README.md).

## Pick Your Path

| Your situation                                                  | Go to                                |
| --------------------------------------------------------------- | ------------------------------------ |
| "I want to use OpenAI / Anthropic / Ollama / my model instead." | [Path A](#path-a-swap-the-llm)       |
| "I'm writing a sidecar in Go / Rust / Node / something else."   | [Path B](#path-b-build-from-scratch) |

Either way, [Verify It Works](#verify-it-works) at the end is the same.

---

## Path A: Swap the LLM

Fork [`gemini/`](./gemini) and replace four things. Everything else — the
WebSocket server, the ACP handlers, `_meta` parsing, the MCP bridge, the
contextual-tool dispatch — already works.

### Step 1 — Copy and rename

```bash
cp -r scripts/ai-sidecar/gemini scripts/ai-sidecar/myprovider
cd scripts/ai-sidecar/myprovider
```

Update `pyproject.toml`: swap `google-genai` for your provider's SDK, rename
the package, bump the service name in `tracing.py` if you want it to show up
under a different name in Jaeger.

### Step 2 — Replace the LLM client

In [`sidecar.py`](./gemini/sidecar.py), the Gemini client is built once at
agent construction:

```python
# sidecar.py around line 67
self._gemini = genai.Client(api_key=config.gemini_api_key)
```

Swap that for your provider's client. Then update
[`sidecar_config.py`](./gemini/sidecar_config.py) so the env var matches what
your provider expects (e.g. `OPENAI_API_KEY` instead of `GEMINI_API_KEY`).

### Step 3 — Replace the agentic loop

This is the heart of it. In `sidecar.py`, `_run_agentic_gemini_loop`
(around line 279) does the model→tool→model dance:

```python
# Build the tool list — MCP tools + the contextual tools the gateway
# attached on session/new (already prefixed with `ui_`):
mcp_tools = await self._mcp.get_gemini_tools()
contextual_tools = self._contextual_tools.get(session_id, [])
contextual_tool_names = {t["name"] for t in contextual_tools if t.get("name")}
tools_for_llm = merge(mcp_tools, _build_gemini_contextual_tool(contextual_tools))

# Open a chat, send the user message:
chat = self._gemini.chats.create(model=..., tools=tools_for_llm, ...)
response = await asyncio.to_thread(chat.send_message, user_text)

# Loop until the model stops calling tools:
while response.function_calls:
    function_responses = []
    for fc in response.function_calls:
        if fc.name in contextual_tool_names:
            # Route to the gateway via ACP extension method
            result = await self._execute_contextual_tool(...)
        else:
            # Route to the Jaeger MCP server
            result = await self._execute_tool(...)
        function_responses.append(...)
    response = await asyncio.to_thread(chat.send_message, function_responses)

return response.text or ""
```

Replace the body with your provider's equivalent. The only thing you must
preserve is **the routing decision**: if the model picks a name that's in
`contextual_tool_names`, dispatch through `_execute_contextual_tool` (which
sends the ACP extension method back to the gateway); otherwise use
`_execute_tool` (which calls the Jaeger MCP server).

> **Gotcha — don't reformat the tool names.** The names in the contextual
> snapshot already start with `ui_`. Pass them to your LLM exactly as you
> received them, and route on the exact string the model gives back. The
> gateway strips the prefix on its side.

### Step 4 — Translate tool schemas

Each LLM provider uses its own shape for function declarations. The Gemini
shape lives in [`sidecar_helpers.py`](./gemini/sidecar_helpers.py):

- `_build_gemini_contextual_tool` — turns the JSON snapshot into a Gemini
  `types.Tool`.
- `_extract_function_declaration` — extracts a single Gemini-shaped
  declaration.
- `JaegerMCPBridge.get_gemini_tools` (in [`mcp_bridge.py`](./gemini/mcp_bridge.py))
  — turns MCP tool metadata into Gemini `types.Tool` instances.

Rewrite these two helpers in your provider's shape (e.g. OpenAI `tools=[{type:
"function", function: {...}}]` or Anthropic `tools=[{name, description,
input_schema}]`). The JSON schema in the contextual snapshot is plain JSON
Schema, so the conversion is usually a thin wrapper.

That's it. Skip to [Verify It Works](#verify-it-works).

---

## Path B: Build From Scratch

Building a sidecar in another language is roughly eight steps. Each one
points at the matching file in the Gemini reference so you can copy the
behavior even if you can't copy the code.

### 1. Stand up a WebSocket server

Listen on a host/port that matches what the operator will configure for
`extensions.jaeger_query.ai.agent_url` (e.g. `ws://localhost:16688`). Each
incoming connection handles one ACP session and closes when the prompt
completes.

> Reference: [`gemini/main.py`](./gemini/main.py),
> [`gemini/sidecar.py:handle_websocket`](./gemini/sidecar.py).

### 2. Speak ACP JSON-RPC over the socket

Use an off-the-shelf ACP SDK if one exists for your language; otherwise
implement the JSON-RPC framing yourself (one JSON message per WebSocket text
frame works — see how the gateway side does it in
[`ws_adapter.go`](../../cmd/jaeger/internal/extension/jaegerquery/internal/jaegerai/ws_adapter.go)).

You must handle three inbound methods:

- `initialize` — return your protocol version and capabilities. The gateway
  declares no fs/terminal capabilities, so don't depend on them.
- `session/new` — allocate a session id and return it. **This is where the
  `_meta` snapshot arrives — see step 4.**
- `session/prompt` — run a turn. See steps 5–8.

> Reference: `initialize`, `new_session`, and `prompt` in
> [`gemini/sidecar.py`](./gemini/sidecar.py).

> **Gotcha — permission requests will be denied.** The gateway always denies
> `session/request_permission`. Don't bother asking.

### 3. Discover and call Jaeger MCP tools

The sidecar talks to Jaeger's MCP server directly over HTTP (default
`http://127.0.0.1:16687/mcp`). Use any MCP client library, call `tools/list`
once per session, and call `tools/call` when the LLM picks one of those
names.

These calls do **not** go through the gateway.

> Reference: [`gemini/mcp_bridge.py`](./gemini/mcp_bridge.py).

### 4. Parse the contextual-tools snapshot from `_meta`

This is the most important step and the one nothing else in ACP tells you
to do. On every `session/new`, look at the `_meta` field on the request.
If it contains the key `jaegertracing.io/contextual-tools`, the value is the
list of per-turn UI tools the gateway wants to register:

```jsonc
{
  "_meta": {
    "jaegertracing.io/contextual-tools": {
      "tools": [
        {
          "name": "ui_show_flamegraph",
          "description": "Open the flamegraph view for a given trace_id.",
          "parameters": { "type": "object", "properties": { ... } }
        }
      ]
    }
  }
}
```

Store this list **keyed by the session id you just allocated**. You'll need
it again in `session/prompt` and you must drop it when the prompt ends.

> Reference: `_extract_contextual_tools` in
> [`gemini/sidecar_helpers.py`](./gemini/sidecar_helpers.py) and
> `new_session` in [`gemini/sidecar.py`](./gemini/sidecar.py).

> **Gotcha — the names are already prefixed.** Every contextual tool name
> starts with `ui_`. That's deliberate: it prevents UI tools from shadowing
> built-in MCP tools. Pass the prefixed name to your LLM unchanged; the
> gateway strips the prefix on the way back.

### 5. Merge MCP and contextual tools, hand them to the LLM

When `session/prompt` arrives, build your LLM tool list by combining the MCP
tools (from step 3) with the contextual tools (from step 4). Translate both
into whatever shape your LLM expects.

Keep a `set` of contextual tool names handy — you need it in step 7 to know
how to route each function call.

### 6. Stream progress via `session/update`

While the LLM is thinking and calling tools, emit ACP `session/update`
notifications. The gateway forwards these to the browser as AG-UI SSE
events:

| Your `session/update`          | What the browser sees              |
| ------------------------------ | ---------------------------------- |
| `AgentMessageChunk(text)`      | `TEXT_MESSAGE_CONTENT`             |
| `start_tool_call(...)`         | `TOOL_CALL_START` (+ `ARGS`)       |
| `update_tool_call(...)`        | `TOOL_CALL_ARGS` / `RESULT` / `END` |

Wrap every tool call — MCP or contextual — with `start_tool_call` then
`update_tool_call` so the UI renders progress consistently.

> Reference: `_execute_tool` and `_execute_contextual_tool` in
> [`gemini/sidecar.py`](./gemini/sidecar.py).

### 7. Route function calls — MCP vs. contextual

When the LLM emits a function call, check the name:

- **In your MCP set** → call the Jaeger MCP server (step 3).
- **In your contextual set** → send an ACP extension method back to the
  gateway.

The extension method is the second crucial piece. Its name is
`_meta/jaegertracing.io/tools/call` (with the leading underscore). The
payload looks like this:

```jsonc
{
  "sessionId": "<the session id from session/new>",
  "name": "ui_show_flamegraph",
  "args": { "trace_id": "abc123" }
}
```

The gateway will respond, **immediately**, with:

```jsonc
{ "result": { "acknowledged": true }, "isError": false }
```

That's it. There is no real result coming back from the browser. UI tools
are commands (navigate, render, filter), not queries, so feed that
acknowledgement to the LLM as the function response and keep going. The
browser saw your `session/update` and is already performing the side
effect. Full rationale in
[RFC 0002 §6.6](../../docs/rfc/0002-ai-gateway-contextual-tools.md#66-why-fire-and-forget).

> **Gotcha — leading underscore quirk.** Some ACP libraries (the Python one,
> for instance) automatically prepend the `_` to extension method names at
> send-time, so the constant in user code reads
> `meta/jaegertracing.io/tools/call`. Other libraries want you to include
> it. Check what yours does — the bytes on the wire must be
> `_meta/jaegertracing.io/tools/call`.

> **Gotcha — don't wait for the browser.** If you block on the
> extension-method response hoping for a "real" result, you'll deadlock.
> The ack *is* the result.

> Reference: `_execute_contextual_tool` in
> [`gemini/sidecar.py`](./gemini/sidecar.py), specifically the
> `conn.ext_method(EXT_METHOD_JAEGER_TOOL_CALL, ...)` call around line 220.

### 8. Clean up on prompt end

When `session/prompt` returns (success, error, or client disconnect), drop
the snapshot you stored in step 4. The gateway opens one ACP session per
chat request and never reuses session ids, so cleanup is unconditional —
just `pop` the entry.

> Reference: the `finally` block at the end of `prompt` in
> [`gemini/sidecar.py`](./gemini/sidecar.py).

---

## The Three Constants You Must Agree On

Wire-level strings that have to match exactly on both sides:

| Constant                       | Value                                | Where it appears                                       |
| ------------------------------ | ------------------------------------ | ------------------------------------------------------ |
| `CONTEXTUAL_TOOLS_META_KEY`    | `jaegertracing.io/contextual-tools`  | The key inside `_meta` on `NewSessionRequest`          |
| `ExtMethodJaegerToolCall`      | `_meta/jaegertracing.io/tools/call`  | ACP extension method, sidecar → gateway                |
| `UIToolPrefix`                 | `ui_`                                | Prepended by the gateway to every contextual tool name |

---

## Verify It Works

End-to-end smoke test — works for either path.

### 1. Start Jaeger with your sidecar configured

```yaml
# config.yaml
extensions:
  jaeger_query:
    ai:
      agent_url: "ws://localhost:16688"
```

```bash
go run ./cmd/jaeger --config config.yaml
```

### 2. Start your sidecar

For Path A:

```bash
cd scripts/ai-sidecar/myprovider
export OPENAI_API_KEY=...   # or whichever provider
uv run python main.py
```

You should see:

```text
Jaeger ACP Sidecar listening on ws://localhost:16688
```

### 3. Send a chat request

```bash
curl -N -X POST http://localhost:16686/api/ai/chat \
  -H 'Content-Type: application/json' \
  -d '{
    "threadId": "t1",
    "runId": "r1",
    "messages": [{"role": "user", "content": "what services are running?"}],
    "tools": []
  }'
```

You should see a stream of AG-UI SSE frames: `RUN_STARTED`,
`TEXT_MESSAGE_START`, one or more `TEXT_MESSAGE_CONTENT`, optionally
`TOOL_CALL_*` frames if the LLM decides to call MCP tools, then
`TEXT_MESSAGE_END`, `RUN_FINISHED`.

### 4. Test the contextual-tool path

Add a contextual tool to the request and ask the model to use it:

```bash
curl -N -X POST http://localhost:16686/api/ai/chat \
  -H 'Content-Type: application/json' \
  -d '{
    "threadId": "t1", "runId": "r2",
    "messages": [{"role": "user", "content": "show the flamegraph for trace abc123"}],
    "tools": [{
      "name": "show_flamegraph",
      "description": "Open the flamegraph view for a trace_id.",
      "parameters": {"type":"object","properties":{"trace_id":{"type":"string"}},"required":["trace_id"]}
    }]
  }'
```

You should see `TOOL_CALL_START` / `TOOL_CALL_ARGS` / `TOOL_CALL_END` frames
for `show_flamegraph` (note: no `ui_` prefix — the gateway stripped it
before forwarding to the browser).

### 5. Borrow the reference tests

The Gemini sidecar ships two pytest files worth mirroring in whatever test
framework you use:

- [`gemini/test_sidecar_workflow.py`](./gemini/test_sidecar_workflow.py) —
  connects to a running sidecar over WebSocket and drives the full
  `initialize` → `session/new` → `session/prompt` flow against a mocked LLM.

### 6. Confirm the UI lights up automatically

You don't need to flip a UI flag. The Jaeger backend periodically probes the
configured `agent_url` (default every 5s; tunable via
`jaeger_query.ai.health_probe_interval`) and advertises the result to the UI
as a backend capability. Open the Jaeger UI in a fresh browser tab after
your sidecar is responding to `initialize` — the chat surface should appear
on the next page load. Stop the sidecar and the chat surface goes away the
same way.

---

## Where to Read More

- **Architecture and protocol details:**
  [gateway README](../../cmd/jaeger/internal/extension/jaegerquery/internal/jaegerai/README.md).
- **Why it's designed this way:**
  [RFC 0002](../../docs/rfc/0002-ai-gateway-contextual-tools.md).
- **Working code to copy from:** [`gemini/`](./gemini) and its
  [README](./gemini/README.md).
