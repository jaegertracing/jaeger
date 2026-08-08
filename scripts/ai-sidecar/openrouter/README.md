# Jaeger OpenRouter ACP Sidecar

A thin, model-agnostic BYOA agent for Jaeger's AI assistant. Point it at any
model OpenRouter hosts by setting one environment variable.

## Why this exists

The Gemini sidecar ties Jaeger's AI assistant to one vendor and to google-adk's
agent framework. This one does not: it speaks the same ACP surface to the
gateway, but the model behind it is an OpenAI-style chat completion against
OpenRouter, so any tool-calling model there can drive the assistant. Operators
who already have an OpenRouter key, or who want a model other than Gemini —
whether for cost, licensing, data residency, or simple preference — can run the
assistant without a code change.

It is deliberately thin. There is no agent framework here, four runtime
dependencies, and roughly 400 lines of agent: the ACP handshake, an MCP client,
and the loop between them. That makes it easy to read, easy to fork for a
provider OpenRouter does not cover, and cheap to keep working as the gateway
evolves.

Being thin also makes it a useful **reference arm for evaluation**. Because it
adds no repair or retry logic of its own, what you observe is the model's own
behaviour rather than a framework's compensation for it — so it can serve as a
control when comparing models, or when comparing harnesses by running the same
model through it and through the ADK sidecar. That is a consequence of the
design, not its only purpose.

## Design principles

**Hardened at the edges, transparent in the middle.**

At the edges (present by design): a timeout on every network call; clean
shutdown on `SIGINT`/`SIGTERM`; structured logs carrying session id, tool name
and status — never payload bodies, never the API key; and every exception
surfacing as an ACP error update before the turn dies.

In the middle (absent by design): no retries on model or tool calls; no
validation or repair of model-emitted tool calls — malformed tool-call JSON
raises and shows up as a failure; no massaging of MCP tool schemas — if a model
chokes on jaegermcp's JSON Schema shapes, that is worth knowing rather than
hiding; no response caching, request queuing, rate limiting, budget caps, or
context folding; no middleware or plugin structure of any kind.

The reason is that a layer which quietly fixes a model's mistakes also hides
them. A malformed tool call that gets silently corrected here is a bug you never
learn about — in the model, in a skill's wording, or in a tool's schema. The
iteration ceiling (`MAX_ITERATIONS`) follows the same rule: it raises rather
than returning a partial answer, so a truncated turn is never mistaken for a
finished one.

**The sidecar has zero skill awareness.** It has never heard of
`detect-n-plus-one`. Skill selection is model-side: jaegermcp serves a root
index, the model reads it and follows wiki links into whichever skill fits the
task. The sidecar relays tools and executes calls. That the whole skills system
works through an agent that knows nothing about skills is the Open Knowledge
Format model working as designed.

## How it works

The gateway dials the sidecar; the sidecar dials OpenRouter and jaegermcp.

```text
jaegerai gateway ──ACP over WebSocket (the gateway dials us)──▶ sidecar
sidecar ──OpenAI-style chat completions over HTTPS──▶ OpenRouter
sidecar ──MCP (streamable HTTP)──▶ jaegermcp
```

Two architectural invariants follow from BYOA. The sidecar runs in the user's
environment and never touches Jaeger's disk — only MCP and HTTP cross the
boundary, and skills reach the model solely as MCP tool results. And the
sidecar is stateless with respect to Jaeger: everything it knows about a turn
arrives over ACP.

Per turn:

1. The gateway opens a WebSocket. `handle_websocket` bridges it to ACP's stdio
   framing through a socketpair, so no ACP framing is reimplemented here.
2. `initialize` advertises two capabilities: `session/close` support, and
   **streamable-HTTP MCP**.
3. `session/new` arrives carrying the turn-scoped MCP URL the gateway minted.
   The sidecar builds one `JaegerMCPClient` per session against that URL.
4. On the first prompt the sidecar connects to MCP, lists tools once, and seeds
   the message history with the base system prompt **plus the MCP server's own
   `instructions` text**.
5. The loop runs: POST the full history to OpenRouter → if the reply has no
   `tool_calls`, emit it and end the turn → otherwise execute each call over
   MCP, stream `tool_call_start` / `update_tool_call` to the browser, append the
   result as a `role: "tool"` message, and go again.
6. `session/close` drops the history and closes the MCP client.

Because OpenRouter is stateless, the full history is resent on every completion
call. That is not an inefficiency to fix — it makes the request body the
token-measurement surface, so no extra instrumentation is needed to know exactly
what a model was shown.

### What makes it agentic

Step 5 is the agent loop, and the agency lives in the model, not in this code.
The sidecar never decides which tool to call, in what order, or when the
investigation is finished. It runs one rule — *if the reply contains tool calls,
execute them and ask again; otherwise the turn is over* — and the model drives
everything else through it. Control flow is the model's output.

The loop is what makes multi-step investigation possible: each tool result is
appended to the history, so the model chooses its next call knowing what the
previous ones returned. A skill's procedure is executed by the model reading the
skill and then walking its steps through this loop, one round trip at a time.

Concretely, from a verified run (see below), the model spent its first two round
trips reading the skills index and then the skill it picked, and only then began
querying telemetry. Nothing in this repository told it to do that.

The one thing the loop imposes is a ceiling (`MAX_ITERATIONS`), and when a turn
hits it the sidecar raises rather than returning a partial answer, because a
truncated run recorded as a completed one would corrupt an evaluation.

### Verified end-to-end

Run against HotROD through the real gateway with
`OPENROUTER_MODEL=nvidia/nemotron-3-super-120b-a12b:free`, the model produced
this call sequence unaided:

```text
read_skill(SKILL.md)                     <- entered the catalog from MCP instructions
read_skill(detect-n-plus-one/SKILL.md)   <- followed the index to a skill
get_services / search_traces             <- found a candidate trace
get_trace_topology / get_span_details    <- walked the span tree
search_traces(duration_min=500ms)        <- confirmed against slow traces
```

It then correctly reported HotROD's genuine N+1 — `FindDriverIDs` returning ten
driver IDs followed by ten sequential Redis `GetDriver` calls — applying the
skill's own discriminating criteria (sequential rather than overlapped
execution, similar durations, same downstream target). The sidecar contributed
no skill knowledge to that result.

## Setup

| Variable | Required | Default | Meaning |
| --- | --- | --- | --- |
| `OPENROUTER_API_KEY` | yes | — | OpenRouter key. Environment only — never a flag, never logged, never in an error message. |
| `OPENROUTER_MODEL` | yes | — | Model slug, e.g. `google/gemini-2.5-flash`. The variable under test. |
| `JAEGER_MCP_URL` | no | `http://localhost:16686/mcp` | **Fallback only.** Used when no gateway announces an MCP server. |
| `SIDECAR_HOST` | no | `localhost` | WebSocket bind host. |
| `SIDECAR_PORT` | no | `16688` | WebSocket bind port; matches the gateway's `DefaultAIAgentURL`. |
| `JAEGER_MCP_TIMEOUT_SEC` | no | `30` | MCP connect and per-tool-call timeout. |
| `OPENROUTER_TIMEOUT_SEC` | no | `120` | OpenRouter request timeout. |
| `MAX_ITERATIONS` | no | `25` | Model round trips before a turn is abandoned with an error. |
| `EVAL_TRANSCRIPT` | no | unset | Path for the JSONL turn transcript. Unset means every hook is a no-op. |

Run it:

```bash
export OPENROUTER_API_KEY=sk-or-...
export OPENROUTER_MODEL=google/gemini-2.5-flash
uv sync
uv run python main.py
```

Then point Jaeger at it — `ws://localhost:16688` is the gateway's default, so
usually nothing to configure. The gateway dials the sidecar, never the reverse,
so the sidecar needs no inbound Jaeger credentials.

## Choosing a model

Switching models is the whole point, and it is one variable: set
`OPENROUTER_MODEL` to any slug from <https://openrouter.ai/models> and restart.
Nothing else changes — not the prompt, not the tools, not the loop. That is what
makes two runs comparable.

```bash
OPENROUTER_MODEL=nvidia/nemotron-3-super-120b-a12b:free uv run python main.py
```

**The model must support tool calling.** A model without it will answer from
prior knowledge and never touch telemetry, which looks like a catastrophic skill
failure but is really a capability mismatch. Filter for it before you draw
conclusions:

```bash
curl -s https://openrouter.ai/api/v1/models \
  -H "Authorization: Bearer $OPENROUTER_API_KEY" |
  jq -r '.data[] | select(.supported_parameters | index("tools"))
         | select((.pricing.prompt|tonumber) == 0)
         | .id'
```

Paid slugs need credits on the account, and a model you cannot afford fails with
`402 Payment Required` rather than anything model-shaped — worth recognising, as
it is easy to misread as a broken sidecar. Free tool-calling slugs (the `:free`
suffix) are enough to exercise the whole path end to end; the run documented
above used one.

For the harness control described below, run Gemini here with
`OPENROUTER_MODEL=google/gemini-2.5-flash`.

**Handshake auth: none.** `DialWsAdapter` in the gateway dials with a plain
`websocket.Dialer` and a nil header map — no token, no scheme. Bind the sidecar
to loopback and treat the port as trusted. Any auth added later must be
implemented to match whatever the gateway sends; do not invent a scheme here.

## Integration constraints

Seven things that are easy to get wrong. Each was found the hard way, and each
is verified against the pinned SDK versions (`agent-client-protocol` 0.12.0,
`mcp` 2.0.0).

1. **The MCP SDK function is `streamable_http_client`**, not
   `streamablehttp_client`. The latter does not exist in `mcp>=2.0`.
2. **MCP tool schemas expose `input_schema`**, not `inputSchema` — the wire
   alias is camelCase but the Python model attribute is snake_case.
3. **The ACP runtime supplies the connection via `on_connect`**, not the
   constructor. An agent built before the transport exists has nothing to send
   on.
4. **`session/close` is in ACP's unstable protocol**, so `run_agent` must be
   called with `use_unstable_protocol=True`. Confirmed still true in 0.12.0:
   `acp/agent/router.py` registers the route with `unstable=True`. Without the
   flag the route is never registered and the gateway's close fails with
   "Method not found" at the end of every turn.
5. **`update_tool_call` must set `raw_output`**, or the AG-UI stream carries the
   tool call but not its result and the browser renders an empty result panel.
6. **`initialize` must advertise `mcp_capabilities.http`.** The gateway's
   `announceMCP` returns an empty `mcpServers` list unless the agent opts into
   streamable HTTP, so an agent that omits this capability is never told the
   turn-scoped MCP URL — including the server `instructions` that explain how to
   enter the skills catalog. This is the difference between a model that
   ignores skills and a model that was never told they exist.
7. **The MCP connection must be opened and closed in the same task.** ACP
   dispatches every request in its own task, so a connection opened during
   `session/prompt` is closed during `session/close` — a different task. The
   `mcp` transport is built on anyio task groups, whose cancel scopes reject
   that with "Attempted to exit cancel scope in a different task than it was
   entered in", surfacing to the gateway as an Internal error on every turn.
   `JaegerMCPClient` therefore gives the connection its own long-lived task
   (`_serve`) that owns the whole exit stack; other tasks only send requests
   through the session, which is safe.

Constraint 6 pairs with the `instructions` relay in `_start_session`. Listing
tools is not enough: jaegermcp uses the MCP initialize result's `instructions`
field to tell the model where the skills index lives, and a sidecar that drops
that text leaves the model holding tool names and no map. It is relayed
verbatim — paraphrasing it would itself be a repair.

## Harness effects

The Gemini arm and this arm differ in **harness**, not only in model. The Gemini
sidecar drives google-adk, which brings its own tool-dispatch and retry
behaviour; this sidecar has none of that. A raw Gemini-vs-OpenRouter comparison
therefore confounds the two.

The control is to run **Gemini through this sidecar**
(`OPENROUTER_MODEL=google/gemini-2.5-flash`). Same model, different harness —
which isolates how much of any observed difference belongs to the framework
rather than the model.

## Transcripts

Set `EVAL_TRANSCRIPT=/path/to/run.jsonl` and every loop event is appended as one
JSON object per line: `prompt_received`, `session_started` (MCP URL, tool names,
whether instructions arrived), `completion` (the raw `tool_calls` block
verbatim, plus finish reason and usage), `tool_executed` (name, args, status,
result size), and `turn_ended`. Unset, every hook is a no-op with zero effect on
the model path.

## Layout

```
scripts/ai-sidecar/
  shared/ws_commands.py   # WebSocket <-> ACP stdio bridge, shared with the Gemini sidecar
  openrouter/
    main.py               # env config, signal handling, WebSocket server
    sidecar.py            # the ACP agent and the model/tool loop
    mcp_client.py         # streamable-HTTP MCP client, schema and instructions pass-through
    transcript.py         # optional JSONL evaluation transcript
```

Development checks, matching the Gemini sidecar:

```bash
uv run pyright ./
uv run pytest -q
```
