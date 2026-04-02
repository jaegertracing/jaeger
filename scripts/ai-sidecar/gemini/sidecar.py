# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

import asyncio
import json
import os
import socket
from dataclasses import dataclass
from typing import Any, Callable

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
            return declaration

    # ADK BaseTool currently exposes declaration via _get_declaration().
    # Keep this fallback isolated in one place to reduce breakage risk.
    private_get_declaration = getattr(tool, "_get_declaration", None)
    if callable(private_get_declaration):
        return private_get_declaration()

    return None


class JaegerMCPBridge:
    def __init__(self, mcp_url: str):
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
        adk_tools = await self._toolset.get_tools()
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


class JaegerSidecarAgent(Agent):
    def __init__(self, config: SidecarConfig):
        super().__init__()
        self._conn: Client = None
        if not config.gemini_api_key:
            raise RuntimeError(
                "GEMINI_API_KEY environment variable is not set; cannot initialize Gemini client."
            )
        self._gemini = genai.Client(api_key=config.gemini_api_key)
        self._mcp = JaegerMCPBridge(config.mcp_url)
        self._next_session_id = 1


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

    def on_connect(self, conn: Client) -> None:
        self._conn = conn

    async def initialize(
        self,
        protocol_version: int,
        **kwargs: Any,
    ) -> InitializeResponse:
        print(f"Agent initialized with protocol version {protocol_version}")
        return InitializeResponse(
            protocol_version=PROTOCOL_VERSION,
            agent_capabilities=AgentCapabilities(),
            agent_info=Implementation(name="jaeger-gemini-sidecar", title="Jaeger AI", version="0.1.0"),
        )

    async def new_session(self, **kwargs: Any) -> NewSessionResponse:
        session_id = f"sess-{self._next_session_id}"
        self._next_session_id += 1
        return NewSessionResponse(session_id=session_id)

    async def _execute_tool(self, session_id: str, tool_name: str, args: dict[str, Any], tool_call_id: str) -> Any:
        await self._conn.session_update(
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

        await self._conn.session_update(
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
        system_instruction = (
            "You are a Jaeger tracing assistant. "
            "A tool named search_traces is available. "
            "Call this tool whenever trace/span lookup data is needed before answering. "
        )

        mcp_tools = await self._mcp.get_gemini_tools()
        print(f"Passing tools to Gemini: {[tool.function_declarations[0].name for tool in mcp_tools]}")

        chat = self._gemini.chats.create(
            model="gemini-2.5-flash",
            config=types.GenerateContentConfig(
                system_instruction=system_instruction,
                tools=mcp_tools,
                automatic_function_calling=types.AutomaticFunctionCallingConfig(disable=True),
            ),
        )

        print(f"Sending user message to Gemini: {user_text}")
        response = chat.send_message(user_text)

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
                call_id = function_call.id or f"{name}-{self._next_session_id}"
                print(f"Gemini requested tool call: {name} with args {args} and call_id {call_id}")
                tool_output = await self._execute_tool(session_id, name, args, call_id)
                function_responses.append(
                    types.Part.from_function_response(name=name, response={"result": tool_output})
                )

            print(f"Sending function responses back to Gemini: {function_responses}")
            response = chat.send_message(function_responses)
            print(f"Gemini response after tool calls: {response.text}")

        print(f"Final Gemini response: {response.text}")
        return response.text or ""

    async def prompt(self, session_id: str, prompt: list[Any], **kwargs: Any) -> PromptResponse:
        print(f"Received prompt request for session {session_id}")

        # Extract text from prompt blocks
        user_text = ""
        for block in prompt:
            if hasattr(block, "text"):
                user_text += block.text

        try:
            final_answer = await self._run_agentic_gemini_loop(session_id, user_text)
            if final_answer:
                print(f"final answer from Gemini: {final_answer} with session_id {session_id}")
                await self._conn.session_update(
                    session_id,
                    update_agent_message(text_block(final_answer)),
                )
        except Exception as e:
            print(f"Error calling Gemini: {e}")
            await self._conn.session_update(
                session_id,
                update_agent_message(text_block(f"\n[Error: {str(e)}]"))
            )
        finally:
            await self._conn.session_update(
                session_id,
                update_agent_message(text_block(END_OF_TURN_MARKER)),
            )

        return PromptResponse(stop_reason="end_turn")


async def handle_websocket(websocket, agent_factory: Callable[[], Agent] | None = None):
    print("New websocket connection from Jaeger AI Gateway")

    # Bridge ACP stdio-style streams to WebSocket transport used by the Go gateway.
    # Socketpair avoids reimplementing ACP framing logic in this process.

    asock, csock = socket.socketpair()

    agent_reader, agent_writer = await asyncio.open_connection(sock=asock)
    client_reader, client_writer = await asyncio.open_connection(sock=csock)

    # Start the ACP local agent linked to the agent ends of the socket pair
    if agent_factory is None:
        config = build_config_from_env()
        agent_factory = lambda: JaegerSidecarAgent(config)

    agent = agent_factory()
    agent_task = asyncio.create_task(run_agent(agent, agent_writer, agent_reader), name="agent_task")

    # Bridge the client ends of the socket pair up to the WebSocket
    ws_read_task = asyncio.create_task(ws_to_client_writer(websocket, client_writer), name="ws_read_task")
    ws_write_task = asyncio.create_task(client_reader_to_ws(websocket, client_reader), name="ws_write_task")

    # Wait for the connection to end
    done, pending = await asyncio.wait(
        [agent_task, ws_read_task, ws_write_task],
        return_when=asyncio.FIRST_COMPLETED,
    )

    for task in done:
        print(f"Task finished: {task.get_name()}")
        if task.cancelled():
            print(f"Task was cancelled: {task.get_name()}")
            continue
        if task.exception():
            print(f"Task exception: {task.exception()}")

    for task in pending:
        task.cancel()

    print("Websocket connection closed")
