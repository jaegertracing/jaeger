# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

import asyncio
import logging
from typing import Any

from google.adk.tools.mcp_tool import MCPToolset, StreamableHTTPConnectionParams
from google.genai import types
from opentelemetry.trace import Status, StatusCode

from sidecar_helpers import _extract_function_declaration, _to_jsonable
from tracing import get_tracer


logger = logging.getLogger(__name__)


class JaegerMCPBridge:
    """Loads MCP tools once and exposes them for Gemini tool-calling."""

    def __init__(self, mcp_url: str, timeout_sec: float):
        self._mcp_url = mcp_url
        self._timeout_sec = timeout_sec
        self._toolset = MCPToolset(
            connection_params=StreamableHTTPConnectionParams(url=mcp_url),
        )
        self._tools_by_name: dict[str, Any] = {}
        self._gemini_tools: list[types.Tool] = []
        self._initialized = False

    async def initialize(self) -> None:
        if self._initialized:
            return

        tracer = get_tracer()
        with tracer.start_as_current_span("mcp.discover_tools", attributes={
            "mcp.url": self._mcp_url,
        }) as span:
            # Discover available MCP tools once, then expose them to Gemini as function declarations.
            logger.info(
                f"Initializing MCP tool discovery from {self._mcp_url} "
                f"(single attempt timeout={self._timeout_sec}s)"
            )

            try:
                logger.info(
                    f"MCP connection trying {self._mcp_url} "
                    f"(timeout {self._timeout_sec:.1f}s)"
                )
                adk_tools = await asyncio.wait_for(self._toolset.get_tools(), timeout=self._timeout_sec)
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

            self._tools_by_name = {tool.name: tool for tool in adk_tools}
            logger.info("Retrieved tools from MCP: %s", list(self._tools_by_name.keys()))
            span.set_attribute("mcp.tool_count", len(self._tools_by_name))

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

        tracer = get_tracer()
        with tracer.start_as_current_span("mcp.call_tool", attributes={
            "tool.name": name,
        }) as span:
            tool = self._tools_by_name.get(name)
            if tool is None:
                span.set_status(Status(StatusCode.ERROR, description=f"unsupported tool: {name}"))
                return {"error": f"unsupported tool: {name}"}

            try:
                result = await tool.run_async(args=args or {}, tool_context=None)
                return _to_jsonable(result)
            except Exception as e:
                span.record_exception(e)
                span.set_status(Status(StatusCode.ERROR, description=str(e)))
                raise
