# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

from __future__ import annotations

import asyncio
import json
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager
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
from mcp.types import CallToolResult, ListToolsResult, PaginatedRequestParams, Tool

import gateway_mcp_client
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

    async def prompt(self, session_id: str, prompt: list[Any], message_id: str | None = None, **kwargs: Any) -> PromptResponse:
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
    """Minimal stand-in for the ACP Client the runtime passes to on_connect.

    It still records session_update calls, because the tests assert the sidecar
    no longer emits any around a tool call — the gateway is the single source of
    the TOOL_CALL_* events now.
    """

    def __init__(self) -> None:
        self.session_updates: list[Any] = []

    async def session_update(self, session_id: str, update: Any, **kwargs: Any) -> None:
        self.session_updates.append(update)


class FakeMCPClient:
    """Stand-in for the per-turn GatewayMCPClient, covering the surface
    _execute_tool uses."""

    def __init__(self, tool_output: Any) -> None:
        self._tool_output = tool_output
        self.closed = False

    async def call_tool(self, name: str, args: dict[str, Any]) -> Any:
        return self._tool_output

    async def aclose(self) -> None:
        self.closed = True


def _agent_with_tool_output(tool_output: Any) -> tuple[sidecar.JaegerSidecarAgent, FakeConn]:
    """An agent whose session "sess-1" has a live MCP client returning tool_output."""
    agent = _new_jaeger_sidecar_agent()
    conn = FakeConn()
    agent.on_connect(conn)  # pyright: ignore[reportArgumentType]
    agent._mcp_clients["sess-1"] = FakeMCPClient(tool_output)  # pyright: ignore[reportArgumentType]
    return agent, conn


def _new_jaeger_sidecar_agent() -> sidecar.JaegerSidecarAgent:
    config = SidecarConfig(
        gemini_api_key="test-key",
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
    monkeypatch.setattr(gateway_mcp_client, "tracer", lambda: test_tracer)
    return exporter


def test_execute_tool_records_arguments_and_result(
    span_exporter: InMemorySpanExporter,
) -> None:
    agent, _ = _agent_with_tool_output({"services": ["frontend", "backend"]})

    args = {"service": "frontend", "limit": 20}
    result = asyncio.run(agent._execute_tool("sess-1", "search_traces", args, "call-1"))

    assert result == {"services": ["frontend", "backend"]}
    span = _find_span(span_exporter, "sidecar.execute_tool")
    assert json.loads(span.attributes["gen_ai.tool.call.arguments"]) == args
    assert json.loads(span.attributes["gen_ai.tool.call.result"]) == {
        "services": ["frontend", "backend"]
    }


def test_execute_tool_emits_no_session_updates(span_exporter: InMemorySpanExporter) -> None:
    """The gateway owns the TOOL_CALL_* SSE for every tool now that all calls go
    through its endpoint. Emitting from here too would double each event on the
    AG-UI wire, so the sidecar must stay silent."""
    agent, conn = _agent_with_tool_output({"services": []})

    asyncio.run(agent._execute_tool("sess-1", "search_traces", {}, "call-1"))

    assert conn.session_updates == []


def test_execute_tool_without_announced_endpoint_fails_loudly(
    span_exporter: InMemorySpanExporter,
) -> None:
    """A session the gateway announced no MCP endpoint for has no tools to call.
    Failing here beats returning a silent empty result that Gemini would narrate
    as a real answer."""
    agent = _new_jaeger_sidecar_agent()
    agent.on_connect(FakeConn())  # pyright: ignore[reportArgumentType]

    with pytest.raises(RuntimeError, match="no MCP endpoint was announced"):
        asyncio.run(agent._execute_tool("sess-unknown", "search_traces", {}, "call-1"))


def test_execute_tool_truncates_oversized_result_on_span_only(
    span_exporter: InMemorySpanExporter,
) -> None:
    from sidecar_helpers import MAX_SPAN_ATTR_CHARS

    huge_output = {"text": "x" * (MAX_SPAN_ATTR_CHARS * 2)}
    agent, _ = _agent_with_tool_output(huge_output)

    result = asyncio.run(agent._execute_tool("sess-1", "search_traces", {}, "call-3"))

    # The full, untruncated payload still reaches the LLM loop.
    assert result == huge_output

    # Only the span attribute is capped, to protect OTLP export from
    # arbitrarily large tool payloads.
    span = _find_span(span_exporter, "sidecar.execute_tool")
    result_attr = span.attributes["gen_ai.tool.call.result"]
    assert len(result_attr) <= MAX_SPAN_ATTR_CHARS
    assert result_attr.endswith("chars total]")


# --- the gateway announcement: session/new -> the turn's MCP endpoint ---------


def test_initialize_advertises_http_mcp_capability() -> None:
    """ACP requires an agent to opt into each McpServer variant, and the gateway
    announces nothing to an agent that did not. Without mcpCapabilities.http the
    sidecar would receive no mcpServers and every turn would silently run with
    zero tools — so this is the capability the whole design hangs on."""
    agent = _new_jaeger_sidecar_agent()

    response = asyncio.run(agent.initialize(PROTOCOL_VERSION))

    caps = response.agent_capabilities
    assert caps is not None and caps.mcp_capabilities is not None
    assert caps.mcp_capabilities.http is True


def test_announced_mcp_endpoint_skips_sse() -> None:
    """An SSE announcement carries the same url/headers shape as HTTP but speaks a
    different protocol, so dialing it with the streamable-HTTP client would fail
    at connect. Only the http variant is consumed."""
    from sidecar_helpers import _announced_mcp_endpoint

    assert _announced_mcp_endpoint([
        {"type": "sse", "name": "jaeger", "url": "http://x/sse", "headers": []}
    ]) is None


def test_announced_mcp_endpoint_reads_url_and_headers() -> None:
    """The announcement is the only thing telling the sidecar where to dial, and
    its headers are load-bearing: the endpoint sits behind jaeger-query's tenancy
    extraction, so a dropped tenant header turns every tool call into a 401."""
    from sidecar_helpers import _announced_mcp_endpoint

    announced = _announced_mcp_endpoint([
        {
            "type": "http",
            "name": "jaeger",
            "url": "http://127.0.0.1:16686/api/ai/mcp/route-1/",
            "headers": [{"name": "x-tenant", "value": "acme"}],
        }
    ])

    assert announced == (
        "http://127.0.0.1:16686/api/ai/mcp/route-1/",
        {"x-tenant": "acme"},
    )


def test_announced_mcp_endpoint_accepts_acp_models() -> None:
    """Depending on how the router decoded session/new, entries arrive as parsed
    ACP models rather than dicts; both must read the same."""
    from acp.schema import HttpMcpServer

    from sidecar_helpers import _announced_mcp_endpoint

    server = HttpMcpServer.model_validate({
        "type": "http",
        "name": "jaeger",
        "url": "http://127.0.0.1:16686/api/ai/mcp/route-2/",
        "headers": [{"name": "x-tenant", "value": "acme"}],
    })

    assert _announced_mcp_endpoint([server]) == (
        "http://127.0.0.1:16686/api/ai/mcp/route-2/",
        {"x-tenant": "acme"},
    )


@pytest.mark.parametrize(
    "mcp_servers",
    [
        pytest.param(None, id="nothing announced"),
        pytest.param([], id="empty announcement"),
        pytest.param([{"type": "stdio", "name": "x", "command": "y"}], id="stdio only"),
    ],
)
def test_announced_mcp_endpoint_absent(mcp_servers: Any) -> None:
    """No HTTP endpoint announced is the shape of a deployment without the MCP
    config block — the turn runs with no tools rather than failing at parse."""
    from sidecar_helpers import _announced_mcp_endpoint

    assert _announced_mcp_endpoint(mcp_servers) is None


def test_new_session_builds_client_from_announcement() -> None:
    agent = _new_jaeger_sidecar_agent()

    response = asyncio.run(
        agent.new_session(
            cwd="/",
            mcp_servers=[
                {
                    "type": "http",
                    "name": "jaeger",
                    "url": "http://127.0.0.1:16686/api/ai/mcp/route-1/",
                    "headers": [{"name": "x-tenant", "value": "acme"}],
                }
            ],
        )
    )

    client = agent._mcp_clients[response.session_id]
    assert client._url == "http://127.0.0.1:16686/api/ai/mcp/route-1/"
    assert client._headers == {"x-tenant": "acme"}


def test_new_session_without_announcement_registers_no_client() -> None:
    agent = _new_jaeger_sidecar_agent()

    response = asyncio.run(agent.new_session(cwd="/", mcp_servers=[]))

    assert response.session_id not in agent._mcp_clients


def test_prompt_closes_the_turns_mcp_client() -> None:
    """The gateway opens one ACP session per chat request and never reuses the id,
    so a client left open would leak an HTTP connection per turn. prompt closes it
    in finally, which also covers turns the gateway never closes (client
    disconnect mid-stream)."""
    agent, _ = _agent_with_tool_output({"services": []})
    client = agent._mcp_clients["sess-1"]

    asyncio.run(agent.prompt("sess-1", [text_block("hello")]))

    assert client.closed  # pyright: ignore[reportAttributeAccessIssue]
    assert "sess-1" not in agent._mcp_clients


def test_aclose_releases_clients_for_unfinished_sessions() -> None:
    """A session the gateway opened but never prompted never reaches prompt's
    finally, and session/close is not advertised — so the connection teardown is
    the only thing left to release its MCP client."""
    agent, _ = _agent_with_tool_output({"services": []})
    client = agent._mcp_clients["sess-1"]

    asyncio.run(agent.aclose())

    assert client.closed  # pyright: ignore[reportAttributeAccessIssue]
    assert agent._mcp_clients == {}


def test_gateway_http_client_uses_sdk_transport_defaults() -> None:
    """The client is built with the MCP SDK's own factory, so its transport
    defaults stay the SDK's. httpx alone would default every phase to 5s — capping
    the configured connect budget and timing out an idle SSE read while the
    gateway is still working — so both must be set, and the idle-read allowance
    must be the SDK's value rather than a copy of it."""
    from mcp.shared._httpx_utils import MCP_DEFAULT_SSE_READ_TIMEOUT

    from gateway_mcp_client import GatewayMCPClient

    client = GatewayMCPClient("http://x/", {"x-tenant": "acme"}, 15.0)._new_http_client()
    try:
        assert client.timeout.connect == 15.0, "the configured budget must reach the connect phase"
        assert client.timeout.read == MCP_DEFAULT_SSE_READ_TIMEOUT, (
            "the idle-read allowance is the SDK's, not a hand-copied constant"
        )
        assert client.headers["x-tenant"] == "acme", "announced headers must ride on the client"
        assert client.follow_redirects, "the SDK factory's defaults are kept"
    finally:
        asyncio.run(client.aclose())


class RecordingSession:
    """Stand-in for the MCP ClientSession that records the _meta each request
    carried to the gateway."""

    def __init__(self) -> None:
        self.list_meta: dict[str, Any] | None = None
        self.call_meta: dict[str, Any] | None = None

    async def __aenter__(self) -> RecordingSession:
        return self

    async def __aexit__(self, *exc: object) -> None:
        return None

    async def initialize(self) -> None:
        return None

    async def list_tools(self, *, params: PaginatedRequestParams | None = None) -> ListToolsResult:
        meta = params.meta if params else None
        self.list_meta = meta.model_dump(exclude_none=True) if meta else None
        return ListToolsResult(tools=[Tool(name="get_services", inputSchema={"type": "object"})])

    async def call_tool(
        self, name: str, arguments: dict[str, Any] | None = None, *, meta: dict[str, Any] | None = None
    ) -> CallToolResult:
        self.call_meta = meta
        return CallToolResult(content=[])


@asynccontextmanager
async def _fake_streamable_http_client(**_: Any) -> AsyncIterator[tuple[None, None, None]]:
    yield None, None, None


def test_mcp_requests_carry_the_span_that_sent_them(
    span_exporter: InMemorySpanExporter, monkeypatch: pytest.MonkeyPatch
) -> None:
    """The gateway's MCP middleware parents each method span on the trace context
    in the request's _meta. Without it the gateway starts a trace of its own, and
    the turn's trace shows the sidecar calling a tool but never the tool running."""
    session = RecordingSession()
    monkeypatch.setattr(gateway_mcp_client, "streamable_http_client", _fake_streamable_http_client)
    monkeypatch.setattr(gateway_mcp_client, "ClientSession", lambda read, write: session)
    client = gateway_mcp_client.GatewayMCPClient("http://x/", {}, 1.0)

    async def turn() -> None:
        try:
            await client.call_tool("get_services", {})
        finally:
            await client.aclose()

    asyncio.run(turn())

    def traceparent(span_name: str) -> str:
        ctx = _find_span(span_exporter, span_name).context
        return f"00-{ctx.trace_id:032x}-{ctx.span_id:016x}-{ctx.trace_flags:02x}"

    assert session.list_meta == {"traceparent": traceparent("mcp.discover_tools")}
    assert session.call_meta == {"traceparent": traceparent("mcp.call_tool")}
