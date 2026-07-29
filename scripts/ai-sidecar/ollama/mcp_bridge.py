# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

import asyncio
import logging
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager
from typing import Any

from mcp import ClientSession
from mcp.client.streamable_http import streamablehttp_client
from mcp.types import CallToolResult, TextContent, Tool
from opentelemetry.semconv._incubating.attributes.gen_ai_attributes import GEN_AI_TOOL_NAME
from opentelemetry.semconv.attributes.url_attributes import URL_FULL
from opentelemetry.trace import Status, StatusCode

from sidecar_helpers import _build_ollama_tool, _to_jsonable
from tracing import tracer


logger = logging.getLogger(__name__)


def _to_ollama_tool(tool: Tool) -> dict[str, Any]:
    """Advertise an MCP tool to Ollama.

    MCP's ``inputSchema`` is already JSON Schema, which is the shape Ollama
    wants, so this is an envelope and nothing more.
    """
    return _build_ollama_tool(
        name=tool.name,
        description=tool.description or "",
        parameters=tool.inputSchema,
    )


def _tool_result_output(result: CallToolResult) -> Any:
    """Reduce an MCP tool result to something a model can read.

    Prefers ``structuredContent`` when the tool provides it, then falls back to
    concatenated text blocks, then to the raw serialized content for exotic
    block types (images, embedded resources).
    """
    if result.structuredContent is not None:
        return result.structuredContent

    texts = [block.text for block in result.content if isinstance(block, TextContent)]
    if texts:
        return "\n".join(texts)

    return [_to_jsonable(block) for block in result.content]


class JaegerMCPBridge:
    """Discovers Jaeger MCP tools once and exposes them for Ollama tool-calling.

    A fresh MCP session is opened per operation rather than held open for the
    sidecar's lifetime. The streamable-HTTP client is built on anyio cancel
    scopes, which must be entered and exited in the same task; a long-lived
    session shared across the ACP runtime's per-request tasks violates that.
    Discovery results are cached, so the per-call cost is one short-lived HTTP
    session against a server that is normally in the same pod.
    """

    def __init__(self, mcp_url: str, timeout_sec: float):
        self._mcp_url = mcp_url
        self._timeout_sec = timeout_sec
        self._tool_names: set[str] = set()
        self._ollama_tools: list[dict[str, Any]] = []
        self._initialized = False

    @asynccontextmanager
    async def _connect(self) -> AsyncIterator[ClientSession]:
        async with streamablehttp_client(self._mcp_url) as (read_stream, write_stream, _get_session_id):
            async with ClientSession(read_stream, write_stream) as session:
                await session.initialize()
                yield session

    async def _discover(self) -> list[Tool]:
        async with self._connect() as session:
            listed = await session.list_tools()
            return listed.tools

    async def initialize(self) -> None:
        if self._initialized:
            return

        with tracer().start_as_current_span("mcp.discover_tools", attributes={
            URL_FULL: self._mcp_url,
        }) as span:
            # Discover available MCP tools once, then expose them to the model.
            logger.info(
                f"Initializing MCP tool discovery from {self._mcp_url} "
                f"(single attempt timeout={self._timeout_sec}s)"
            )

            try:
                tools = await asyncio.wait_for(self._discover(), timeout=self._timeout_sec)
            except asyncio.CancelledError:
                span.set_status(Status(StatusCode.ERROR, description="cancelled"))
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
                span.record_exception(exc)
                span.set_status(Status(StatusCode.ERROR, description=message))
                logger.error("%s Error: %s", message, exc)
                raise RuntimeError(message) from exc

            self._tool_names = {tool.name for tool in tools}
            self._ollama_tools = [_to_ollama_tool(tool) for tool in tools]
            logger.info("Retrieved tools from MCP: %s", sorted(self._tool_names))

            self._initialized = True

    async def get_ollama_tools(self) -> list[dict[str, Any]]:
        await self.initialize()
        return self._ollama_tools

    async def call_tool(self, name: str, args: dict[str, Any]) -> Any:
        await self.initialize()

        with tracer().start_as_current_span("mcp.call_tool", attributes={
            GEN_AI_TOOL_NAME: name,
        }) as span:
            if name not in self._tool_names:
                span.set_status(Status(StatusCode.ERROR, description=f"unsupported tool: {name}"))
                return {"error": f"unsupported tool: {name}"}

            try:
                async with self._connect() as session:
                    result = await session.call_tool(name, args or {})
            except Exception as e:
                span.record_exception(e)
                span.set_status(Status(StatusCode.ERROR, description=str(e)))
                raise

            output = _tool_result_output(result)
            if result.isError:
                # A tool-level error is the tool's answer, not a transport
                # failure: hand it back so the model can retry with different
                # arguments or explain the problem, but mark the span failed.
                span.set_status(Status(StatusCode.ERROR, description=str(output)))
                logger.warning("MCP tool %s reported an error: %s", name, output)
            return output
