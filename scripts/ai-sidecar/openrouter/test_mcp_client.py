# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

"""Integration tests for JaegerMCPClient against a real MCP server.

These run an actual streamable-HTTP MCP server in-process rather than faking
the transport, because the bugs worth catching here live in the transport's
task and cancel-scope handling — exactly what a fake would paper over.
"""

from __future__ import annotations

import asyncio
import socket
from collections.abc import AsyncIterator
from typing import Any

import pytest
import uvicorn
from mcp.server.mcpserver import MCPServer

from mcp_client import JaegerMCPClient

SERVER_INSTRUCTIONS = "Start at SKILL.md and follow the wiki links to the right skill."


def _free_port() -> int:
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        return int(s.getsockname()[1])


def build_server() -> MCPServer:
    server = MCPServer(name="jaeger-test", instructions=SERVER_INSTRUCTIONS)

    @server.tool()
    def get_services(limit: int = 10) -> str:
        """List services."""
        return f"services(limit={limit})"

    @server.tool()
    def always_fails() -> str:
        """Raise, to exercise the failure path."""
        raise RuntimeError("tool blew up")

    return server


@pytest.fixture
async def mcp_url() -> AsyncIterator[str]:
    port = _free_port()
    app = build_server().streamable_http_app()
    config = uvicorn.Config(app, host="127.0.0.1", port=port, log_level="warning", lifespan="on")
    uv_server = uvicorn.Server(config)
    task = asyncio.create_task(uv_server.serve())

    for _ in range(200):
        if uv_server.started:
            break
        await asyncio.sleep(0.05)
    else:  # pragma: no cover - only on a pathologically slow machine
        uv_server.should_exit = True
        await task
        pytest.fail("MCP test server did not start")

    try:
        yield f"http://127.0.0.1:{port}/mcp"
    finally:
        uv_server.should_exit = True
        await task


async def test_connect_exposes_tools_and_instructions(mcp_url: str) -> None:
    client = JaegerMCPClient(mcp_url)
    try:
        await client.connect()

        assert client.instructions == SERVER_INSTRUCTIONS
        names = {t["function"]["name"] for t in client.openai_tools()}
        assert {"get_services", "always_fails"} <= names
    finally:
        await client.close()


async def test_tool_schema_is_forwarded_verbatim(mcp_url: str) -> None:
    """Whatever the server publishes is what the model is shown."""
    client = JaegerMCPClient(mcp_url)
    try:
        await client.connect()
        tool = next(t for t in client.openai_tools() if t["function"]["name"] == "get_services")
        params = tool["function"]["parameters"]

        assert params["type"] == "object"
        assert params["properties"]["limit"]["type"] == "integer"
    finally:
        await client.close()


async def test_call_tool_joins_text_content(mcp_url: str) -> None:
    client = JaegerMCPClient(mcp_url)
    try:
        await client.connect()
        assert await client.call_tool("get_services", {"limit": 3}) == "services(limit=3)"
    finally:
        await client.close()


async def test_close_from_a_different_task_than_connect(mcp_url: str) -> None:
    """Regression: ACP dispatches every request in its own task.

    A connection opened during `session/prompt` is closed during
    `session/close`, so the two calls land in different tasks. Holding the
    transport's anyio cancel scopes across that boundary used to fail with
    "Attempted to exit cancel scope in a different task than it was entered
    in", which surfaced to the gateway as an Internal error on every turn.
    """
    client = JaegerMCPClient(mcp_url)

    async def opener() -> None:
        await client.connect()
        await client.call_tool("get_services", {})

    async def closer() -> None:
        await client.close()

    await asyncio.create_task(opener())
    await asyncio.create_task(closer())

    assert client._session is None
    assert client._task is None


async def test_close_is_idempotent(mcp_url: str) -> None:
    client = JaegerMCPClient(mcp_url)
    await client.connect()
    await client.close()
    await client.close()


async def test_connect_is_idempotent(mcp_url: str) -> None:
    client = JaegerMCPClient(mcp_url)
    try:
        await client.connect()
        first = client._task
        await client.connect()
        assert client._task is first
    finally:
        await client.close()


async def test_connect_failure_propagates_and_leaves_no_task() -> None:
    """A dead endpoint must raise here, not silently degrade to zero tools."""
    client = JaegerMCPClient(f"http://127.0.0.1:{_free_port()}/mcp", timeout_sec=2.0)

    with pytest.raises(Exception):
        await client.connect()

    assert client._task is None
    await client.close()


async def test_call_tool_before_connect_raises(mcp_url: str) -> None:
    client = JaegerMCPClient(mcp_url)
    with pytest.raises(RuntimeError, match="not connected"):
        await client.call_tool("get_services", {})


async def test_failing_tool_surfaces_as_content(mcp_url: str) -> None:
    """MCP reports tool errors in the result, so the text reaches the model."""
    client = JaegerMCPClient(mcp_url)
    try:
        await client.connect()
        out: Any = await client.call_tool("always_fails", {})
        assert "blew up" in out or out == "(empty result)"
    finally:
        await client.close()
