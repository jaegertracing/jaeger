# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

"""ACP agent backed by OpenRouter.

Presents the same ACP surface to the Jaeger AI gateway as the Gemini sidecar,
but the model behind it is a plain OpenAI-style chat completion, so any
tool-calling model OpenRouter hosts can drive the assistant. Choosing one is a
matter of setting OPENROUTER_MODEL — no agent framework, no vendor SDK.

DESIGN INTENT — read before changing anything here.

This agent executes exactly what the model emits. Nothing between the model and
the wire repairs, retries, validates, coerces, or caches, because a layer that
quietly fixes a model's mistakes also hides them: a malformed tool call that
gets corrected here is a bug nobody learns about — in the model, in a skill's
wording, or in a tool's schema. Staying transparent is also what lets this
sidecar serve as a control when comparing models or harnesses.

Hardened at the edges, transparent in the middle.

Edges (these belong here):
  - a timeout on every network call — OpenRouter, MCP, WebSocket
  - clean shutdown on SIGINT/SIGTERM
  - structured logs carrying session id, tool name, status — never payload
    bodies, never the API key
  - every exception surfaces as an ACP error update before the turn dies

Middle (these must never appear here):
  - retries on model calls or tool calls — a retry is a repair
  - validation, coercion, or fixing of model-emitted tool calls — malformed
    tool-call JSON raises and surfaces loudly
  - massaging of MCP tool schemas — straight pass-through; a model choking on
    jaegermcp's JSON Schema shapes is worth knowing rather than hiding
  - response caching, request queuing, rate limiting, budget caps, context
    folding, middleware, plugins

The sidecar has zero skill awareness. Skill selection is model-side: jaegermcp
serves a root index and the model follows wiki links into the skill it wants.
This file relays tools and executes calls. That is the whole job.
"""

import asyncio
import json
import logging
import os
import socket
from typing import Any, Callable

import httpx2
from acp import (
    PROTOCOL_VERSION,
    Agent,
    InitializeResponse,
    PromptResponse,
    run_agent,
    text_block,
    update_agent_message,
)
from acp.helpers import start_tool_call, tool_content, update_tool_call
from acp.interfaces import Client
from acp.schema import (
    AgentCapabilities,
    CloseSessionResponse,
    Implementation,
    ListSessionsResponse,
    LoadSessionResponse,
    McpCapabilities,
    NewSessionResponse,
    SessionCapabilities,
    SessionCloseCapabilities,
)

from mcp_client import JaegerMCPClient
from shared.ws_commands import client_reader_to_ws, ws_to_client_writer
from transcript import Transcript

logger = logging.getLogger(__name__)

OPENROUTER_URL = "https://openrouter.ai/api/v1/chat/completions"

# Deliberately thin. It says how to behave, not what to investigate: the
# investigation procedure lives in the skills, which the model reaches through
# the MCP server's own instructions (appended below at session start). Encoding
# skill knowledge here would couple the sidecar to a particular set of skills
# and mask which of them the model can actually follow on its own.
BASE_SYSTEM_PROMPT = (
    "You are Jaeger AI, an assistant for distributed tracing investigations. "
    "Telemetry arrives via MCP tool results; treat it as your only source of truth. "
    "When evidence is needed, call the tool rather than answering from assumptions. "
    "Do not invent telemetry. If a result is empty, say so."
)

# A turn that has not converged after this many model round trips is not going
# to. Each iteration is one billable request, so the ceiling is also a cost guard.
#
# 25 rather than something tighter because models differ in how much they batch:
# one that emits a single tool call per round trip spends a dozen round trips on
# the discovery steps alone (read the skills index, open the skill, list
# services, search traces) before it starts analysing. A ceiling low enough to
# cut those off would penalise batching style rather than investigative ability.
# Tunable, since the right budget is model-dependent.
MAX_ITERATIONS = int(os.environ.get("MAX_ITERATIONS", "25"))


class SessionState:
    """Everything one ACP session owns.

    OpenRouter is stateless, so the full message history is resent on every
    completion call. A side benefit is that the request body is then the whole
    of what the model was shown, so token accounting needs no separate
    instrumentation.
    """

    def __init__(self, mcp: JaegerMCPClient):
        self.mcp = mcp
        self.messages: list[dict[str, Any]] = []
        # Tools are listed once per session and held here. Not process-wide:
        # the gateway mints a turn-scoped MCP route per request and the skills
        # behind it can change between sessions.
        self.tools: list[dict[str, Any]] = []
        self.started = False


class JaegerOpenRouterAgent(Agent):
    def __init__(
        self,
        model: str,
        api_key: str,
        fallback_mcp_url: str,
        transcript: Transcript,
        mcp_timeout_sec: float = 30.0,
        model_timeout_sec: float = 120.0,
    ):
        super().__init__()
        self._conn: Client | None = None
        self._model = model
        self._api_key = api_key
        self._fallback_mcp_url = fallback_mcp_url
        self._transcript = transcript
        self._mcp_timeout = mcp_timeout_sec
        self._model_timeout = model_timeout_sec
        self._next_session_id = 1
        self._sessions: dict[str, SessionState] = {}

    def on_connect(self, conn: Client) -> None:
        """Receive the ACP connection from the runtime once transport is up.

        The runtime supplies the connection here, not through the constructor —
        an agent built before the transport exists has nothing to send on.
        """
        self._conn = conn

    def _require_conn(self) -> Client:
        if self._conn is None:
            raise RuntimeError("ACP connection is not initialized")
        return self._conn

    async def initialize(
        self,
        protocol_version: int,
        client_capabilities: Any = None,
        client_info: Any = None,
        **kwargs: Any,
    ) -> InitializeResponse:
        if protocol_version != PROTOCOL_VERSION:
            raise ValueError(
                f"Unsupported ACP protocol version: {protocol_version}. "
                f"Supported version: {PROTOCOL_VERSION}."
            )
        logger.info("Agent initialized with protocol version %s", protocol_version)
        return InitializeResponse(
            protocol_version=PROTOCOL_VERSION,
            agent_capabilities=AgentCapabilities(
                session_capabilities=SessionCapabilities(close=SessionCloseCapabilities()),
                # The gateway gates its session/new MCP announcement on this
                # flag (see announceMCP in endpoint_chat.go): an agent that does
                # not advertise streamable HTTP is handed an empty mcpServers
                # list and never learns the turn-scoped URL. Without it the
                # sidecar falls back to a static env URL, which serves the
                # telemetry tools but not this turn's UI tools — and, in a
                # deployment where the fallback URL is wrong, no tools at all.
                mcp_capabilities=McpCapabilities(http=True),
            ),
            agent_info=Implementation(
                name="jaeger-openrouter-sidecar", title="Jaeger AI", version="0.1.0"
            ),
        )

    def _resolve_mcp(self, mcp_servers: Any) -> JaegerMCPClient:
        """Pick the MCP endpoint for a session.

        Prefer whatever the gateway announced in session/new: that URL is scoped
        to this turn and carries both the telemetry tools and the browser's UI
        tools. The env-configured URL is only a fallback for running the sidecar
        against a bare jaegermcp with no gateway in front of it.
        """
        for server in mcp_servers or []:
            url = getattr(server, "url", None)
            if not url:
                continue  # stdio/acp variants carry no URL; we only speak HTTP
            headers = {h.name: h.value for h in (getattr(server, "headers", None) or [])}
            logger.info("Using gateway-announced MCP server %r at %s", getattr(server, "name", "?"), url)
            return JaegerMCPClient(url, headers=headers, timeout_sec=self._mcp_timeout)

        logger.info("No MCP server announced; falling back to %s", self._fallback_mcp_url)
        return JaegerMCPClient(self._fallback_mcp_url, timeout_sec=self._mcp_timeout)

    async def new_session(
        self,
        cwd: str,
        additional_directories: list[str] | None = None,
        mcp_servers: Any = None,
        **kwargs: Any,
    ) -> NewSessionResponse:
        session_id = f"sess-{self._next_session_id}"
        self._next_session_id += 1
        self._sessions[session_id] = SessionState(self._resolve_mcp(mcp_servers))
        logger.info("Opened session %s", session_id)
        return NewSessionResponse(session_id=session_id)

    async def close_session(self, session_id: str, **kwargs: Any) -> CloseSessionResponse:
        state = self._sessions.pop(session_id, None)
        if state is not None:
            await state.mcp.close()
        logger.info("Closed session %s", session_id)
        return CloseSessionResponse()

    async def load_session(
        self,
        cwd: str,
        session_id: str,
        mcp_servers: Any = None,
        additional_directories: list[str] | None = None,
        **kwargs: Any,
    ) -> LoadSessionResponse | None:
        """Session restoration; the gateway opens a fresh session per request."""
        return LoadSessionResponse()

    async def list_sessions(
        self,
        cwd: str | None = None,
        cursor: str | None = None,
        **kwargs: Any,
    ) -> ListSessionsResponse:
        """Nothing is persisted, so there is never anything to enumerate."""
        return ListSessionsResponse(sessions=[])

    async def close_all(self) -> None:
        """Drop every session's MCP connection. Called on SIGINT/SIGTERM."""
        for session_id, state in list(self._sessions.items()):
            try:
                await state.mcp.close()
            except Exception:
                logger.exception("Failed closing MCP client for session %s", session_id)
        self._sessions.clear()

    async def _start_session(self, state: SessionState, session_id: str) -> None:
        """Connect MCP and seed the message history. Runs once per session."""
        if state.started:
            return

        await state.mcp.connect()
        state.tools = state.mcp.openai_tools()

        # THE MCP SERVER'S INSTRUCTIONS ARE PART OF THE MODEL'S CONTEXT.
        # jaegermcp uses the initialize result's `instructions` field to explain
        # how to enter the skills catalog. A sidecar that lists tools but drops
        # this text leaves the model holding tool names with no idea that a
        # skills index exists — which looks exactly like a model that cannot
        # follow skills. Relayed verbatim: it is the server talking to the
        # model, and paraphrasing it here would be a repair.
        system = BASE_SYSTEM_PROMPT
        instructions = state.mcp.instructions
        if instructions:
            system = f"{system}\n\n# Instructions from the Jaeger MCP server\n\n{instructions}"
            logger.info("Session %s: relayed %d chars of MCP server instructions", session_id, len(instructions))
        else:
            logger.warning(
                "Session %s: MCP server at %s supplied no instructions; "
                "the model will only see tool names",
                session_id,
                state.mcp.url,
            )

        state.messages = [{"role": "system", "content": system}]
        state.started = True

        self._transcript.emit(
            session_id,
            "session_started",
            mcp_url=state.mcp.url,
            tool_names=[t["function"]["name"] for t in state.tools],
            has_instructions=bool(instructions),
            instructions_chars=len(instructions or ""),
        )

    async def _call_tool(self, session_id: str, state: SessionState, name: str, args: dict[str, Any], call_id: str) -> str:
        conn = self._require_conn()
        await conn.session_update(
            session_id,
            start_tool_call(call_id, name, kind="search", status="in_progress", raw_input=args),
        )

        # A failing tool is data the model must see and reason about, so the
        # error text goes back into the conversation rather than aborting the
        # turn. This is not a repair: nothing is retried and nothing is fixed.
        try:
            output = await state.mcp.call_tool(name, args)
            failed = False
        except Exception as exc:
            output = json.dumps({"error": str(exc)})
            failed = True
            logger.exception("MCP tool %s failed in session %s", name, session_id)

        await conn.session_update(
            session_id,
            update_tool_call(
                call_id,
                status="failed" if failed else "completed",
                content=[tool_content(text_block(output))],
                # Without raw_output the gateway's AG-UI stream carries the call
                # but not what it returned, so the browser shows an empty result.
                raw_output={"content": output},
            ),
        )

        logger.info(
            "session=%s tool=%s call_id=%s status=%s result_bytes=%d",
            session_id, name, call_id, "failed" if failed else "completed", len(output),
        )
        self._transcript.emit(
            session_id, "tool_executed",
            tool=name, call_id=call_id, args=args,
            status="failed" if failed else "completed",
            result_bytes=len(output),
        )
        return output

    async def _chat(
        self, client: httpx2.AsyncClient, messages: list[dict[str, Any]], tools: list[dict[str, Any]]
    ) -> dict[str, Any]:
        """One completion call. No retry — see the design intent at the top."""
        resp = await client.post(
            OPENROUTER_URL,
            headers={
                "Authorization": f"Bearer {self._api_key}",
                "Content-Type": "application/json",
                # OpenRouter attributes requests to a referring app; harmless
                # to send and it keeps the request off the anonymous pool.
                "HTTP-Referer": "https://github.com/jaegertracing/jaeger",
                "X-Title": "Jaeger AI Sidecar",
            },
            json={
                "model": self._model,
                "messages": messages,
                "tools": tools,
                "tool_choice": "auto",
            },
        )
        resp.raise_for_status()
        return resp.json()

    async def _run_loop(self, session_id: str, state: SessionState, user_text: str) -> str:
        await self._start_session(state, session_id)
        state.messages.append({"role": "user", "content": user_text})

        async with httpx2.AsyncClient(timeout=self._model_timeout) as client:
            for step in range(MAX_ITERATIONS):
                body = await self._chat(client, state.messages, state.tools)
                choice = body["choices"][0]["message"]
                calls = choice.get("tool_calls") or []

                self._transcript.emit(
                    session_id, "completion",
                    step=step + 1, model=self._model,
                    # Verbatim: this block is the thing being measured.
                    tool_calls=calls,
                    finish_reason=body["choices"][0].get("finish_reason"),
                    usage=body.get("usage"),
                )

                if not calls:
                    return choice.get("content") or ""

                state.messages.append(choice)
                logger.info(
                    "session=%s step=%d tool_calls=%d names=%s",
                    session_id, step + 1, len(calls), [c["function"]["name"] for c in calls],
                )

                for call in calls:
                    fn = call["function"]
                    # json.loads raises on malformed arguments, and that is the
                    # point: a model that emits broken tool-call JSON must show
                    # up as a failure, not be quietly patched into success.
                    args = json.loads(fn.get("arguments") or "{}")
                    output = await self._call_tool(session_id, state, fn["name"], args, call["id"])
                    state.messages.append(
                        {"role": "tool", "tool_call_id": call["id"], "content": output}
                    )

        # Silent truncation is repair. A turn that hits the ceiling has to fail
        # loudly, or a partial answer gets mistaken for a finished one.
        raise RuntimeError(
            f"Turn did not converge within {MAX_ITERATIONS} model round trips"
        )

    async def prompt(
        self,
        session_id: str,
        prompt: list[Any],
        **kwargs: Any,
    ) -> PromptResponse:
        user_text = "".join(getattr(b, "text", "") for b in prompt)
        state = self._sessions.get(session_id)
        if state is None:
            # A prompt for a session we never opened; give it one rather than
            # dropping the turn on the floor.
            state = SessionState(self._resolve_mcp(None))
            self._sessions[session_id] = state

        logger.info("session=%s prompt_chars=%d", session_id, len(user_text))
        self._transcript.emit(session_id, "prompt_received", model=self._model, prompt_chars=len(user_text))

        try:
            answer = await self._run_loop(session_id, state, user_text)
            if answer:
                await self._require_conn().session_update(
                    session_id, update_agent_message(text_block(answer))
                )
            self._transcript.emit(session_id, "turn_ended", status="ok", answer_chars=len(answer))
        except asyncio.CancelledError:
            logger.warning("session=%s turn cancelled", session_id)
            self._transcript.emit(session_id, "turn_ended", status="cancelled")
            raise
        except Exception as exc:
            # Surface before the turn dies: a turn that ends silently is
            # indistinguishable from one that produced no findings.
            logger.exception("session=%s turn failed", session_id)
            self._transcript.emit(
                session_id, "turn_ended", status="error", error=f"{type(exc).__name__}: {exc}"
            )
            await self._require_conn().session_update(
                session_id, update_agent_message(text_block(f"\n[Error: {exc}]"))
            )

        return PromptResponse(stop_reason="end_turn")


async def handle_websocket(websocket: Any, agent_factory: Callable[[], Agent]) -> None:
    """Bridge the gateway's WebSocket to ACP's stdio framing via a socketpair.

    Lifted from the Gemini sidecar: reimplementing ACP framing here would be the
    one part of a "thin" sidecar that is not thin.
    """
    logger.info("New websocket connection from Jaeger AI Gateway")
    asock, csock = socket.socketpair()
    tasks: list[asyncio.Task[Any]] = []
    agent_writer = client_writer = None

    try:
        agent_reader, agent_writer = await asyncio.open_connection(sock=asock)
        client_reader, client_writer = await asyncio.open_connection(sock=csock)

        agent_task = asyncio.create_task(
            # session/close lives in ACP's unstable protocol. Without this flag
            # the route is not registered and the gateway's close fails with
            # "Method not found" at the end of every turn.
            run_agent(agent_factory(), agent_writer, agent_reader, use_unstable_protocol=True),
            name="agent_task",
        )
        tasks = [
            agent_task,
            asyncio.create_task(ws_to_client_writer(websocket, client_writer), name="ws_read"),
            asyncio.create_task(client_reader_to_ws(websocket, client_reader), name="ws_write"),
        ]
        done, pending = await asyncio.wait(tasks, return_when=asyncio.FIRST_COMPLETED)
        for task in done:
            if not task.cancelled() and task.exception():
                logger.error("Task %s failed: %s", task.get_name(), task.exception())
        for task in pending:
            task.cancel()
        if pending:
            await asyncio.gather(*pending, return_exceptions=True)
    finally:
        for writer in (client_writer, agent_writer):
            if writer is not None:
                writer.close()
        for sock in (asock, csock):
            try:
                sock.close()
            except OSError:
                pass
        logger.info("Websocket connection closed")
