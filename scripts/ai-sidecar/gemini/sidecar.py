# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

import asyncio
import logging
import socket
from typing import Any, Callable, cast

from google import genai
from google.genai import types
from opentelemetry.semconv._incubating.attributes.gen_ai_attributes import (
    GEN_AI_CONVERSATION_ID,
    GEN_AI_REQUEST_MODEL,
    GEN_AI_TOOL_CALL_ID,
    GEN_AI_TOOL_NAME,
)
from opentelemetry.trace import Status, StatusCode
from ws_commands import ws_to_client_writer, client_reader_to_ws
from gateway_mcp_client import GatewayMCPClient
from sidecar_config import SidecarConfig
from sidecar_helpers import (
    _to_tool_text,
    _validate_function_call,
)
from tracing import tracer

from acp import (
    PROTOCOL_VERSION,
    Agent,
    InitializeResponse,
    PromptResponse,
    run_agent,
    text_block,
    update_agent_message,
)
from acp.interfaces import Client
from acp.schema import (
    AgentCapabilities,
    CloseSessionResponse,
    Implementation,
    ListSessionsResponse,
    LoadSessionResponse,
    McpCapabilities,
    NewSessionResponse,
)

logger = logging.getLogger(__name__)


class JaegerSidecarAgent(Agent):
    """ACP agent that consumes the Jaeger AI gateway's MCP endpoint as
    its single tool egress.

    Architecture (post-IoC migration):
        - initialize advertises mcpCapabilities.http=true so the gateway
          announces its per-turn MCP URL via NewSessionRequest.mcpServers.
        - new_session stores the announced URL keyed by session_id and
          returns immediately — no network I/O against the gateway.
        - prompt opens a per-session GatewayMCPClient (lazy), runs the
          agentic loop, then closes the client in finally.
        - All tool dispatch — UI tools and telemetry tools alike — flows
          through MCP. The sidecar no longer distinguishes the two; the
          gateway routes by tool name. The gateway emits TOOL_CALL_*
          SSE for the browser, so the sidecar does NOT emit
          session_update notifications for tool calls.
    """

    def __init__(self, config: SidecarConfig):
        super().__init__()
        config.validate()
        self._conn: Client | None = None
        self._gemini = genai.Client(api_key=config.gemini_api_key)
        self._discovery_timeout_sec = config.mcp_discovery_timeout_sec
        self._next_session_id = 1
        self._next_tool_call_id = 1
        # Per-session gateway MCP URL pulled from NewSessionRequest.mcpServers.
        # Stays nil until new_session ran; prompt() builds the per-turn
        # GatewayMCPClient lazily on first use.
        self._mcp_url_by_session: dict[str, str] = {}

    def _new_tool_call_id(self, tool_name: str) -> str:
        call_id = f"{tool_name}-{self._next_tool_call_id}"
        self._next_tool_call_id += 1
        return call_id

    def on_connect(self, conn: Client) -> None:
        """Receive ACP connection object from runtime."""
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
        """Handle ACP initialize handshake.

        Advertises mcp_capabilities.http = True so the gateway announces
        an HTTP MCP server in NewSessionRequest.mcpServers — that's the
        URL this sidecar dials for every tool call.
        """
        if protocol_version != PROTOCOL_VERSION:
            raise ValueError(
                f"Unsupported ACP protocol version: {protocol_version}. "
                f"Supported version: {PROTOCOL_VERSION}."
            )
        logger.info("Agent initialized with protocol version %s", protocol_version)
        # session_capabilities.close is omitted because the
        # agent-client-protocol Python SDK marks session/close as
        # unstable and rejects it with Method-not-found unless
        # use_unstable_protocol is enabled. Not advertising the
        # capability keeps the gateway from ever calling it.
        return InitializeResponse(
            protocol_version=PROTOCOL_VERSION,
            agent_capabilities=AgentCapabilities(
                mcp_capabilities=McpCapabilities(http=True),
            ),
            agent_info=Implementation(name="jaeger-gemini-sidecar", title="Jaeger AI", version="0.1.0"),
        )

    async def new_session(
        self,
        cwd: str,
        additional_directories: list[str] | None = None,
        mcp_servers: Any = None,
        **kwargs: Any,
    ) -> NewSessionResponse:
        """Handle ACP `session/new` RPC.

        Pulls the HTTP MCP URL out of ``mcp_servers`` and stashes it
        per-session so prompt() can spin up a GatewayMCPClient on
        demand. No network I/O happens here — the gateway hasn't yet
        registered the session in its uuid→session map (that happens
        once this RPC returns), and dialing now would race the
        gateway's registration step.
        """
        session_id = f"sess-{self._next_session_id}"
        self._next_session_id += 1

        url = _http_mcp_url_from(mcp_servers)
        if url:
            self._mcp_url_by_session[session_id] = url
            logger.info("Registered gateway MCP URL for session %s: %s", session_id, url)
        else:
            logger.warning(
                "session/new for %s did not include an HTTP MCP server; "
                "tool calls will fail until the gateway announces one",
                session_id,
            )

        return NewSessionResponse(session_id=session_id)

    async def close_session(self, session_id: str, **kwargs: Any) -> CloseSessionResponse:
        """Handle ACP `session/close` RPC. Drops the cached MCP URL."""
        self._mcp_url_by_session.pop(session_id, None)
        logger.info("Closed session %s", session_id)
        return CloseSessionResponse()

    async def load_session(
        self,
        cwd: str,
        session_id: str,
        additional_directories: list[str] | None = None,
        mcp_servers: Any = None,
        **kwargs: Any,
    ) -> LoadSessionResponse | None:
        return LoadSessionResponse()

    async def list_sessions(
        self,
        additional_directories: list[str] | None = None,
        cursor: str | None = None,
        cwd: str | None = None,
        **kwargs: Any,
    ) -> ListSessionsResponse:
        return ListSessionsResponse(sessions=[])

    async def _execute_tool(
        self,
        mcp_client: GatewayMCPClient,
        session_id: str,
        tool_name: str,
        args: dict[str, Any],
        tool_call_id: str,
    ) -> Any:
        """Dispatch a tool call through the gateway's MCP endpoint.

        Note: the sidecar deliberately does NOT emit start_tool_call /
        update_tool_call session_update notifications around the
        forward. Post-IoC migration, the gateway emits the full
        TOOL_CALL_START/ARGS/RESULT/END SSE lifecycle directly from
        its MCP proxy, and a parallel sidecar emission would produce
        duplicate events on the browser stream.
        """
        with tracer().start_as_current_span("sidecar.execute_tool", attributes={
            GEN_AI_TOOL_NAME: tool_name,
            GEN_AI_TOOL_CALL_ID: tool_call_id,
            GEN_AI_CONVERSATION_ID: session_id,
        }) as span:
            try:
                _validate_function_call(tool_name, args, tool_call_id)
                return await mcp_client.call_tool(tool_name, args)
            except Exception as e:
                span.record_exception(e)
                span.set_status(Status(StatusCode.ERROR, description=str(e)))
                raise

    async def _run_agentic_gemini_loop(
        self, mcp_client: GatewayMCPClient, session_id: str, user_text: str,
    ) -> str:
        with tracer().start_as_current_span("sidecar.agentic_loop", attributes={
            GEN_AI_CONVERSATION_ID: session_id,
            GEN_AI_REQUEST_MODEL: "gemini-2.5-flash",
        }):
            logger.info("Starting agentic Gemini loop for session %s", session_id)
            system_instruction = (
                "You are Jaeger AI, an assistant for distributed tracing investigations. "
                "You will be given telemetry information from MCP tool results; treat that data as your source of truth. "
                "When telemetry evidence is needed, request the MCP tool instead of answering from assumptions. "
                "Before each MCP tool call, briefly state what you are querying and why. "
                "Do not invent telemetry data. If the tool result is empty, say so clearly and suggest how to narrow or "
                "expand the query (service name, operation name, tags, and time range). "
                "After tool calls, provide a concise answer with: findings, probable cause, and next debugging steps."
            )

            tools_for_gemini = await mcp_client.get_gemini_tools()
            tool_names: set[str] = set()
            for tool in tools_for_gemini:
                if tool.function_declarations:
                    tool_names.update(fd.name for fd in tool.function_declarations if fd.name)

            logger.info("Passing %d tool(s) to Gemini: %s", len(tool_names), sorted(tool_names))

            chat = self._gemini.chats.create(
                model="gemini-2.5-flash",
                config=types.GenerateContentConfig(
                    system_instruction=system_instruction,
                    tools=cast(Any, tools_for_gemini),
                    automatic_function_calling=types.AutomaticFunctionCallingConfig(disable=True),
                ),
            )

            logger.info("Sending user message to Gemini")
            response = await asyncio.to_thread(chat.send_message, user_text)

            # Iterate model->tool->model until Gemini produces a final text response.
            while True:
                function_calls = response.function_calls
                if not function_calls:
                    logger.info("No function calls in Gemini response, returning final text")
                    return response.text or ""

                function_responses = []
                for function_call in function_calls:
                    name = function_call.name or ""
                    args = function_call.args or dict[str, Any]()
                    call_id = function_call.id or self._new_tool_call_id(name or "unnamed")
                    logger.info("Gemini requested tool call: %s (call_id=%s)", name, call_id)
                    tool_output = await self._execute_tool(mcp_client, session_id, name, args, call_id)
                    tool_text = _to_tool_text(tool_output)
                    function_responses.append(
                        types.Part.from_function_response(name=name, response={"result": tool_text})
                    )

                logger.debug("Sending function responses back to Gemini")
                response = await asyncio.to_thread(chat.send_message, function_responses)
                logger.info("Received Gemini response after tool calls")

    async def prompt(
        self,
        prompt: list[Any],
        session_id: str,
        message_id: str | None = None,
        **kwargs: Any,
    ) -> PromptResponse:
        """Handle ACP `session/prompt` RPC."""
        with tracer().start_as_current_span("sidecar.prompt", attributes={
            GEN_AI_CONVERSATION_ID: session_id,
        }) as span:
            logger.info("Received prompt request for session %s", session_id)

            user_text = ""
            for block in prompt:
                if hasattr(block, "text"):
                    user_text += block.text

            url = self._mcp_url_by_session.get(session_id)
            if not url:
                # Stricter than a warning: without an announced MCP URL
                # we have no way to fetch tools or dispatch tool calls,
                # so the loop would either crash or silently produce
                # incomplete answers. Surface a clean error to the
                # browser instead.
                conn = self._require_conn()
                await conn.session_update(
                    session_id,
                    update_agent_message(
                        text_block(
                            "[Error: gateway did not announce an MCP server URL for this session]"
                        )
                    ),
                )
                return PromptResponse(stop_reason="end_turn")

            mcp_client = GatewayMCPClient(url, self._discovery_timeout_sec)
            try:
                conn = self._require_conn()
                final_answer = await self._run_agentic_gemini_loop(mcp_client, session_id, user_text)
                if final_answer:
                    logger.info("Sending final answer for session %s", session_id)
                    await conn.session_update(
                        session_id,
                        update_agent_message(text_block(final_answer)),
                    )
            except asyncio.CancelledError:
                span.set_status(Status(StatusCode.ERROR, description="cancelled"))
                logger.warning(
                    f"Prompt handling cancelled for session {session_id} "
                    "(connection/task terminated before response completed)."
                )
                raise
            except Exception as e:
                span.record_exception(e)
                span.set_status(Status(StatusCode.ERROR, description=str(e)))
                logger.exception("Error calling Gemini: %s", e)
                conn = self._require_conn()
                await conn.session_update(
                    session_id,
                    update_agent_message(text_block(f"\n[Error: {str(e)}]"))
                )
            finally:
                await mcp_client.aclose()

            return PromptResponse(stop_reason="end_turn")


def _http_mcp_url_from(mcp_servers: Any) -> str | None:
    """Find the URL of the first HTTP-typed MCP server entry in the
    session/new request. Returns None when the list is missing,
    malformed, or only contains non-HTTP transports (e.g. ACP-only).

    The Python ACP runtime hands ``mcp_servers`` to handlers as either
    a list of schema objects (when the version is recent enough to
    type-check) or a list of dicts (when older / hand-built); accept
    both shapes since the sidecar runs against a few SDK builds.
    """
    if not isinstance(mcp_servers, list):
        return None
    for server in mcp_servers:
        # Schema-object case: pydantic model with `.url` and `.type`.
        url = getattr(server, "url", None)
        kind = getattr(server, "type", None)
        if isinstance(url, str) and (kind is None or kind == "http"):
            return url
        # Dict case: literal {"type": "http", "url": "..."}.
        if isinstance(server, dict):
            if server.get("type") == "http":
                value = server.get("url")
                if isinstance(value, str) and value:
                    return value
    return None


async def handle_websocket(websocket: Any, agent_factory: Callable[[], Agent] | None = None) -> None:
    logger.info("New websocket connection from Jaeger AI Gateway")

    # Bridge ACP stdio-style streams to WebSocket transport used by the Go gateway.
    # Socketpair avoids reimplementing ACP framing logic in this process.

    asock, csock = socket.socketpair()
    agent_writer = None
    client_writer = None
    tasks: list[asyncio.Task[Any]] = []

    try:
        agent_reader, agent_writer = await asyncio.open_connection(sock=asock)
        client_reader, client_writer = await asyncio.open_connection(sock=csock)

        if agent_factory is None:
            raise RuntimeError("agent_factory must be provided by the application entrypoint")

        agent = agent_factory()
        agent_task = asyncio.create_task(run_agent(agent, agent_writer, agent_reader), name="agent_task")

        ws_read_task = asyncio.create_task(ws_to_client_writer(websocket, client_writer), name="ws_read_task")
        ws_write_task = asyncio.create_task(client_reader_to_ws(websocket, client_reader), name="ws_write_task")
        tasks = [agent_task, ws_read_task, ws_write_task]

        done, pending = await asyncio.wait(
            tasks,
            return_when=asyncio.FIRST_COMPLETED,
        )

        for task in done:
            logger.info("Task finished: %s", task.get_name())
            if task.cancelled():
                logger.info("Task was cancelled: %s", task.get_name())
                continue
            task_exc = task.exception()
            if task_exc:
                logger.error("Task exception (%s): %s", task.get_name(), task_exc)
            else:
                logger.info("Task completed normally: %s", task.get_name())

        for task in pending:
            task.cancel()
        if pending:
            await asyncio.gather(*pending, return_exceptions=True)
    finally:
        for writer in (client_writer, agent_writer):
            if writer is None:
                continue
            writer.close()

        for writer in (client_writer, agent_writer):
            if writer is None:
                continue
            try:
                await writer.wait_closed()
            except Exception:
                pass

        for sock in (asock, csock):
            try:
                sock.close()
            except OSError:
                pass

        lingering = [task for task in tasks if not task.done()]
        if lingering:
            logger.info(
                "Cancelling lingering tasks during websocket shutdown: %s",
                [task.get_name() for task in lingering],
            )
        for task in lingering:
            task.cancel()
        if lingering:
            await asyncio.gather(*lingering, return_exceptions=True)

        logger.info("Websocket connection closed")
