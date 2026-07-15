# RFC 0008: AI Gateway — Unified MCP Tool Routing

- **Status:** Draft
- **Author:** Yuri Shkuro
- **Created:** 2026-07-15
- **Last Updated:** 2026-07-15

This RFC captures the "MCP movement": consolidating every AI tool call — telemetry and UI alike — through a single gateway-hosted MCP dispatch surface. It supersedes the tool-routing decision recorded in [RFC 0002 §5](./0002-ai-gateway-contextual-tools.md) (which chose an ACP extension method and rejected a gateway-hosted MCP server), building on the internal [Tool Routing Design doc][doc-routing] and the runnable spike in POC [#8854][pr-8854]. It does not touch [RFC 0003](./0003-simplify-ai-sidecar-setup.md) (sidecar setup ergonomics), which remains a separate, already-shipping concern.

---

## Implementation status

| Milestone | What | Status |
|---|---|---|
| M0 | Baseline: UI tools via ACP ext-method; telemetry via standalone `jaeger_mcp:16687` | ✅ Shipped pre-movement — [#8423][pr-8423] |
| M1 | Merge `jaeger_mcp` into the query extension: retire the standalone extension; its tools become the `mcptools` library taking `QueryService` directly (removes the inter-extension runtime coupling; one MCP implementation behind both endpoints) | ✅ Done — [#8894][pr-8894] |
| M2 | Turn-scoped MCP endpoint `/api/ai/mcp/<id>/` + turn registry ([#8910][pr-8910]), then serving telemetry **+** per-turn UI tools over it ([#8973][pr-8973]) — dormant until a URL is announced | ✅ Done |
| M3 | Tool-call observability: GenAI span attributes + gateway↔sidecar trace propagation | ✅ Done — [#8942][pr-8942] |
| M4 | Announce the turn-scoped endpoint to the sidecar over **HTTP** (`ai.mcp_base_url`, defaulting to `localhost` — §4.3) | ⏳ In review — [#9009][pr-9009] |
| M5 | **Terminology cleanup** — apply §2 (remove the `session`/`stream` overload; one name for UI tools; rename endpoints/registry/ids) | ⏳ **Next — lands before any further tool-routing PRs** |
| M6 | Migrate the Gemini sidecar onto the gateway MCP URL; drop its own MCP client and the ext-method path | ⏳ Pending |
| M7 | Consolidate the shared and turn-scoped mounts onto **one** `mcp.Server` (two instances today; the turn-scoped middleware already degrades to telemetry-only when no turn is active, so one serves both) | ⏳ Proposed — cleanup |
| M8 | Probe `ai.mcp_base_url` reachability at startup before announcing (analogous to the ACP agent health probe), so a wrong address surfaces at startup, not mid-turn | ⏳ Proposed |
| — | Claude Code sidecar (parallel track; consumes the same URL) | ⏳ In progress — [#8631][pr-8631] |

Spike (not for merge): POC [#8854][pr-8854] validated both transports end-to-end.

---

## Abstract

Every AI sidecar in the repo re-implements the same plumbing: an MCP client dialing Jaeger's telemetry tools, plus per-turn UI-tool handling, plus the ACP WebSocket. Worse, the two kinds of tool call travel different paths — UI tools loop back through a Jaeger-specific ACP extension method, while telemetry tools bypass the gateway entirely by dialing the standalone `jaeger_mcp` server directly. The gateway therefore cannot observe, trace, or gate the majority of tool traffic, and every new agent pays the duplication tax.

This RFC proposes a single dispatch surface: the gateway hosts its own MCP server, and sidecars dial **it** (never `jaeger_mcp` directly). The gateway implements MCP itself and routes each call — UI tools to the browser over the existing SSE stream, telemetry tools to the in-process query service — so both kinds converge on one dispatch point inside the gateway, which it fully observes. Sidecars collapse to thin protocol adapters. The design was first documented in Nabil Salah's [Tool Routing Design doc][doc-routing] and prototyped in POC [#8854][pr-8854]. This RFC fixes the subsystem's overloaded vocabulary (§2), corrects inaccuracies in the prior record (§3), specifies the target design (§4), and tracks the roadmap (status table).

---

## 1. Motivation

Three structural problems compound as agents multiply:

1. **Telemetry tool calls bypass the gateway.** Each sidecar embeds its own MCP client dialing `jaeger_mcp:16687`. The gateway never sees those calls, so it cannot trace, log, authorize, or rate-limit them. This is the core inversion-of-control (IoC) violation.
2. **Two dispatch protocols per sidecar.** UI tools flow agent → gateway → browser via the ACP extension method `_meta/jaegertracing.io/tools/call`; telemetry tools flow through the sidecar's own MCP client. Every sidecar author must implement and understand both.
3. **Per-sidecar duplication.** The MCP client and the UI-tool machinery are re-written in every sidecar (Python Gemini, the in-progress Claude Code bridge, and any future ones). The goal is one dispatch surface owned by the gateway, with sidecars as thin adapters.

Note the scope difference from [RFC 0002](./0002-ai-gateway-contextual-tools.md): that RFC solved **UI-tool** dispatch. This RFC's driving problem is **telemetry** dispatch and the duplication across sidecars — a different problem that reopens a design RFC 0002 had closed (§3.2).

---

## 2. Terminology (decided)

The single biggest readability problem in the package is overloaded vocabulary: "session" names four different things, "stream" three, the frontend tools have two names ("contextual" and "UI"), and the two endpoints are labelled with the very word we want to reserve. This section fixes the vocabulary, and it comes before the design because everything below uses it. **Applying it is the next milestone (M5), to land before any further tool-routing PRs bake the current names into the sidecar-facing contract.**

**Governing principle:**

- **"session"** — *only* a protocol-level session: the **ACP session** (`session/new`) and the **MCP transport session** (`Mcp-Session-Id`, SDK-managed). Never gateway-internal per-turn bookkeeping.
- **"turn"** — one chat exchange = one `POST /api/ai/chat` = one run = one ACP-session lifecycle. Gateway per-turn state uses this word.
- **"stream"** — *only* the SSE channel to the browser.
- **"UI tools"** — the frontend-declared, browser-executed tools. One name — *not* also "contextual tools."
- The two MCP mounts are the **shared endpoint** (no turn id, telemetry only) and the **turn-scoped endpoint** (per turn, telemetry + that turn's UI tools) — never "session-free / session-scoped."
- External AG-UI identifiers (`threadId`, `runId`) are unchanged — they belong to that protocol.

**Concepts and identifiers:**

| Concept | Current | Proposed |
|---|---|---|
| Server-minted per-turn routing id (URL path segment + registry key) | `mcpSessionID`, `{sessionID}` | `mcpRouteID`, `{mcpRouteID}` |
| Frontend / browser-executed tools | "contextual tools" **and** "UI tools" (both used) | **UI tools** (single name) |
| Stateless telemetry mount | "session-free endpoint" | **shared endpoint** |
| Per-turn mount | "session-scoped endpoint" | **turn-scoped endpoint** |
| ACP session id | ACP `SessionId` | unchanged (a real session) |
| MCP transport session id | `Mcp-Session-Id` | unchanged (SDK-owned) |

**Code components (files and types):**

| File → type | Role | Problem | Proposed |
|---|---|---|---|
| `session_streams.go` → `sessionStreams`, `session` | per-turn registry + per-turn state | "session" **+** "stream" glued onto one map | `turn_registry.go` → `turnRegistry`, `turnState` |
| `mcp_endpoint.go` → `mcpSessionHandler` | the turn-scoped MCP endpoint | "session" | `turnScopedMCPHandler` |
| `contextual_tools.go` → `ContextualToolsStore` | ext-method UI-tool store (legacy path) | "contextual" is a second name for "UI"; on the ext-method path being retired | deleted with the ext-method (M6); use UI-tools vocabulary until then |
| `dispatcher.go` → the inbound ACP router | routes inbound ACP JSON-RPC | "dispatch" also names `uiDispatchMiddleware` | optional: name it the ACP router (`acpRouter`) |
| `mcp_ui_tools.go` → `uiDispatchMiddleware` | layers a turn's UI tools onto the MCP server | consistent with "UI tools" | unchanged |
| `streaming_client.go` → `streamingClient` | the SSE writer to the browser | "stream" reserved for SSE, so consistent | unchanged |
| `handler.go`, `routes.go`, `translation.go`, `trace_propagation.go`, `ws_adapter.go` | HTTP handlers; route registration; AG-UI↔ACP translation; trace propagation; WebSocket adapter | fine | unchanged |

Rationale for `mcpRouteID` over the tempting alternatives: `turnID`/`runID` collide with `runId` (which already *is* the turn id from the client's side — there are three per-turn ids with identical lifecycles, distinguished only by provenance and ordering); `streamID`/`streamKey` pile onto the already-saturated "stream" cluster; anything with "session" collides with the two real protocol sessions. `mcpRouteID` names the id by its job — routing the MCP callback for a turn — on an axis nothing else uses. `callbackID` or `rendezvousID` are acceptable synonyms if `route` reads oddly. **The principle is decided** (session = protocol only; turn = gateway bookkeeping; stream = SSE only; one name for UI tools; shared/turn-scoped endpoints) and the names above are adopted, pending wider review of this RFC. Doing it now is cheap: `mcpSessionID` and the `{sessionID}` path segment have existed since [#8910][pr-8910], but the endpoint is dormant — its URL is not announced to any sidecar until [#9009][pr-9009] (still open), and no sidecar consumes these names yet — so nothing external is bound to them. It gets expensive once the URL is announced and a sidecar contract forms (M6).

---

## 3. Correcting the record

The prior record ([RFC 0002][rfc-0002], the [Tool Routing Design doc][doc-routing], and issue [#8890][issue-8890]) contains statements that are now inaccurate or were never accurate. This section fixes them so the forward design rests on true premises.

### 3.1 The baseline before the movement

The architecture that existed when this work began:

```
Browser ──[AG-UI / HTTP+SSE]──► Gateway (jaeger-query :16686) ──[ACP / WebSocket]──► Sidecar ──► LLM
                                     │                                                   │
                                UI tool call:                                    Telemetry tool call:
                                agent → gateway via ACP ext-method               sidecar dials jaeger_mcp:16687
                                _meta/jaegertracing.io/tools/call                DIRECTLY — bypasses the gateway
                                (fire-and-forget; browser executes                (gateway is blind to these calls)
                                 the side effect off the SSE stream)
```

- The chat endpoint (`POST /api/ai/chat`) bridges AG-UI ↔ ACP (RFC 0002).
- UI tools (called *contextual tools* on this legacy path): the browser declares them per turn in its chat request (`RunAgentInput.tools`); the gateway advertises that snapshot to the sidecar by attaching it to `NewSessionRequest.Meta` (namespaced `jaegertracing.io/contextual-tools`, names prefixed `ui_`); the sidecar registers them with the LLM as callable tools. When the LLM calls one, the sidecar dispatches it back to the gateway via the ACP ext-method `_meta/jaegertracing.io/tools/call`; the gateway acknowledges fire-and-forget and, in parallel, emits `TOOL_CALL_*` events on the browser's SSE stream, from which the browser performs the actual side effect ([#8423][pr-8423]).
- Telemetry tools: served by the standalone `jaeger_mcp` OTel extension on `:16687`; each sidecar dials it directly.

### 3.2 RFC 0002 rejected a gateway-hosted MCP server for UI tools; the broader telemetry problem revives it

[RFC 0002 §5.2][rfc-0002] rejected a per-turn gateway-hosted MCP server and [§5.4][rfc-0002] chose the ACP ext-method. Two considerations drove that choice: (1) **minimize the number of distinct data flows** — reuse the single open ACP WebSocket instead of adding another connection; and (2) an **assumption that ACP was built for exactly this** — that the agent could send tool calls back over that same ACP connection (what MCP-over-ACP would provide).

Both premises have since shifted:

- **(2) was wrong.** ACP provides no usable same-connection tool-call path today: MCP-over-ACP is an UNSTABLE, unfinished draft (§3.3). The assumption that made reusing the WebSocket the clean choice does not hold.
- **(1) gets worse, not better.** Routing tool calls through a gateway-hosted MCP server over HTTP *adds* a distinct data flow — a second connection back into the process, even on `localhost` — the opposite of minimizing flows. (The *configuration* cost is largely avoided by defaulting `ai.mcp_base_url` to `localhost` (§4.3), which works for a co-located sidecar; only other topologies need an override. But the extra connection itself remains.)

So the gateway-hosted server is not a strict win on RFC 0002's own criteria — it is a deliberate trade-off, justified by a **larger problem RFC 0002 was not solving**: telemetry tool calls bypass the gateway entirely (§1), and the goal is to put the gateway on the path of *every* tool call. The reconciling point with RFC 0002's objection is that the issue was never *whether* the agent dials an MCP server — MCP is simply how agents consume tools — but *which* server: dialing `jaeger_mcp` (or any external server) directly leaves the gateway blind (the violation), whereas dialing the gateway's **own** MCP endpoint keeps it on the path of every call, able to see, trace, and gate the traffic. Accepting the extra flow and the URL config is the price of that visibility — and MCP-over-ACP, if it stabilizes, would later recover consideration (1) by folding the flow back onto the WebSocket (§6).

Action: RFC 0002 gets a pointer banner to this RFC noting that its Alternative B is revived under a broader problem statement. Its historical analysis is preserved, not rewritten.

### 3.3 MCP-over-ACP: the actual SDK state

The "keep everything on the one WebSocket" transport (MCP-over-ACP) matters because the whole "HTTP now, ACP later" plan rests on it. It exists only in the **Go** ACP SDK (`coder/acp-go-sdk`), which the *gateway* uses. The shipped Gemini sidecar is Python, and the in-progress Claude Code sidecar ([#8631][pr-8631]) is Node.js on `claude-agent-acp` — neither's ACP SDK provides the tunnel. It is therefore a future opportunity, not a path either sidecar can take today. In the Go SDK it is UNSTABLE and unfinished: `coder/acp-go-sdk` v0.13.0 (the pinned version) and v0.13.5 (the version #8890 cites) are identical for this feature, and both provide a **handshake scaffold, not a turnkey tunnel**. The [v0.13.0 release notes][rel-0130] call it *"unstable MCP-over-ACP support"* ("Agents can now host MCP servers over the ACP channel itself"); concretely that support is:

- **Wire types + method-name constants** for all three methods (`mcp/connect`, `mcp/message`, `mcp/disconnect`), plus `McpServer.Acp` and `McpCapabilities.Acp`. All are explicitly **UNSTABLE** — each carries the generated comment *"not part of the spec yet, and may be removed or changed at any point."* (`McpServer.Http` is stable by contrast; `Stdio` is mandatory.)
- **`mcp/connect` / `mcp/disconnect`: handshake plumbing only.** The SDK generates a sender on the agent side (`AgentSideConnection.UnstableConnectMcp` / `UnstableDisconnectMcp`) and inbound dispatch cases on the client side — but the dispatch delegates to *optional* interface methods (`UnstableConnectMcp` / `UnstableDisconnectMcp`) that the consumer must implement, and returns `MethodNotFound` if it hasn't. The SDK supplies routing, **not behavior**.
- **`mcp/message`: types only.** This is the method that carries the actual MCP payloads (`tools/list`, `tools/call`). The SDK has its *types* and *constants* but — unlike connect/disconnect — **no generated sender and no dispatch case** (the release notes list a *method* only for connect/disconnect, and types for message). A consumer must drive it by hand on the low-level `SendRequest`/notification primitives.

So the SDK provides the wire vocabulary plus connect/disconnect routing hooks; the substantive work — the behavior behind connect/disconnect (the optional interface methods) and the entire `mcp/message` ↔ MCP-server bridge — falls to the consumer. POC [#8854][pr-8854] implements exactly that: its own `HandleConnect` / `HandleMessage` / `HandleDisconnect` in `mcp_acp_dispatch.go`.

Both prior sources describe this state inaccurately:

- The [Tool Routing Design doc][doc-routing] (Q3) says the `acp` variant *"does not exist in the ACP spec or claude-agent-acp's implementation; was misread by the author."* It is partly accurate: MCP-over-ACP is an unaccepted **draft RFD** ([mcp-over-acp][rfd-mcp-acp]), so it is not in the stable spec, and `claude-agent-acp` does not implement it. The blanket "does not exist" is wrong: the Go SDK carries it, as UNSTABLE scaffolding (above) rather than a finished implementation.
- Issue [#8890][issue-8890] says it is *"implemented in the Go ACP SDK (`coder/acp-go-sdk v0.13.5+`)."* Overstated on both counts: the scaffolding is already in the pinned **v0.13.0** (it predates 0.13.5), and even at **v0.13.5** it is not a full implementation — `mcp/message` is unwired and the capability is UNSTABLE.

Two consequences for the design:

1. **Both ends have real work; neither is "ready."** The gateway must implement the connect/disconnect interface methods and the entire `mcp/message` bridge itself (the SDK routes the handshake but serves nothing — above); and neither sidecar can consume the transport at all, because it exists only in the Go SDK — the shipped Gemini sidecar's Python ACP SDK lacks it, as does the `claude-agent-acp` (Node.js) that the in-progress Claude Code sidecar ([#8631][pr-8631]) uses. The gateway side is feasible — POC [#8854][pr-8854] does it in roughly one dispatch file — but "HTTP first" avoids implementation work on *both* ends, not just the sidecar.
2. **The transport is a moving target.** MCP-over-ACP is an open draft RFD ([mcp-over-acp][rfd-mcp-acp]) and the SDK marks it "may be removed or changed at any point." So HTTP — the *stable* MCP transport — is the **durable default**, and MCP-over-ACP an optimization to add once the RFD settles and both ends implement it; not a destination to migrate everything onto. This is the reverse of the "HTTP = stepping stone, ACP = destination" framing in the prior record. See §4.3.

---

## 4. Proposed design

### 4.1 One gateway-hosted MCP dispatch surface

The gateway hosts an MCP server and implements the MCP methods itself (it is **not** an HTTP redirect or transparent proxy):

| MCP method | Behavior |
|---|---|
| `initialize` | Advertises the `tools` capability. |
| `tools/list` | Returns telemetry tools **+** the calling turn's UI tools (UI wins on name collision). |
| `tools/call` | Routes by name. A UI tool: emit its `TOOL_CALL_*` events on the browser's SSE stream (the browser executes the side effect) and immediately return a synthetic "dispatched" result to the caller — the gateway does not wait for the browser (fire-and-forget; §4.4 explains why). Any other name: execute the telemetry tool and return its real result. |

The UI-vs-telemetry decision is made once, in the gateway's `tools/call` handling — today the `uiDispatchMiddleware` layered on the MCP server — which the gateway fully observes (M3 gives it tracing). Sidecars stop carrying an MCP client and UI-tool machinery; they become thin ACP adapters that point their MCP client at the gateway URL.

### 4.2 Shared vs. turn-scoped endpoints

Two mounts on the query port:

```
/api/ai/mcp/              → shared       — telemetry tools only; stateless; for external MCP clients (Cursor, IDEs)
/api/ai/mcp/<mcpRouteID>/ → turn-scoped  — telemetry + this turn's UI tools; for the sidecar mid-chat
```

The turn-scoped path carries the per-turn id (`mcpRouteID`, §2) so the gateway can look up the turn's UI-tool snapshot and SSE stream. The shared path replaced the standalone `jaeger_mcp:16687` for **external MCP clients** — Cursor, IDEs, and other AI tools that are *not* Jaeger's own chat sidecar — when the extension was merged into the query extension (§4.5). Making the id optional is what lets one implementation serve both audiences.

### 4.3 Transport (HTTP; MCP-over-ACP deferred)

The turn-scoped endpoint is served over **HTTP** (standard streamable-HTTP MCP), and that is the **permanent** transport: it works with any MCP-speaking agent and there is no plan to replace it. A second transport, MCP-over-ACP, was evaluated and **deferred as a future enhancement** (§6).

**HTTP**, at `/api/ai/mcp/<mcpRouteID>/`. The gateway announces the URL in `NewSessionRequest.mcpServers`, but only when the agent advertised `mcpCapabilities.http` in its `InitializeResponse` (announcing a transport the agent cannot consume would make it fail the session) ([#9009][pr-9009]).

**Decision — default the base URL to localhost.** `ai.mcp_base_url` defaults to the gateway's own address on `localhost` (scheme + query port from its HTTP config, e.g. `http://localhost:16686`). That works whenever the sidecar is co-located with the gateway — the common deployment, a sidecar in the same pod dialing `localhost` — so the endpoint is announced out of the box, with no configuration. Operators override `ai.mcp_base_url` only when the sidecar reaches the gateway at a *different* address (behind a proxy, in another network namespace, or with TLS terminated elsewhere); the query server can infer the localhost address but not that one. (This revises [#9009][pr-9009], which currently defaults the value empty and announces nothing.)

**Reachability probe (M8).** The gateway should probe the base URL — the localhost default or an override — before relying on it, analogous to the existing ACP agent health probe, so a wrong address surfaces at startup rather than mid-turn.

**Why not MCP-over-ACP now.** It would keep all tool traffic on the single ACP WebSocket, needing no second connection and no reachable-URL config. But it is an UNSTABLE, unfinished draft RFD that may never be finalized; its `mcp/message` bridge is unimplemented in the SDK; and none of our sidecars' SDKs support it (§3.3 — Python for the shipped Gemini sidecar, `claude-agent-acp`/Node.js for the in-progress Claude Code sidecar [#8631][pr-8631]). HTTP works today and is stable, so MCP-over-ACP is recorded as a future enhancement (§6), not planned work.

The transports considered (🟢 good / 🟡 partial / 🔴 poor):

| Criterion | ext-method (legacy, retired — §4.4) | HTTP (chosen) | MCP-over-ACP (deferred) |
|---|---|---|---|
| Reuses the one open connection | 🟢 | 🔴 second HTTP dial back into the process | 🟢 |
| Covers telemetry **and** UI on one path | 🔴 UI only | 🟢 | 🟢 |
| Gateway observes/gates every call (IoC) | 🟡 UI only | 🟢 | 🟢 |
| Transport speakable by today's sidecar SDKs | 🟢 | 🟢 | 🔴 Python/Node SDKs lack it |
| No externally-reachable URL to configure | 🟢 | 🔴 needs `ai.mcp_base_url` | 🟢 |
| Standard MCP `tools/call` (no custom method) | 🔴 Jaeger-custom `_meta/…/tools/call` | 🟢 | 🟢 |
| Stable wire contract | 🟡 Jaeger-defined: stable, but bespoke | 🟢 standard, stable | 🔴 UNSTABLE draft |

HTTP is the only option that is standard, stable, **and** speakable by today's sidecars; the ext-method is retired (§4.4) and MCP-over-ACP deferred (§6).

### 4.4 UI-tool dispatch is fire-and-forget today (an implementation limit), and the ext-method is retired

Today the gateway dispatches a UI tool by emitting its `TOOL_CALL_*` events on the browser's SSE stream and immediately returning a synthetic ack to the caller — the old path returns `{acknowledged: true}` (`dispatcher.go`), the MCP path a `"…dispatched to the browser"` `CallToolResult` (`mcp_ui_tools.go`). It does not wait for, and cannot receive, a result from the browser.

This is a property of the current **implementation**, not of UI tools and not of AG-UI. The browser↔gateway leg is a single `POST /api/ai/chat` whose response is a one-way SSE stream (server→browser); there is no channel for the browser to return a value mid-turn, and RFC 0002 §6.6 deliberately chose to synthesize the ack rather than build one (a `POST /api/ai/tool-result` endpoint plus per-call rendezvous state). AG-UI itself supports frontend tool calls that return structured results (plus HITL interrupts and shared-state sync), so query-type UI tools such as `get_current_viewport` or `read_selection` are not categorically impossible — they are unsupported only until a browser→gateway result channel exists. That is **out of scope for this RFC** (§6): nothing in the gateway↔sidecar protocol prevents it, but it needs UI-side integration and belongs in its own RFC.

The agent→gateway transport is orthogonal to all of this. On every transport the gateway acks a UI-tool call and returns a real result for a telemetry call, so from the agent's side the call is synchronous either way — the ext-method's `{acknowledged: true}` is a normal request/response that acks only because there is no browser result to return, not because the transport cannot carry one. Switching from the ACP ext-method to MCP `tools/call` therefore does not touch the browser leg. **The MCP `tools/call` route replaces the custom ext-method — they do not coexist:** when the Gemini sidecar migrates (M6), the ext-method path (`dispatcher.go` + `ContextualToolsStore`) is removed.

### 4.5 `jaeger_mcp` is merged into the query extension

The telemetry tools no longer live in a standalone `jaeger_mcp` extension; they are merged into the query extension as the `mcptools` library ([#8894][pr-8894]). This follows directly from §4.1:

- Serving UI tools as MCP tools requires an MCP server **inside the gateway** — one with access to the per-turn UI state — and the gateway lives in the query extension. So an in-gateway MCP server must exist regardless of `jaeger_mcp`.
- The standalone `jaeger_mcp` extension ([ADR-002](../adr/002-mcp-server.md)) had a runtime dependency on the query extension: it fetched `QueryService` via `GetExtension(host)` at startup. The separate-extension boundary bought nothing except that coupling.
- Once an in-gateway MCP server is needed anyway, a separate extension is redundant. Merging `jaeger_mcp`'s tool handlers into the query extension (they already took `*querysvc.QueryService` as a parameter) removes the inter-extension coupling — `mcptools.NewServer` now takes `QueryService` directly — and lets one MCP implementation (the `mcptools` library) back both the shared and turn-scoped endpoints, advertised to an agent with or without a turn id (today via two server instances; collapsing them to one is a cleanup milestone, M7).

The standalone extension and its `:16687` listener are retired; the shared `/api/ai/mcp/` mount on the query port (`:16686`) replaces them for external MCP clients (Cursor, IDEs, and other AI tools that are not Jaeger's own chat sidecar).

### 4.6 External MCP servers: pass-through, not proxy

The gateway serves Jaeger's own telemetry in-process (§4.5); it does **not** proxy other MCP servers. ACP's `NewSessionRequest.mcpServers` is a list, so the architectural stance for any external MCP servers an operator might configure is **pass-through** — the gateway announces them and the agent dials them directly — rather than routing them through the gateway. Proxying external servers through the gateway, to bring their tool calls under the same tracing and gating as Jaeger's, is a possible future enhancement (§6). Neither external-server configuration nor proxying is built today.

---

## 5. Roadmap

See the **Implementation status** table at the top. In narrative:

- **Done:** `jaeger_mcp` is merged into the query extension (M1, [#8894][pr-8894]); the turn-scoped endpoint serves telemetry + UI tools (M2, [#8973][pr-8973]); tool calls are traceable (M3, [#8942][pr-8942]). The endpoint stayed dormant until announced.
- **In review:** announce the endpoint to the sidecar over HTTP (M4, [#9009][pr-9009]) — the PR that wakes the dormant endpoint.
- **Next — before any further tool-routing PRs:** the terminology cleanup (M5, §2), so the renamed ids/endpoints are in place before a sidecar contract forms. #9009 (M4) should adopt the final names before it merges.
- **Then:** migrate the Gemini sidecar onto the URL and remove its MCP client + the ext-method (M6) — the sidecar-simplification payoff and the ext-method retirement.
- **Cleanups:** consolidate the two MCP server instances (M7); add the reachability probe (M8).
- **Out of scope:** MCP-over-ACP, query-type UI tools, and gateway-as-proxy — see §6.

---

## 6. Out of scope (future enhancements)

Considered and deliberately deferred; each decision is stated in the section noted.

- **MCP-over-ACP transport (§4.3, §3.3).** Keeping tool traffic on the one WebSocket is attractive, but the transport is an UNSTABLE, unfinished draft RFD ([mcp-over-acp][rfd-mcp-acp]) that may never be finalized, its `mcp/message` bridge is unimplemented, and no sidecar SDK supports it. HTTP is the permanent transport; MCP-over-ACP may be revisited only if the RFD is accepted and the SDKs implement it.
- **Query-type UI tools (§4.4).** UI tools that return data to the LLM (read the viewport, a selection, a form value). Nothing in the gateway↔sidecar protocol prevents them; they need a browser→gateway result channel and UI-side integration, so they belong in their own future RFC.
- **Gateway proxying external MCP servers (§4.6).** External MCP servers are passed through — the agent dials them directly. Routing them through the gateway, to bring their tool calls under the same tracing and gating, is a possible later enhancement.

---

## Appendix — Provenance

- **RFC 0002** — the ACP ext-method design this RFC supersedes for tool routing. Its analysis is preserved as a historical snapshot; a banner points here.
- **RFC 0003** — sidecar setup ergonomics (launchers, `aiAssistant` capability). Unrelated to tool routing; unaffected.
- **[Tool Routing Design doc][doc-routing]** (internal) — where "Option B" was laid out and the IoC framing (raised in [#8631][pr-8631] review) written up; corrected on the coder SDK facts here (§3.3).
- **Issue [#8890][issue-8890]** (supersedes [#8853][issue-8853]) — the convergence proposal; corrected on the SDK version/stability here.
- **POC [#8854][pr-8854]** — runnable spike proving both transports; explicitly not for merge; being split into the milestone PRs above.

[rfc-0002]: ./0002-ai-gateway-contextual-tools.md
[rfc-0003]: ./0003-simplify-ai-sidecar-setup.md
[doc-routing]: https://docs.google.com/document/d/1CHzLmqR9vjsFGEs6ngS4BBFR352r6n2sruhqrZCiMyE/edit
[rfd-mcp-acp]: https://agentclientprotocol.com/rfds/mcp-over-acp
[rel-0130]: https://github.com/coder/acp-go-sdk/releases/tag/v0.13.0
[pr-8423]: https://github.com/jaegertracing/jaeger/pull/8423
[pr-8631]: https://github.com/jaegertracing/jaeger/pull/8631
[pr-8894]: https://github.com/jaegertracing/jaeger/pull/8894
[pr-8910]: https://github.com/jaegertracing/jaeger/pull/8910
[pr-8854]: https://github.com/jaegertracing/jaeger/pull/8854
[pr-8942]: https://github.com/jaegertracing/jaeger/pull/8942
[pr-8973]: https://github.com/jaegertracing/jaeger/pull/8973
[pr-9009]: https://github.com/jaegertracing/jaeger/pull/9009
[issue-8853]: https://github.com/jaegertracing/jaeger/issues/8853
[issue-8890]: https://github.com/jaegertracing/jaeger/issues/8890
