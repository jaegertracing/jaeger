# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

from __future__ import annotations

import asyncio
import contextlib
import json
from pathlib import Path
from typing import Any
from functools import partial
from unittest.mock import patch

import pytest
import websockets
from mcp.types import Implementation as MCPImplementation
from mcp.types import InitializeResult, ServerCapabilities

from acp import Agent, PROTOCOL_VERSION
from acp.schema import AgentCapabilities, Implementation, ListSessionsResponse, LoadSessionResponse, NewSessionResponse, PromptResponse
from acp.helpers import text_block, update_agent_message

import sidecar
from mcp_bridge import JaegerMCPBridge

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


class _FakeInitializeSession:
    """Stands in for mcp.ClientSession, returning a canned initialize result."""

    def __init__(self, instructions: str | None) -> None:
        self._instructions = instructions

    async def __aenter__(self) -> "_FakeInitializeSession":
        return self

    async def __aexit__(self, *exc_info: Any) -> None:
        return None

    async def initialize(self) -> InitializeResult:
        return InitializeResult(
            protocolVersion="2025-06-18",
            capabilities=ServerCapabilities(),
            serverInfo=MCPImplementation(name="fake-mcp-server", version="0"),
            instructions=self._instructions,
        )


@contextlib.asynccontextmanager
async def _fake_streamablehttp_client(url: str, **kwargs: Any):
    yield (None, None, None)


def _patch_tool_discovery(bridge: JaegerMCPBridge) -> Any:
    """Bypasses the real MCPToolset connection; tool discovery is not under test here."""

    async def fake_get_tools() -> list[Any]:
        return []

    return patch.object(bridge._toolset, "get_tools", side_effect=fake_get_tools)


def test_mcp_bridge_exposes_server_instructions() -> None:
    """Regression test for #8897: JaegerMCPBridge must surface the MCP
    server's `instructions` field from its initialize response, since
    MCPToolset's tool discovery does not expose it."""

    async def run() -> None:
        bridge = JaegerMCPBridge("http://example.invalid/mcp", timeout_sec=5.0)
        expected = "Always call list_traces before summarizing a service."

        with (
            _patch_tool_discovery(bridge),
            patch("mcp_bridge.streamablehttp_client", _fake_streamablehttp_client),
            patch("mcp_bridge.ClientSession", lambda *args, **kwargs: _FakeInitializeSession(expected)),
        ):
            await bridge.initialize()

        assert bridge.instructions == expected

    asyncio.run(run())


def test_mcp_bridge_instructions_default_to_empty_string() -> None:
    """An MCP server with no `instructions` field should not break the bridge."""

    async def run() -> None:
        bridge = JaegerMCPBridge("http://example.invalid/mcp", timeout_sec=5.0)

        with (
            _patch_tool_discovery(bridge),
            patch("mcp_bridge.streamablehttp_client", _fake_streamablehttp_client),
            patch("mcp_bridge.ClientSession", lambda *args, **kwargs: _FakeInitializeSession(None)),
        ):
            await bridge.initialize()

        assert bridge.instructions == ""

    asyncio.run(run())


def test_mcp_bridge_instructions_handshake_failure_is_non_fatal() -> None:
    """If the extra initialize handshake fails, tool discovery must still
    succeed and `instructions` should fall back to an empty string rather
    than raising."""

    async def run() -> None:
        bridge = JaegerMCPBridge("http://example.invalid/mcp", timeout_sec=5.0)

        @contextlib.asynccontextmanager
        async def _broken_streamablehttp_client(url: str, **kwargs: Any):
            raise RuntimeError("connection refused")
            yield  # pragma: no cover - unreachable, keeps this an async generator

        with (
            _patch_tool_discovery(bridge),
            patch("mcp_bridge.streamablehttp_client", _broken_streamablehttp_client),
        ):
            await bridge.initialize()

        assert bridge.instructions == ""

    asyncio.run(run())

