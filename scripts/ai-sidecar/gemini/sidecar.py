# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

import asyncio
import json
import os
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
DEFAULT_MCP_URL = "http://127.0.0.1:16687/mcp"


@dataclass(frozen=True)
class SidecarConfig:
    gemini_api_key: str
    mcp_url: str = DEFAULT_MCP_URL


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
    def __init__(self, mcp_url: str):
        self._mcp_url = mcp_url
        self._toolset = MCPToolset(
            connection_params=StreamableHTTPConnectionParams(url=mcp_url),
        )
        self._tools_by_name: dict[str, Any] = {}
        self._gemini_tools: list[types.Tool] = []
        self._initialized = False

    async def initialize(self) -> None:
        if self._initialized:
            return

        timeout_sec = 15.0

        # Discover available MCP tools once, then expose them to Gemini as function declarations.
        print(
            f"Initializing MCP tool discovery from {self._mcp_url} "
            f"(single attempt timeout={timeout_sec}s)"
        )

        try:
            print(
                f"MCP connection attempt 1: trying {self._mcp_url} "
                f"(timeout {timeout_sec:.1f}s)"
            )
            adk_tools = await asyncio.wait_for(self._toolset.get_tools(), timeout=timeout_sec)
        except asyncio.CancelledError:
            print(
                "MCP tool discovery cancelled before completion "
                "(client likely disconnected before MCP became available)."
            )
            print(f"MCP was not connected for {self._mcp_url}; request aborted.")
            raise
        except Exception as exc:
            message = (
                f"Unable to connect to MCP at {self._mcp_url} on first attempt. "
                "Stopping request."
            )
            print(f"{message} Error: {exc}")
            raise RuntimeError(message) from exc

        self._tools_by_name = {tool.name: tool for tool in adk_tools}
        print(f"Retrieved tools from MCP: {list(self._tools_by_name.keys())}")

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


def build_config_from_env() -> SidecarConfig:
    api_key = os.environ.get("GEMINI_API_KEY")
    if not api_key:
        raise RuntimeError(
            "GEMINI_API_KEY environment variable is not set; cannot initialize Gemini client."
        )
    return SidecarConfig(
        gemini_api_key=api_key,
        mcp_url=os.environ.get("JAEGER_MCP_URL", DEFAULT_MCP_URL),
    )


class JaegerSidecarAgent(Agent):
    def __init__(self, config: SidecarConfig):
        super().__init__()
        self._conn: Client | None = None
        if not config.gemini_api_key:
            raise RuntimeError(
                "GEMINI_API_KEY environment variable is not set; cannot initialize Gemini client."
            )
        self._gemini = genai.Client(api_key=config.gemini_api_key)
        self._mcp = JaegerMCPBridge(config.mcp_url)
        self._next_session_id = 1
        self._next_tool_call_id = 1

    def _new_tool_call_id(self, tool_name: str) -> str:
        call_id = f"{tool_name}-{self._next_tool_call_id}"
        self._next_tool_call_id += 1
        return call_id

    def on_connect(self, conn: Client) -> None:
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
        print(f"Agent initialized with protocol version {protocol_version}")
        return InitializeResponse(
            protocol_version=PROTOCOL_VERSION,
            agent_capabilities=AgentCapabilities(),
            agent_info=Implementation(name="jaeger-gemini-sidecar", title="Jaeger AI", version="0.1.0"),
        )

    async def new_session(self, cwd: str, mcp_servers: Any = None, **kwargs: Any) -> NewSessionResponse:
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
        return LoadSessionResponse()

    async def list_sessions(self, cursor: str | None = None, cwd: str | None = None, **kwargs: Any) -> ListSessionsResponse:
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
        print(f"Starting agentic Gemini loop for session {session_id} with user text: {user_text!r}")
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
        print(f"Passing tools to Gemini: {tool_names}")

        
        chat = self._gemini.chats.create(
            model="gemini-2.5-flash",
            config=types.GenerateContentConfig(
                system_instruction=system_instruction,
                tools=cast(Any, mcp_tools),
                automatic_function_calling=types.AutomaticFunctionCallingConfig(disable=True),
            ),
        )

        print(f"Sending user message to Gemini: {user_text}")
        response = await asyncio.to_thread(chat.send_message, user_text)

        # Iterate model->tool->model until Gemini produces a final text response.
        for _ in range(6):
            function_calls = response.function_calls
            if not function_calls:
                print("No function calls in Gemini response, breaking loop")
                print(f"break Gemini final response: {response.text}")
                return response.text or ""

            function_responses = []

            for function_call in function_calls:
                name = function_call.name or ""
                args = function_call.args or {}
                call_id = function_call.id or self._new_tool_call_id(name)
                print(f"Gemini requested tool call: {name} with args {args} and call_id {call_id}")
                tool_output = await self._execute_tool(session_id, name, args, call_id)
                function_responses.append(
                    types.Part.from_function_response(name=name, response={"result": tool_output})
                )

            print(f"Sending function responses back to Gemini: {function_responses}")
            response = await asyncio.to_thread(chat.send_message, function_responses)
            print(f"Gemini response after tool calls: {response.text}")

        print(f"Final Gemini response: {response.text}")
        return response.text or ""

    async def prompt(self, prompt: list[Any], session_id: str, **kwargs: Any) -> PromptResponse:
        print(f"Received prompt request for session {session_id}")

        # Extract text from prompt blocks
        user_text = ""
        for block in prompt:
            if hasattr(block, "text"):
                user_text += block.text

        try:
            conn = self._require_conn()
            final_answer = await self._run_agentic_gemini_loop(session_id, user_text)
            if final_answer:
                print(f"final answer from Gemini: {final_answer} with session_id {session_id}")
                await conn.session_update(
                    session_id,
                    update_agent_message(text_block(final_answer)),
                )
        except asyncio.CancelledError:
            print(
                f"Prompt handling cancelled for session {session_id} "
                "(connection/task terminated before response completed)."
            )
            raise
        except Exception as e:
            print(f"Error calling Gemini: {e}")
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


async def handle_websocket(websocket, agent_factory: Callable[[], Agent] | None = None):
    print("New websocket connection from Jaeger AI Gateway")

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
            config = build_config_from_env()
            agent_factory = lambda: JaegerSidecarAgent(config)  # pyright: ignore[reportAbstractUsage]

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
            print(f"Task finished: {task.get_name()}")
            if task.cancelled():
                print(f"Task was cancelled: {task.get_name()}")
                continue
            if task.exception():
                print(f"Task exception: {task.exception()}")
            else:
                print(f"Task completed normally: {task.get_name()}")

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
        for task in lingering:
            task.cancel()
        if lingering:
            await asyncio.gather(*lingering, return_exceptions=True)

        print("Websocket connection closed")
