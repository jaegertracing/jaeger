import asyncio
import json
import os
import socket
from urllib.parse import quote_plus

import websockets

from google import genai
from google.genai import types
from typing import Any

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

api_key = os.environ.get("GEMINI_API_KEY")

class JaegerSidecarAgent(Agent):
    def __init__(self):
        super().__init__()
        self._conn: Client = None
        self._gemini = genai.Client(api_key=api_key)
        self._next_session_id = 1

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

    async def _execute_search_traces_tool(self, session_id: str, query: str, tool_call_id: str) -> str:
        tool_path = f"acp://tool/search_traces?q={quote_plus(query)}"

        await self._conn.session_update(
            session_id,
            start_tool_call(
                tool_call_id,
                "search_traces",
                kind="search",
                status="in_progress",
            ),
        )

        result = await self._conn.read_text_file(path=tool_path, session_id=session_id)

        await self._conn.session_update(
            session_id,
            update_tool_call(
                tool_call_id,
                status="completed",
                content=[tool_content(text_block(result.content))],
                raw_output={"content": result.content},
            ),
        )

        return result.content

    async def _run_agentic_gemini_loop(self, session_id: str, user_text: str) -> str:
        system_instruction = (
            "You are a Jaeger tracing assistant. "
            "A tool named search_traces is available. "
            "Call this tool whenever trace/span lookup data is needed before answering."
        )

        search_traces_tool = types.Tool(
            function_declarations=[
                types.FunctionDeclaration(
                    name="search_traces",
                    description="Search traces from Jaeger ACP client proxy.",
                    parameters=types.Schema(
                        type=types.Type.OBJECT,
                        properties={
                            "query": types.Schema(
                                type=types.Type.STRING,
                                description="Natural language or structured trace lookup query.",
                            ),
                        },
                    ),
                ),
            ]
        )

        chat = self._gemini.chats.create(
            model="gemini-2.5-flash",
            config=types.GenerateContentConfig(
                system_instruction=system_instruction,
                tools=[search_traces_tool],
                automatic_function_calling=types.AutomaticFunctionCallingConfig(disable=True),
            ),
        )

        response = chat.send_message(user_text)

        for _ in range(6):
            function_calls = response.function_calls
            if not function_calls:
                return response.text or ""

            function_responses = []

            for function_call in function_calls:
                name = function_call.name or ""
                args = function_call.args or {}
                call_id = function_call.id or f"search-traces-{self._next_session_id}"

                if name == "search_traces":
                    query = str(args.get("query", user_text))
                    tool_output = await self._execute_search_traces_tool(session_id, query, call_id)
                    try:
                        parsed = json.loads(tool_output)
                        function_responses.append(
                            types.Part.from_function_response(name=name, response={"result": parsed})
                        )
                    except Exception:
                        function_responses.append(
                            types.Part.from_function_response(name=name, response={"result": tool_output})
                        )
                else:
                    function_responses.append(
                        types.Part.from_function_response(
                            name=name,
                            response={"error": f"unsupported tool: {name}"},
                        )
                    )

            response = chat.send_message(function_responses)

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

        return PromptResponse(stop_reason="end_turn")


async def ws_to_client_writer(websocket, client_writer):
    try:
        async for message in websocket:
            if isinstance(message, str):
                message = message.encode('utf-8')
            client_writer.write(message)
            if b'\n' not in message:
                client_writer.write(b'\n')
            await client_writer.drain()
    except websockets.exceptions.ConnectionClosed:
        pass
    except Exception as e:
        print(f"Error in ws_to_client reads: {e}")
    finally:
        client_writer.close()

async def client_reader_to_ws(websocket, client_reader):
    try:
        while True:
            line = await client_reader.readline()
            if not line:
                break
            await websocket.send(line.decode('utf-8'))
    except websockets.exceptions.ConnectionClosed:
        pass
    except Exception as e:
        print(f"Error in client_to_ws writes: {e}")


async def handle_websocket(websocket):
    print("New websocket connection from Jaeger AI Gateway")
    
    # Create bidirectional streams using an in-memory socket pair to link acp_sdk to websocket
    asock, csock = socket.socketpair()
    
    agent_reader, agent_writer = await asyncio.open_connection(sock=asock)
    client_reader, client_writer = await asyncio.open_connection(sock=csock)
    
    # Start the ACP local agent linked to the agent ends of the socket pair
    agent = JaegerSidecarAgent()
    agent_task = asyncio.create_task(run_agent(agent, agent_writer, agent_reader), name="agent_task")
    
    # Bridge the client ends of the socket pair up to the WebSocket
    ws_read_task = asyncio.create_task(ws_to_client_writer(websocket, client_writer), name="ws_read_task")
    ws_write_task = asyncio.create_task(client_reader_to_ws(websocket, client_reader), name="ws_write_task")
    
    # Wait for the connection to end
    done, pending = await asyncio.wait(
        [agent_task, ws_read_task, ws_write_task],
        return_when=asyncio.FIRST_COMPLETED
    )
    
    for task in done:
        print(f"Task finished: {task.get_name()}")
        if task.exception():
            print(f"Task exception: {task.exception()}")

    for task in pending:
        task.cancel()
        
    print("Websocket connection closed")

async def main():
    async with websockets.serve(handle_websocket, "localhost", 9000):
        print("Jaeger ACP Sidecar listening on ws://localhost:9000")
        await asyncio.Future()

if __name__ == "__main__":
    asyncio.run(main())
