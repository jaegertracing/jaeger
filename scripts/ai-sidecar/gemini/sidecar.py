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
from mcp_bridge import JaegerMCPBridge
from sidecar_config import SidecarConfig
from sidecar_helpers import (
    _build_gemini_contextual_tool,
    _extract_contextual_tools,
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
from acp.helpers import start_tool_call, tool_content, update_tool_call
from acp.interfaces import Client
from acp.schema import (
    AgentCapabilities,
    CloseSessionResponse,
    Implementation,
    ListSessionsResponse,
    LoadSessionResponse,
    NewSessionResponse,
    SessionCapabilities,
    SessionCloseCapabilities,
)

logger = logging.getLogger(__name__)

# EXT_METHOD_JAEGER_TOOL_CALL is the ACP extension method the sidecar
# invokes when Gemini requests a contextual (frontend-supplied) tool. The
# Python ACP runtime prepends a single "_", so we drop the leading "_" we
# share with the Go side (Go const "_meta/jaegertracing.io/tools/call").
EXT_METHOD_JAEGER_TOOL_CALL = "meta/jaegertracing.io/tools/call"


class JaegerSidecarAgent(Agent):
    """ACP agent implementation that proxies trace-analysis requests to Gemini + MCP tools."""

    def __init__(self, config: SidecarConfig):
        super().__init__()
        config.validate()
        self._conn: Client | None = None
        self._client_caps: Any = None
        self._gemini = genai.Client(api_key=config.gemini_api_key)
        self._mcp = JaegerMCPBridge(config.mcp_url, config.mcp_discovery_timeout_sec)
        self._next_session_id = 1
        self._next_tool_call_id = 1
        # Per-session AG-UI tool snapshot pulled from NewSessionRequest._meta.
        # Each entry is the raw tool definition dict the frontend supplied
        # (shape: {name, description?, parameters?}). The agentic loop uses
        # the names to decide whether a Gemini function_call dispatches via
        # MCP (built-in) or via the ACP extension method (contextual).
        self._contextual_tools: dict[str, list[dict[str, Any]]] = {}

    def _new_tool_call_id(self, tool_name: str) -> str:
        call_id = f"{tool_name}-{self._next_tool_call_id}"
        self._next_tool_call_id += 1
        return call_id

    def on_connect(self, conn: Client) -> None:
        """Receive ACP connection object from runtime.

        Called by the ACP runtime when the agent is attached to an active
        transport so session updates can be sent back to the client.
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
        """Handle ACP initialize handshake.

        This method is invoked by the ACP runtime (via the `initialize` RPC),
        not called directly by our application code. It is required by the ACP
        protocol so the agent and client can negotiate protocol version and
        advertise capabilities before any session/new or session/prompt calls.
        """
        if protocol_version != PROTOCOL_VERSION:
            raise ValueError(
                f"Unsupported ACP protocol version: {protocol_version}. "
                f"Supported version: {PROTOCOL_VERSION}."
            )
        self._client_caps = client_capabilities
        logger.info("Agent initialized with protocol version %s", protocol_version)
        return InitializeResponse(
            protocol_version=PROTOCOL_VERSION,
            agent_capabilities=AgentCapabilities(
                session_capabilities=SessionCapabilities(close=SessionCloseCapabilities()),
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

        Invoked by ACP runtime dispatch (not direct app code) to allocate a new
        session id that the client will use for subsequent prompt calls.

        Reads the optional contextual tools snapshot the gateway attaches via
        NewSessionRequest._meta and stashes it per-session so the agentic loop
        can merge those tools into the Gemini chat config. The Python ACP
        router spreads ``_meta``'s inner keys into this handler's ``**kwargs``,
        so ``kwargs`` itself is the meta dict to look up the namespaced key in.
        """
        session_id = f"sess-{self._next_session_id}"
        self._next_session_id += 1

        contextual = _extract_contextual_tools(kwargs)
        if contextual:
            self._contextual_tools[session_id] = contextual
            logger.info(
                "Registered %d contextual tool(s) for session %s: %s",
                len(contextual),
                session_id,
                [t.get("name") for t in contextual],
            )

        return NewSessionResponse(session_id=session_id)

    async def close_session(self, session_id: str, **kwargs: Any) -> CloseSessionResponse:
        """Handle ACP `session/close` RPC.

        Invoked by the gateway when an HTTP chat request finishes (success,
        failure, or client disconnect mid-stream). Drops any per-session
        bookkeeping the agent holds. ``pop(..., None)`` is idempotent so
        sessions that never registered contextual tools — or that were
        already cleaned up by ``prompt``'s ``finally`` block — are safe to
        close again. Capability is advertised in ``initialize`` via
        ``SessionCapabilities.close``.
        """
        self._contextual_tools.pop(session_id, None)
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
        """Handle ACP `session/load` RPC.

        Called by the ACP runtime during session restoration flows.
        """
        return LoadSessionResponse()

    async def list_sessions(
        self,
        additional_directories: list[str] | None = None,
        cursor: str | None = None,
        cwd: str | None = None,
        **kwargs: Any,
    ) -> ListSessionsResponse:
        """Handle ACP `session/list` RPC.

        Called by ACP runtime to enumerate available sessions for the client.
        """
        return ListSessionsResponse(sessions=[])

    def _build_acp_tool_declarations(self) -> list[types.FunctionDeclaration]:
        """Bridge advertised ACP client capabilities into Gemini function declarations.

        Reads clientCapabilities from the initialize handshake and returns
        FunctionDeclaration objects for each capability the gateway advertised.
        No skills-specific logic — pure ACP capability bridging.
        """
        declarations: list[types.FunctionDeclaration] = []
        fs = getattr(self._client_caps, "fs", None)
        if fs and getattr(fs, "read_text_file", False):
            declarations.append(
                types.FunctionDeclaration(
                    name="fs_read_text_file",
                    description="Read a text file from the workspace.",
                    parameters_json_schema={
                        "type": "object",
                        "properties": {
                            "path": {
                                "type": "string",
                                "description": "Path to the file to read.",
                            }
                        },
                        "required": ["path"],
                    },
                )
            )
        return declarations

    async def _execute_acp_function_call(
        self,
        session_id: str,
        name: str,
        args: dict[str, Any],
        tool_call_id: str,
    ) -> str | None:
        """Execute an ACP function call locally via the ACP connection.

        Returns the result string, or None if this isn't a recognized ACP function
        (caller should route to MCP or contextual tools instead).
        """
        if name != "fs_read_text_file":
            return None

        with tracer().start_as_current_span("sidecar.execute_acp_call", attributes={
            GEN_AI_TOOL_NAME: name,
            GEN_AI_TOOL_CALL_ID: tool_call_id,
            GEN_AI_CONVERSATION_ID: session_id,
        }) as span:
            try:
                _validate_function_call(name, args, tool_call_id)
                conn = self._require_conn()
                await conn.session_update(
                    session_id,
                    start_tool_call(
                        tool_call_id,
                        name,
                        kind="search",
                        status="in_progress",
                    ),
                )

                path = args.get("path", "")
                result = await conn.read_text_file(path=path, session_id=session_id)
                output_text = result.content

                await conn.session_update(
                    session_id,
                    update_tool_call(
                        tool_call_id,
                        status="completed",
                        content=[tool_content(text_block(output_text))],
                        raw_output={"content": output_text},
                    ),
                )

                return output_text
            except Exception as e:
                span.record_exception(e)
                span.set_status(Status(StatusCode.ERROR, description=str(e)))
                logger.warning("ACP call %s failed for session %s: %s", name, session_id, e)
                return f"error: {e}"

    async def _execute_contextual_tool(
        self,
        session_id: str,
        tool_name: str,
        args: dict[str, Any],
        tool_call_id: str,
    ) -> Any:
        """Dispatch a contextual (frontend-supplied) tool call back to the
        gateway via the ACP extension method, fire-and-forget.

        Two surfaces are involved and must not be conflated:

        1. **AG-UI wire (browser):** the parallel ``session_update``
           notifications below become ``TOOL_CALL_START`` /
           ``TOOL_CALL_ARGS`` / ``TOOL_CALL_END`` SSE events. The browser
           matches the tool name to its registered AG-UI tool and runs
           ``execute(args)`` locally — *that* is the actual execution.
           We deliberately do NOT populate ``raw_output`` / ``content`` on
           the completion update: doing so would cause the streaming
           client to emit ``TOOL_CALL_RESULT``, which tricks assistant-ui
           into thinking the server already produced a result and skips
           the local ``execute()``.

        2. **LLM loop (Gemini):** the ext_method returns the gateway's
           synthetic ``{"acknowledged": true}`` ack, which we feed back
           into the agentic loop as the function response so Gemini
           produces a final text answer. We never wait for the browser to
           confirm — that's the "forget" half of fire-and-forget.
        """
        with tracer().start_as_current_span("sidecar.execute_contextual_tool", attributes={
            GEN_AI_TOOL_NAME: tool_name,
            GEN_AI_TOOL_CALL_ID: tool_call_id,
            GEN_AI_CONVERSATION_ID: session_id,
        }) as span:
            try:
                _validate_function_call(tool_name, args, tool_call_id)
                conn = self._require_conn()
                # raw_input carries the LLM-generated arguments onto the
                # AG-UI wire as TOOL_CALL_ARGS so the browser knows what
                # to highlight / render / etc.
                await conn.session_update(
                    session_id,
                    start_tool_call(
                        tool_call_id,
                        tool_name,
                        kind="other",
                        status="in_progress",
                        raw_input=args,
                    ),
                )

                response = await conn.ext_method(
                    EXT_METHOD_JAEGER_TOOL_CALL,
                    {"sessionId": session_id, "name": tool_name, "args": args},
                )

                # Status=completed alone fires TOOL_CALL_END (no RESULT)
                # because raw_output is intentionally absent — see the
                # method docstring for why.
                await conn.session_update(
                    session_id,
                    update_tool_call(
                        tool_call_id,
                        status="completed",
                    ),
                )
                return response
            except Exception as e:
                span.record_exception(e)
                span.set_status(Status(StatusCode.ERROR, description=str(e)))
                raise

    async def _execute_tool(self, session_id: str, tool_name: str, args: dict[str, Any], tool_call_id: str) -> Any:
        with tracer().start_as_current_span("sidecar.execute_tool", attributes={
            GEN_AI_TOOL_NAME: tool_name,
            GEN_AI_TOOL_CALL_ID: tool_call_id,
            GEN_AI_CONVERSATION_ID: session_id,
        }) as span:
            try:
                _validate_function_call(tool_name, args, tool_call_id)
                conn = self._require_conn()
                await conn.session_update(
                    session_id,
                    start_tool_call(
                        tool_call_id,
                        tool_name,
                        kind="search",
                        status="in_progress",
                    ),
                )

                tool_output = await self._mcp.call_tool(tool_name, args)
                output_text = _to_tool_text(tool_output)

                await conn.session_update(
                    session_id,
                    update_tool_call(
                        tool_call_id,
                        status="completed",
                        content=[tool_content(text_block(output_text))],
                        raw_output={"content": tool_output},
                    ),
                )

                return tool_output
            except Exception as e:
                span.record_exception(e)
                span.set_status(Status(StatusCode.ERROR, description=str(e)))
                raise

    async def _run_agentic_gemini_loop(self, session_id: str, user_text: str) -> str:
        with tracer().start_as_current_span("sidecar.agentic_loop", attributes={
            GEN_AI_CONVERSATION_ID: session_id,
            GEN_AI_REQUEST_MODEL: "gemini-2.5-flash",
        }):
            logger.info("Starting agentic Gemini loop for session %s", session_id)
            base_instruction = (
                "You are Jaeger AI, an assistant for distributed tracing investigations. "
                "You will be given telemetry information from MCP tool results; treat that data as your source of truth. "
                "When telemetry evidence is needed, request the MCP tool instead of answering from assumptions. "
                "Before each MCP tool call, briefly state what you are querying and why. "
                "Do not invent telemetry data. If the tool result is empty, say so clearly and suggest how to narrow or "
                "expand the query (service name, operation name, tags, and time range). "
                "After tool calls, provide a concise answer with: findings, probable cause, and next debugging steps."
            )

            mcp_instructions = await self._mcp.get_server_instructions()
            if mcp_instructions:
                system_instruction = base_instruction + "\n\n" + mcp_instructions
            else:
                system_instruction = base_instruction

            mcp_tools = await self._mcp.get_gemini_tools()
            mcp_tool_names: set[str] = set()
            for tool in mcp_tools:
                if tool.function_declarations:
                    mcp_tool_names.update(fd.name for fd in tool.function_declarations if fd.name)

            contextual_tools = self._contextual_tools.get(session_id, [])
            contextual_tool_names = {t["name"] for t in contextual_tools if t.get("name")}
            contextual_gemini_tool = _build_gemini_contextual_tool(contextual_tools)

            acp_declarations = self._build_acp_tool_declarations()
            acp_tool_names = {d.name for d in acp_declarations if d.name}

            tools_for_gemini: list[Any] = list(mcp_tools)
            if acp_declarations:
                tools_for_gemini.append(types.Tool(function_declarations=acp_declarations))
            if contextual_gemini_tool is not None:
                tools_for_gemini.append(contextual_gemini_tool)

            logger.info(
                "Passing tools to Gemini: mcp=%s acp=%s contextual=%s",
                sorted(mcp_tool_names),
                sorted(acp_tool_names),
                sorted(contextual_tool_names),
            )

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
                    if name in acp_tool_names:
                        logger.info("Gemini requested ACP call: %s (call_id=%s)", name, call_id)
                        tool_output = await self._execute_acp_function_call(session_id, name, args, call_id)
                    elif name in contextual_tool_names:
                        logger.info("Gemini requested contextual tool call: %s (call_id=%s)", name, call_id)
                        tool_output = await self._execute_contextual_tool(session_id, name, args, call_id)
                    else:
                        logger.info("Gemini requested MCP tool call: %s (call_id=%s)", name, call_id)
                        tool_output = await self._execute_tool(session_id, name, args, call_id)
                    function_responses.append(
                        types.Part.from_function_response(name=name, response={"result": tool_output})
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
        """Handle ACP `session/prompt` RPC.

        Invoked by ACP runtime dispatch after initialize/session handshake; this
        is the protocol entrypoint for prompt execution.
        """
        with tracer().start_as_current_span("sidecar.prompt", attributes={
            GEN_AI_CONVERSATION_ID: session_id,
        }) as span:
            logger.info("Received prompt request for session %s", session_id)

            # Extract text from prompt blocks
            user_text = ""
            for block in prompt:
                if hasattr(block, "text"):
                    user_text += block.text

            try:
                conn = self._require_conn()
                final_answer = await self._run_agentic_gemini_loop(session_id, user_text)
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
                # Drop the per-session contextual tools snapshot now that
                # the prompt has finished. The Jaeger AI gateway opens one
                # ACP session per chat request and never reuses the
                # session_id, so without this cleanup the dict would grow
                # unbounded over the sidecar's lifetime. pop(..., None)
                # is idempotent — safe even if no entry exists for this
                # session (which is the common PR1 case).
                self._contextual_tools.pop(session_id, None)

            return PromptResponse(stop_reason="end_turn")


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

        # Start the ACP local agent linked to the agent ends of the socket pair
        if agent_factory is None:
            raise RuntimeError("agent_factory must be provided by the application entrypoint")

        agent = agent_factory()
        agent_task = asyncio.create_task(run_agent(agent, agent_writer, agent_reader), name="agent_task")

        # Bridge the client ends of the socket pair up to the WebSocket
        ws_read_task = asyncio.create_task(ws_to_client_writer(websocket, client_writer), name="ws_read_task")
        ws_write_task = asyncio.create_task(client_reader_to_ws(websocket, client_reader), name="ws_write_task")
        tasks = [agent_task, ws_read_task, ws_write_task]

        # Wait for the connection to end
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
        # Close stream writers first so transports can flush and release fds.
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

        # socketpair fds are handed to asyncio transports above; explicit close is
        # a safe fallback if they remain open due to early failures.
        for sock in (asock, csock):
            try:
                sock.close()
            except OSError:
                pass

        # If any task survived above due to unexpected failure, cancel+drain here.
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
