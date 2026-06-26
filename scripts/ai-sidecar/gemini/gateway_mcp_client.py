# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

"""Per-session MCP client that dials the Jaeger AI gateway's MCP endpoint.

This is the IoC-clean replacement for the previous ``JaegerMCPBridge``,
which spoke MCP directly to ``jaeger_mcp`` and so kept tool dispatch
outside the gateway's tracing/auth path. The new client points at the
per-turn URL the gateway announces in ``NewSessionRequest.mcpServers``
— so UI tools and telemetry tools both flow through the same gateway-
owned MCP server, and the sidecar carries no per-transport routing
glue.

Each chat turn gets its own ``GatewayMCPClient``:

 1. The chat handler announces ``mcpServers = [{type:"http", url:
    "http://.../api/ai/mcp/<uuid>/"}]`` in session/new.
 2. ``JaegerSidecarAgent.new_session`` stores the URL in a per-session
    map; no network I/O happens here.
 3. On ``session/prompt`` the agentic loop calls
    ``GatewayMCPClient.initialize()``, which opens the streamable HTTP
    transport, completes the inner-MCP handshake, and caches
    ``tools/list`` once.
 4. Each ``call_tool(name, args)`` is forwarded as an MCP ``tools/call``
    on the same session.
 5. ``aclose()`` runs in the turn's finally so the HTTP keepalive and
    background reader task don't leak.
"""

from __future__ import annotations

import asyncio
import logging
from contextlib import AsyncExitStack
from typing import Any

from google.genai import types
from mcp import ClientSession
from mcp.client.streamable_http import streamable_http_client
from opentelemetry.semconv._incubating.attributes.gen_ai_attributes import GEN_AI_TOOL_NAME
from opentelemetry.semconv.attributes.url_attributes import URL_FULL
from opentelemetry.trace import Status, StatusCode

from sidecar_helpers import _to_jsonable
from tracing import tracer


logger = logging.getLogger(__name__)


class GatewayMCPClient:
    """Per-session MCP client pointed at the gateway's announced URL.

    Lifecycle:
        client = GatewayMCPClient(url, timeout_sec)
        await client.initialize()      # opens the transport + tools/list
        tools = await client.get_gemini_tools()
        result = await client.call_tool("ui_highlight_span", {...})
        await client.aclose()          # idempotent

    Not safe to share across asyncio Tasks; one client per chat turn.
    """

    def __init__(self, mcp_url: str, timeout_sec: float):
        self._mcp_url = mcp_url
        self._timeout_sec = timeout_sec
        # AsyncExitStack lets us layer the streamable-http transport
        # context manager and the ClientSession context manager under
        # one aclose. Otherwise we'd have to track two `__aexit__` calls
        # and order them correctly on every error path.
        self._stack: AsyncExitStack | None = None
        self._session: ClientSession | None = None
        self._tools_by_name: dict[str, Any] = {}
        self._gemini_tools: list[types.Tool] = []
        self._initialized = False
        # Re-entrancy guard: get_gemini_tools and call_tool both
        # initialize-on-first-use, and Gemini's agentic loop may invoke
        # them from nested awaits within one turn.
        self._init_lock = asyncio.Lock()

    async def initialize(self) -> None:
        """Open the MCP session and cache the tool list. Idempotent —
        subsequent calls are no-ops so callers can guard themselves
        without coordinating."""
        async with self._init_lock:
            if self._initialized:
                return

            with tracer().start_as_current_span(
                "gateway_mcp.discover_tools",
                attributes={URL_FULL: self._mcp_url},
            ) as span:
                logger.info(
                    "Opening MCP session against gateway URL %s (timeout=%.1fs)",
                    self._mcp_url,
                    self._timeout_sec,
                )
                stack = AsyncExitStack()
                try:
                    # The streamable-http client returns (read, write, _),
                    # where the third value is a future for the spec's
                    # post-handshake "extra" header info. We don't need it.
                    read_stream, write_stream, _ = await asyncio.wait_for(
                        stack.enter_async_context(streamable_http_client(self._mcp_url)),
                        timeout=self._timeout_sec,
                    )
                    session = await stack.enter_async_context(ClientSession(read_stream, write_stream))
                    await asyncio.wait_for(session.initialize(), timeout=self._timeout_sec)
                    listed = await asyncio.wait_for(session.list_tools(), timeout=self._timeout_sec)
                except asyncio.CancelledError:
                    await stack.aclose()
                    span.set_status(Status(StatusCode.ERROR, description="cancelled"))
                    logger.warning(
                        "MCP discovery cancelled before completion (client likely disconnected)"
                    )
                    raise
                except Exception as exc:
                    await stack.aclose()
                    message = (
                        f"Unable to reach Jaeger gateway MCP at {self._mcp_url}. "
                        "Stopping request."
                    )
                    span.record_exception(exc)
                    span.set_status(Status(StatusCode.ERROR, description=message))
                    logger.error("%s Error: %s", message, exc)
                    raise RuntimeError(message) from exc

                self._stack = stack
                self._session = session
                self._tools_by_name = {tool.name: tool for tool in listed.tools}
                self._gemini_tools = _build_gemini_tools(listed.tools)
                self._initialized = True

                logger.info(
                    "MCP tools discovered: %s",
                    sorted(self._tools_by_name.keys()),
                )

    async def get_gemini_tools(self) -> list[types.Tool]:
        await self.initialize()
        return self._gemini_tools

    async def call_tool(self, name: str, args: dict[str, Any]) -> Any:
        await self.initialize()
        assert self._session is not None, "initialize() guarantees a live session"

        with tracer().start_as_current_span(
            "gateway_mcp.call_tool",
            attributes={GEN_AI_TOOL_NAME: name},
        ) as span:
            if name not in self._tools_by_name:
                span.set_status(Status(StatusCode.ERROR, description=f"unsupported tool: {name}"))
                return {"error": f"unsupported tool: {name}"}
            try:
                result = await self._session.call_tool(name, arguments=args or {})
            except Exception as exc:
                span.record_exception(exc)
                span.set_status(Status(StatusCode.ERROR, description=str(exc)))
                raise
            return _to_jsonable(result)

    async def aclose(self) -> None:
        """Close the transport + session and release the reader task.
        Idempotent so a chat turn's ``finally`` block can call this
        unconditionally."""
        stack = self._stack
        self._stack = None
        self._session = None
        self._initialized = False
        if stack is not None:
            await stack.aclose()


def _build_gemini_tools(mcp_tools: Any) -> list[types.Tool]:
    """Translate each MCP Tool into a Gemini FunctionDeclaration and
    bundle them into a single Tool. An empty list returns an empty
    Gemini tools list so the agentic loop can branch cleanly."""
    declarations: list[types.FunctionDeclaration] = []
    for tool in mcp_tools:
        # MCP Tool.inputSchema is a JSON Schema dict; Gemini accepts it
        # directly via parameters_json_schema. A few MCP servers ship
        # tools with no schema at all — coerce to a permissive empty
        # object so registration succeeds.
        schema = tool.inputSchema or {"type": "object"}
        description = tool.description or ""
        declarations.append(
            types.FunctionDeclaration(
                name=tool.name,
                description=description,
                parameters_json_schema=schema,
            )
        )
    if not declarations:
        return []
    return [types.Tool(function_declarations=declarations)]


