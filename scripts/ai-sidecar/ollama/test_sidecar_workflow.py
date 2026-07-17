# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

from __future__ import annotations

import asyncio
import contextlib
import json
import threading
from collections.abc import Iterator
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from typing import Any
from functools import partial

import pytest
import websockets

from acp import Agent, PROTOCOL_VERSION
from acp.schema import AgentCapabilities, Implementation, ListSessionsResponse, LoadSessionResponse, NewSessionResponse, PromptResponse
from acp.helpers import text_block, update_agent_message

import llm
import sidecar
from mcp.types import CallToolResult, TextContent, Tool
from mcp_bridge import _to_ollama_tool, _tool_result_output
from sidecar_config import SidecarConfig
from sidecar_helpers import _build_contextual_ollama_tools

DEFAULT_PROMPT = "hello"
DEFAULT_CWD = str(Path.cwd())


class PendingRequests:
    def __init__(self) -> None:
        self._next_id = 1
        self._futures: dict[str, asyncio.Future[dict[str, Any]]] = {}

    def new_id(self) -> str:
        request_id = str(self._next_id)
        self._next_id += 1
        return request_id

    def register(self, request_id: str) -> asyncio.Future[dict[str, Any]]:
        future: asyncio.Future[dict[str, Any]] = asyncio.get_running_loop().create_future()
        self._futures[request_id] = future
        return future

    def resolve(self, request_id: str, payload: dict[str, Any]) -> None:
        future = self._futures.pop(request_id, None)
        if future is not None and not future.done():
            future.set_result(payload)


class FakeAgent(Agent):
    last_instance: "FakeAgent | None" = None

    def __init__(self) -> None:
        super().__init__()
        FakeAgent.last_instance = self
        self._conn = None
        self._next_session_id = 1
        self.received_prompts: list[tuple[str, str]] = []

    def on_connect(self, conn: Any) -> None:
        self._conn = conn

    async def initialize(
        self,
        protocol_version: int,
        client_capabilities: Any = None,
        client_info: Any = None,
        **kwargs: Any,
    ) -> sidecar.InitializeResponse:
        assert protocol_version == PROTOCOL_VERSION
        return sidecar.InitializeResponse(
            protocol_version=PROTOCOL_VERSION,
            agent_capabilities=AgentCapabilities(),
            agent_info=Implementation(name="jaeger-ollama-sidecar", title="Jaeger AI", version="test"),
        )

    async def new_session(
        self,
        cwd: str,
        additional_directories: list[str] | None = None,
        mcp_servers: Any = None,
        **kwargs: Any,
    ) -> NewSessionResponse:
        session_id = f"sess-test-{self._next_session_id}"
        self._next_session_id += 1
        return NewSessionResponse(session_id=session_id)

    async def load_session(
        self,
        cwd: str,
        session_id: str,
        mcp_servers: Any = None,
        additional_directories: list[str] | None = None,
        **kwargs: Any,
    ) -> LoadSessionResponse | None:
        return LoadSessionResponse()

    async def list_sessions(
        self,
        cwd: str | None = None,
        cursor: str | None = None,
        **kwargs: Any,
    ) -> ListSessionsResponse:
        return ListSessionsResponse(sessions=[])

    async def prompt(self, session_id: str, prompt: list[Any], **kwargs: Any) -> PromptResponse:
        user_text = "".join(block.text for block in prompt if hasattr(block, "text"))
        self.received_prompts.append((session_id, user_text))

        assert self._conn is not None
        await self._conn.session_update(session_id, update_agent_message(text_block(f"echo: {user_text}")))
        return PromptResponse(stop_reason="end_turn")


async def recv_loop(
    websocket: Any,
    pending: PendingRequests,
    messages: list[dict[str, Any]],
) -> None:
    try:
        while True:
            raw_message = await websocket.recv()
            if isinstance(raw_message, bytes):
                text = raw_message.decode("utf-8", errors="replace")
            else:
                text = raw_message

            payload = json.loads(text)
            messages.append(payload)

            request_id = payload.get("id")
            if request_id is not None:
                pending.resolve(str(request_id), payload)
    except websockets.exceptions.ConnectionClosed:
        pass


async def send_request(
    websocket: Any,
    pending: PendingRequests,
    method: str,
    params: dict[str, Any],
) -> dict[str, Any]:
    request_id = pending.new_id()
    future = pending.register(request_id)
    try:
        await websocket.send(
            json.dumps(
                {
                    "jsonrpc": "2.0",
                    "id": request_id,
                    "method": method,
                    "params": params,
                }
            )
        )
        return await asyncio.wait_for(future, timeout=10.0)
    except asyncio.TimeoutError:
        pending._futures.pop(request_id, None)
        raise


async def wait_for_message_text(messages: list[dict[str, Any]], expected_text: str) -> None:
    deadline = asyncio.get_running_loop().time() + 10.0
    while asyncio.get_running_loop().time() < deadline:
        if any(expected_text in json.dumps(message) for message in messages):
            return
        await asyncio.sleep(0.05)
    raise TimeoutError(f"did not receive message containing {expected_text!r}")


async def run_workflow_test(
    prompt: str,
    cwd: str,
    agent_factory: Any = FakeAgent,
    expected_agent_name: str = "jaeger-ollama-sidecar",
    expected_session_prefix: str = "sess-test-",
    expected_response_text: str = "echo: ",
) -> str:
    pending = PendingRequests()
    received_messages: list[dict[str, Any]] = []
    session_id = ""

    async with websockets.serve(
        partial(sidecar.handle_websocket, agent_factory=agent_factory),
        "127.0.0.1",
        0,
    ) as server:
        port = next(iter(server.sockets)).getsockname()[1]
        uri = f"ws://127.0.0.1:{port}"

        async with websockets.connect(uri) as websocket:
            receiver_task = asyncio.create_task(recv_loop(websocket, pending, received_messages))
            try:
                init_response = await send_request(
                    websocket,
                    pending,
                    "initialize",
                    {
                        "protocolVersion": PROTOCOL_VERSION,
                        "clientCapabilities": {
                            "fs": {"readTextFile": False, "writeTextFile": False},
                            "terminal": False,
                        },
                        "clientInfo": {
                            "name": "pytest-client",
                            "title": "pytest manual ACP workflow",
                            "version": "test",
                        },
                    },
                )
                init_result = init_response.get("result", init_response)
                assert init_result.get("protocolVersion", init_result.get("protocol_version")) == PROTOCOL_VERSION
                agent_info = init_result.get("agentInfo", init_result.get("agent_info"))
                assert agent_info["name"] == expected_agent_name

                session_response = await send_request(
                    websocket,
                    pending,
                    "session/new",
                    {
                        "cwd": cwd,
                        "mcpServers": [],
                    },
                )
                session_result = session_response.get("result", session_response)
                session_id = session_result.get("sessionId") or session_result.get("session_id")
                assert session_id is not None
                assert session_id.startswith(expected_session_prefix)

                prompt_response = await send_request(
                    websocket,
                    pending,
                    "session/prompt",
                    {
                        "sessionId": session_id,
                        "prompt": [
                            {
                                "type": "text",
                                "text": prompt,
                            }
                        ],
                    },
                )
                prompt_result = prompt_response.get("result", prompt_response)
                assert prompt_result.get("stopReason", prompt_result.get("stop_reason")) == "end_turn"

                await wait_for_message_text(received_messages, expected_response_text)
            finally:
                receiver_task.cancel()
                # recv_loop returns normally when the server closed the
                # connection first, in which case cancel() is a no-op — so
                # accept either outcome rather than requiring CancelledError.
                with contextlib.suppress(asyncio.CancelledError):
                    await receiver_task

    assert any(expected_response_text in json.dumps(message) for message in received_messages)
    return session_id


def test_complete_acp_workflow_with_fake_agent() -> None:
    session_id = asyncio.run(run_workflow_test(DEFAULT_PROMPT, DEFAULT_CWD))
    fake_agent = FakeAgent.last_instance
    assert fake_agent is not None
    assert fake_agent.received_prompts == [(session_id, DEFAULT_PROMPT)]


@contextlib.contextmanager
def fake_ollama(responses: list[dict[str, Any]]) -> Iterator[tuple[str, list[dict[str, Any]]]]:
    """Serve canned /api/chat responses and record what the sidecar sent.

    A stand-in for `ollama serve` so the tool-calling loop is exercised against
    the real wire format without pulling a multi-GB model in CI.
    """
    seen: list[dict[str, Any]] = []
    remaining = list(responses)

    class Handler(BaseHTTPRequestHandler):
        def do_POST(self) -> None:
            length = int(self.headers.get("Content-Length", "0"))
            seen.append({"path": self.path, "body": json.loads(self.rfile.read(length))})
            body = json.dumps(remaining.pop(0)).encode("utf-8")
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        def log_message(self, format: str, *args: Any) -> None:
            pass

    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        yield f"http://127.0.0.1:{server.server_port}", seen
    finally:
        server.shutdown()
        server.server_close()
        thread.join(timeout=5)


def _chat_response(message: dict[str, Any]) -> dict[str, Any]:
    return {"model": "qwen3:8b", "created_at": "2026-01-01T00:00:00Z", "done": True, "message": message}


FIND_TRACES_TOOL = {
    "type": "function",
    "function": {
        "name": "find_traces",
        "description": "Find traces for a service",
        "parameters": {"type": "object", "properties": {"service": {"type": "string"}}, "required": ["service"]},
    },
}


def test_ollama_chat_drives_the_tool_calling_loop() -> None:
    tool_turn = _chat_response(
        {
            "role": "assistant",
            "content": "",
            "tool_calls": [{"function": {"name": "find_traces", "arguments": {"service": "checkout"}}}],
        }
    )
    final_turn = _chat_response({"role": "assistant", "content": "Checkout is slow: the DB span dominates."})

    async def scenario(base_url: str) -> tuple[llm.ModelTurn, llm.ModelTurn]:
        client = llm.build_ollama_client(base_url, timeout_sec=10.0)
        chat = llm.OllamaChat(client=client, model="qwen3:8b", system_instruction="system prompt", tools=[FIND_TRACES_TOOL])
        first = await chat.send_user_message("why is checkout slow?")
        second = await chat.send_tool_results([llm.ToolResult(call=first.tool_calls[0], output={"traces": 1})])
        return first, second

    with fake_ollama([tool_turn, final_turn]) as (base_url, seen):
        first, second = asyncio.run(scenario(base_url))

    assert first.text == ""
    assert [(call.name, dict(call.args)) for call in first.tool_calls] == [("find_traces", {"service": "checkout"})]
    assert second.tool_calls == []
    assert second.text == "Checkout is slow: the DB span dominates."

    assert [request["path"] for request in seen] == ["/api/chat", "/api/chat"]
    first_body = seen[0]["body"]
    assert first_body["model"] == "qwen3:8b"
    assert first_body["stream"] is False
    assert first_body["tools"] == [FIND_TRACES_TOOL]
    assert [(m["role"], m["content"]) for m in first_body["messages"]] == [
        ("system", "system prompt"),
        ("user", "why is checkout slow?"),
    ]

    # Ollama is stateless, so the second request must replay the transcript,
    # including the assistant turn that requested the tool.
    second_messages = seen[1]["body"]["messages"]
    assert [message["role"] for message in second_messages] == ["system", "user", "assistant", "tool"]
    assert second_messages[2]["tool_calls"][0]["function"]["name"] == "find_traces"
    assert second_messages[3]["tool_name"] == "find_traces"
    assert second_messages[3]["content"] == json.dumps({"traces": 1}, ensure_ascii=False)


def test_ollama_chat_keeps_reasoning_out_of_the_answer() -> None:
    # Reasoning models (qwen3 among them) return their scratchpad separately;
    # only `content` is the answer the user should see.
    turn = _chat_response({"role": "assistant", "thinking": "let me check the spans...", "content": "The DB is slow."})

    async def scenario(base_url: str) -> llm.ModelTurn:
        client = llm.build_ollama_client(base_url, timeout_sec=10.0)
        return await llm.OllamaChat(client=client, model="qwen3:8b", system_instruction="", tools=[]).send_user_message("hi")

    with fake_ollama([turn]) as (base_url, seen):
        result = asyncio.run(scenario(base_url))

    assert result.text == "The DB is slow."
    # No system_instruction and no tools: neither should be fabricated.
    assert [m["role"] for m in seen[0]["body"]["messages"]] == ["user"]
    assert seen[0]["body"]["tools"] == []


def test_ollama_chat_reports_unreachable_server() -> None:
    # Port 1 is reserved and never listening, so this exercises the failure an
    # operator hits when they forget to start `ollama serve`.
    async def scenario() -> None:
        client = llm.build_ollama_client("http://127.0.0.1:1", timeout_sec=5.0)
        await llm.OllamaChat(client=client, model="qwen3:8b", system_instruction="", tools=[]).send_user_message("hi")

    with pytest.raises(RuntimeError, match="Unable to reach Ollama"):
        asyncio.run(scenario())


def test_mcp_tool_advertises_its_input_schema_unchanged() -> None:
    # MCP's inputSchema is already JSON Schema, which is what Ollama wants —
    # this sidecar needs no schema translation, and that must stay true.
    schema = {"type": "object", "properties": {"service": {"type": "string"}}, "required": ["service"]}
    tool = Tool(name="find_traces", description="Find traces", inputSchema=schema)

    assert _to_ollama_tool(tool) == {
        "type": "function",
        "function": {"name": "find_traces", "description": "Find traces", "parameters": schema},
    }


def test_contextual_tools_convert_to_ollama_tools() -> None:
    snapshot = [
        {
            "name": "ui_show_flamegraph",
            "description": "Open the flamegraph",
            "parameters": {"type": "object", "properties": {"trace_id": {"type": "string"}}},
        },
        # A tool with no parameters still needs a valid schema object.
        {"name": "ui_refresh"},
        # Malformed entries must not reach the model.
        {"description": "nameless"},
    ]

    converted = _build_contextual_ollama_tools(snapshot)

    assert [tool["function"]["name"] for tool in converted] == ["ui_show_flamegraph", "ui_refresh"]
    assert converted[0]["function"]["parameters"] == {"type": "object", "properties": {"trace_id": {"type": "string"}}}
    assert converted[1]["function"]["parameters"] == {"type": "object", "properties": {}}
    assert converted[1]["function"]["description"] == ""


def test_tool_result_prefers_structured_content_then_text() -> None:
    structured = CallToolResult(content=[TextContent(type="text", text="ignored")], structuredContent={"traces": []})
    assert _tool_result_output(structured) == {"traces": []}

    textual = CallToolResult(content=[TextContent(type="text", text="line 1"), TextContent(type="text", text="line 2")])
    assert _tool_result_output(textual) == "line 1\nline 2"


def _config(**overrides: Any) -> SidecarConfig:
    settings: dict[str, Any] = {
        "ollama_url": "http://localhost:11434",
        "model": "qwen3:8b",
        "ollama_timeout_sec": 300.0,
        "mcp_url": "http://127.0.0.1:16687/mcp",
        "mcp_discovery_timeout_sec": 15.0,
        "otlp_endpoint": "http://localhost:4317",
        "otlp_insecure": True,
    }
    settings.update(overrides)
    return SidecarConfig(**settings)


def test_valid_config_needs_no_api_key() -> None:
    _config().validate()


@pytest.mark.parametrize(
    ("override", "expected_error"),
    [
        ({"ollama_url": ""}, "Ollama URL"),
        ({"model": ""}, "Model name"),
        ({"ollama_timeout_sec": 0.0}, "Ollama request timeout"),
        ({"otlp_endpoint": ""}, "OTEL_EXPORTER_OTLP_ENDPOINT"),
        ({"mcp_url": ""}, "JAEGER_MCP_URL"),
        ({"mcp_discovery_timeout_sec": 0.0}, "MCP discovery timeout"),
    ],
)
def test_invalid_config_is_rejected(override: dict[str, Any], expected_error: str) -> None:
    # Tracing is initialized and MCP is queried on every run, so these stay
    # required even though there is no API key to check.
    with pytest.raises(RuntimeError, match=expected_error):
        _config(**override).validate()
