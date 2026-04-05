# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

import asyncio
import json
import logging
import socket
from dataclasses import dataclass
from typing import Any, Callable, cast

from google.adk.tools.mcp_tool import MCPToolset, StreamableHTTPConnectionParams
from google import genai
from google.genai import types
from ws_commands import ws_to_client_writer, client_reader_to_ws

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
    Implementation,
    ListSessionsResponse,
    LoadSessionResponse,
    NewSessionResponse,
)

END_OF_TURN_MARKER = "__END_OF_TURN__"
logger = logging.getLogger(__name__)


@dataclass(frozen=True)
class SidecarConfig:
    gemini_api_key: str
    mcp_url: str
    mcp_discovery_timeout_sec: float

    def validate(self) -> None:
        if not self.gemini_api_key:
            raise RuntimeError(
                "GEMINI_API_KEY must be provided via --gemini-api-key or environment variable"
            )
        if not self.mcp_url:
            raise RuntimeError("JAEGER_MCP_URL must be provided via --mcp-url or environment variable")
        if self.mcp_discovery_timeout_sec <= 0:
            raise RuntimeError("MCP discovery timeout must be > 0 seconds")


def _to_jsonable(value: Any) -> Any:
    if hasattr(value, "model_dump"):
        return value.model_dump()
    if hasattr(value, "dict"):
        return value.dict()
    return value


def _to_tool_text(value: Any) -> str:
    if isinstance(value, str):
        return value
    try:
        return json.dumps(_to_jsonable(value), ensure_ascii=False)
    except Exception:
        return str(value)


def _extract_function_declaration(tool: Any) -> types.FunctionDeclaration | None:
    """Return a Gemini function declaration from an ADK tool.

    Prefer a public API when available; fall back to ADK's internal method for
    compatibility with current tool implementations.
    """
    get_declaration = getattr(tool, "get_declaration", None)
    if callable(get_declaration):
        declaration = get_declaration()
        if declaration is not None:
            return cast(types.FunctionDeclaration, declaration)

    # ADK BaseTool currently exposes declaration via _get_declaration().
    # Keep this fallback isolated in one place to reduce breakage risk.
    private_get_declaration = getattr(tool, "_get_declaration", None)
    if callable(private_get_declaration):
        return cast(types.FunctionDeclaration, private_get_declaration())

    return None


class JaegerMCPBridge:
    """Loads MCP tools once and exposes them for Gemini tool-calling."""

    def __init__(self, mcp_url: str, timeout_sec: float):
        self._mcp_url = mcp_url
        self._timeout_sec = timeout_sec
        self._toolset = MCPToolset(
            connection_params=StreamableHTTPConnectionParams(url=mcp_url),
        )
        self._tools_by_name: dict[str, Any] = {}
        self._gemini_tools: list[types.Tool] = []
        self._initialized = False

    async def initialize(self) -> None:
        if self._initialized:
            return

        # Discover available MCP tools once, then expose them to Gemini as function declarations.
        logger.info(
            f"Initializing MCP tool discovery from {self._mcp_url} "
            f"(single attempt timeout={self._timeout_sec}s)"
        )

        try:
            logger.info(
                f"MCP connection trying {self._mcp_url} "
                f"(timeout {self._timeout_sec:.1f}s)"
            )
            adk_tools = await asyncio.wait_for(self._toolset.get_tools(), timeout=self._timeout_sec)
        except asyncio.CancelledError:
            logger.warning(
                "MCP tool discovery cancelled before completion "
                "(client likely disconnected before MCP became available)."
            )
            logger.warning("MCP was not connected for %s; request aborted.", self._mcp_url)
            raise
        except Exception as exc:
            message = (
                f"Unable to connect to MCP at {self._mcp_url} on first attempt. "
                "Stopping request."
            )
            logger.error("%s Error: %s", message, exc)
            raise RuntimeError(message) from exc

        self._tools_by_name = {tool.name: tool for tool in adk_tools}
        logger.info("Retrieved tools from MCP: %s", list(self._tools_by_name.keys()))

        function_declarations: list[types.FunctionDeclaration] = []
        for tool in adk_tools:
            declaration = _extract_function_declaration(tool)
            if declaration is not None:
                function_declarations.append(declaration)

        if function_declarations:
            self._gemini_tools = [types.Tool(function_declarations=function_declarations)]
        else:
            self._gemini_tools = []

        self._initialized = True

    async def get_gemini_tools(self) -> list[types.Tool]:
        await self.initialize()
        return self._gemini_tools

    async def call_tool(self, name: str, args: dict[str, Any]) -> Any:
        await self.initialize()

        tool = self._tools_by_name.get(name)
        if tool is None:
            return {"error": f"unsupported tool: {name}"}

        result = await tool.run_async(args=args or {}, tool_context=None)
        return _to_jsonable(result)

class JaegerSidecarAgent(Agent):
    """ACP agent implementation that proxies trace-analysis requests to Gemini + MCP tools."""

    def __init__(self, config: SidecarConfig):
        super().__init__()
        config.validate()
        self._conn: Client | None = None
        self._gemini = genai.Client(api_key=config.gemini_api_key)
        self._mcp = JaegerMCPBridge(config.mcp_url, config.mcp_discovery_timeout_sec)
        self._next_session_id = 1
        self._next_tool_call_id = 1

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
        logger.info("Agent initialized with protocol version %s", protocol_version)
        return InitializeResponse(
            protocol_version=PROTOCOL_VERSION,
            agent_capabilities=AgentCapabilities(),
            agent_info=Implementation(name="jaeger-gemini-sidecar", title="Jaeger AI", version="0.1.0"),
        )

    async def new_session(self, cwd: str, mcp_servers: Any = None, **kwargs: Any) -> NewSessionResponse:
        """Handle ACP `session/new` RPC.

        Invoked by ACP runtime dispatch (not direct app code) to allocate a new
        session id that the client will use for subsequent prompt calls.
        """
        session_id = f"sess-{self._next_session_id}"
        self._next_session_id += 1
        return NewSessionResponse(session_id=session_id)

    async def load_session(
        self,
        cwd: str,
        session_id: str,
        mcp_servers: Any = None,
        **kwargs: Any,
    ) -> LoadSessionResponse | None:
        """Handle ACP `session/load` RPC.

        Called by the ACP runtime during session restoration flows.
        """
        return LoadSessionResponse()

    async def list_sessions(
        self,
        cursor: str | None = None,
        cwd: str | None = None,
        **kwargs: Any,
    ) -> ListSessionsResponse:
        """Handle ACP `session/list` RPC.

        Called by ACP runtime to enumerate available sessions for the client.
        """
        return ListSessionsResponse(sessions=[])

    async def _execute_tool(self, session_id: str, tool_name: str, args: dict[str, Any], tool_call_id: str) -> Any:
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

    async def _run_agentic_gemini_loop(self, session_id: str, user_text: str) -> str:
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

        mcp_tools = await self._mcp.get_gemini_tools()
        tool_names: list[str] = []
        for tool in mcp_tools:
            if tool.function_declarations:
                tool_names.extend(fd.name for fd in tool.function_declarations if fd.name)
        logger.info("Passing tools to Gemini: %s", tool_names)

        chat = self._gemini.chats.create(
            model="gemini-2.5-flash",
            config=types.GenerateContentConfig(
                system_instruction=system_instruction,
                tools=cast(Any, mcp_tools),
                automatic_function_calling=types.AutomaticFunctionCallingConfig(disable=True),
            ),
        )

        logger.info("Sending user message to Gemini")
        response = await asyncio.to_thread(chat.send_message, user_text)

        # Iterate model->tool->model until Gemini produces a final text response.
        for _ in range(6):
            function_calls = response.function_calls
            if not function_calls:
                logger.info("No function calls in Gemini response, returning final text")
                return response.text or ""

            function_responses = []

            for function_call in function_calls:
                name = function_call.name or ""
                args = function_call.args or {}
                call_id = function_call.id or self._new_tool_call_id(name)
                logger.info("Gemini requested tool call: %s (call_id=%s)", name, call_id)
                tool_output = await self._execute_tool(session_id, name, args, call_id)
                function_responses.append(
                    types.Part.from_function_response(name=name, response={"result": tool_output})
                )

            logger.debug("Sending function responses back to Gemini")
            response = await asyncio.to_thread(chat.send_message, function_responses)
            logger.info("Received Gemini response after tool calls")

        logger.info("Reached max tool loop iterations, returning last Gemini response")
        return response.text or ""

    async def prompt(
        self,
        prompt: list[Any],
        session_id: str,
        **kwargs: Any,
    ) -> PromptResponse:
        """Handle ACP `session/prompt` RPC.

        Invoked by ACP runtime dispatch after initialize/session handshake; this
        is the protocol entrypoint for prompt execution.
        """
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
            logger.warning(
                f"Prompt handling cancelled for session {session_id} "
                "(connection/task terminated before response completed)."
            )
            raise
        except Exception as e:
            logger.exception("Error calling Gemini: %s", e)
            conn = self._require_conn()
            await conn.session_update(
                session_id,
                update_agent_message(text_block(f"\n[Error: {str(e)}]"))
            )
        finally:
            conn = self._require_conn()
            await conn.session_update(
                session_id,
                update_agent_message(text_block(END_OF_TURN_MARKER)),
            )

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
            if task.exception():
                logger.error("Task exception (%s): %s", task.get_name(), task.exception())
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
