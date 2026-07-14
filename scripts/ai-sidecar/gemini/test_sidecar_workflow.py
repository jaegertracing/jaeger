# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

from __future__ import annotations

import asyncio
import json
from pathlib import Path
from typing import Any
from functools import partial

import pytest
import websockets
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import SimpleSpanProcessor
from opentelemetry.sdk.trace.export.in_memory_span_exporter import InMemorySpanExporter

from acp import Agent, PROTOCOL_VERSION
from acp.schema import AgentCapabilities, Implementation, ListSessionsResponse, LoadSessionResponse, NewSessionResponse, PromptResponse
from acp.helpers import text_block, update_agent_message

import sidecar
from sidecar_config import SidecarConfig

END_OF_TURN_MARKER = "__END_OF_TURN__"
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
            agent_info=Implementation(name="jaeger-gemini-sidecar", title="Jaeger AI", version="test"),
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

    async def prompt(self, prompt: list[Any], session_id: str, message_id: str | None = None, **kwargs: Any) -> PromptResponse:
        user_text = "".join(block.text for block in prompt if hasattr(block, "text"))
        self.received_prompts.append((session_id, user_text))

        assert self._conn is not None
        await self._conn.session_update(session_id, update_agent_message(text_block(f"echo: {user_text}")))
        await self._conn.session_update(session_id, update_agent_message(text_block(END_OF_TURN_MARKER)))
        return PromptResponse(stop_reason="end_turn")


async def recv_loop(
    websocket: Any,
    pending: PendingRequests,
    messages: list[dict[str, Any]],
    stop_event: asyncio.Event,
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

            if END_OF_TURN_MARKER in text:
                stop_event.set()
    except websockets.exceptions.ConnectionClosed:
        stop_event.set()


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


async def run_workflow_test(prompt: str, cwd: str) -> None:
    pending = PendingRequests()
    received_messages: list[dict[str, Any]] = []
    stop_event = asyncio.Event()

    async with websockets.serve(partial(sidecar.handle_websocket, agent_factory=FakeAgent), "127.0.0.1", 0) as server:
        port = next(iter(server.sockets)).getsockname()[1]
        uri = f"ws://127.0.0.1:{port}"

        async with websockets.connect(uri) as websocket:
            receiver_task = asyncio.create_task(recv_loop(websocket, pending, received_messages, stop_event))
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
                assert agent_info["name"] == "jaeger-gemini-sidecar"

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
                assert session_id.startswith("sess-test-")

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

                await asyncio.wait_for(stop_event.wait(), timeout=10.0)
            finally:
                receiver_task.cancel()
                with pytest.raises(asyncio.CancelledError):
                    await receiver_task

    fake_agent = FakeAgent.last_instance
    assert fake_agent is not None
    assert fake_agent.received_prompts == [(session_id, prompt)]
    assert any(END_OF_TURN_MARKER in json.dumps(message) for message in received_messages)
    assert any("echo: " in json.dumps(message) for message in received_messages)

def test_complete_acp_workflow_with_fake_agent() -> None:
    asyncio.run(run_workflow_test(DEFAULT_PROMPT, DEFAULT_CWD))


class FakeConn:
    """Minimal stand-in for the ACP Client the runtime passes to on_connect,
    covering only the surface JaegerSidecarAgent._execute_tool /
    _execute_contextual_tool actually call."""

    def __init__(self, ext_method_response: dict[str, Any] | None = None) -> None:
        self.session_updates: list[Any] = []
        self._ext_method_response = ext_method_response or {"acknowledged": True}

    async def session_update(self, session_id: str, update: Any, **kwargs: Any) -> None:
        self.session_updates.append(update)

    async def ext_method(self, method: str, params: dict[str, Any]) -> dict[str, Any]:
        return self._ext_method_response


def _fake_call_tool(tool_output: Any) -> Any:
    async def call_tool(name: str, args: dict[str, Any]) -> Any:
        return tool_output

    return call_tool


def _new_jaeger_sidecar_agent() -> sidecar.JaegerSidecarAgent:
    config = SidecarConfig(
        gemini_api_key="test-key",
        mcp_url="http://127.0.0.1:0/mcp",
        mcp_discovery_timeout_sec=1.0,
        otlp_endpoint="127.0.0.1:0",
        otlp_insecure=True,
    )
    return sidecar.JaegerSidecarAgent(config)  # pyright: ignore[reportAbstractUsage]


def _find_span(exporter: InMemorySpanExporter, name: str) -> Any:
    matches = [span for span in exporter.get_finished_spans() if span.name == name]
    assert matches, f"no finished span named {name!r}"
    return matches[0]


@pytest.fixture
def span_exporter(monkeypatch: pytest.MonkeyPatch) -> InMemorySpanExporter:
    exporter = InMemorySpanExporter()
    provider = TracerProvider()
    provider.add_span_processor(SimpleSpanProcessor(exporter))
    test_tracer = provider.get_tracer("test")
    monkeypatch.setattr(sidecar, "tracer", lambda: test_tracer)
    return exporter


def test_execute_tool_records_arguments_and_result(
    span_exporter: InMemorySpanExporter, monkeypatch: pytest.MonkeyPatch
) -> None:
    agent = _new_jaeger_sidecar_agent()
    agent.on_connect(FakeConn())  # pyright: ignore[reportArgumentType]
    monkeypatch.setattr(
        agent._mcp, "call_tool", _fake_call_tool({"services": ["frontend", "backend"]})
    )

    args = {"service": "frontend", "limit": 20}
    result = asyncio.run(agent._execute_tool("sess-1", "search_traces", args, "call-1"))

    assert result == {"services": ["frontend", "backend"]}
    span = _find_span(span_exporter, "sidecar.execute_tool")
    assert json.loads(span.attributes["gen_ai.tool.call.arguments"]) == args
    assert json.loads(span.attributes["gen_ai.tool.call.result"]) == {
        "services": ["frontend", "backend"]
    }


def test_execute_contextual_tool_records_arguments_but_not_synthetic_ack(
    span_exporter: InMemorySpanExporter,
) -> None:
    agent = _new_jaeger_sidecar_agent()
    ext_response = {"acknowledged": True, "requestId": "abc"}
    agent.on_connect(FakeConn(ext_method_response=ext_response))  # pyright: ignore[reportArgumentType]

    args = {"query": "error rate"}
    result = asyncio.run(
        agent._execute_contextual_tool("sess-1", "frontend_widget_tool", args, "call-2")
    )

    assert result == ext_response
    span = _find_span(span_exporter, "sidecar.execute_contextual_tool")
    assert json.loads(span.attributes["gen_ai.tool.call.arguments"]) == args
    # The gateway's ack is not the tool's real output (fire-and-forget; see
    # _execute_contextual_tool's docstring), so it must not be mislabeled as
    # gen_ai.tool.call.result.
    assert "gen_ai.tool.call.result" not in span.attributes


def test_execute_tool_truncates_oversized_result_on_span_only(
    span_exporter: InMemorySpanExporter, monkeypatch: pytest.MonkeyPatch
) -> None:
    from sidecar_helpers import MAX_SPAN_ATTR_CHARS

    agent = _new_jaeger_sidecar_agent()
    conn = FakeConn()
    agent.on_connect(conn)  # pyright: ignore[reportArgumentType]
    huge_output = {"text": "x" * (MAX_SPAN_ATTR_CHARS * 2)}
    monkeypatch.setattr(agent._mcp, "call_tool", _fake_call_tool(huge_output))

    result = asyncio.run(agent._execute_tool("sess-1", "search_traces", {}, "call-3"))

    # The full, untruncated payload still reaches the AG-UI wire.
    assert result == huge_output
    completed_update = conn.session_updates[-1]
    assert completed_update.raw_output == {"content": huge_output}

    # Only the span attribute is capped, to protect OTLP export from
    # arbitrarily large tool payloads.
    span = _find_span(span_exporter, "sidecar.execute_tool")
    result_attr = span.attributes["gen_ai.tool.call.result"]
    assert len(result_attr) <= MAX_SPAN_ATTR_CHARS + len("... [truncated, 99999999 chars total]")
    assert result_attr.endswith("chars total]")

