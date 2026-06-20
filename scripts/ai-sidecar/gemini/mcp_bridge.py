# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

import asyncio
import json
import logging
from typing import Any

import requests
from google.adk.tools.mcp_tool import MCPToolset, StreamableHTTPConnectionParams
from google.genai import types
from opentelemetry.semconv._incubating.attributes.gen_ai_attributes import GEN_AI_TOOL_NAME
from opentelemetry.semconv.attributes.url_attributes import URL_FULL
from opentelemetry.trace import Status, StatusCode

from sidecar_helpers import _extract_function_declaration, _to_jsonable
from tracing import tracer


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
        self._server_instructions: str = ""
        self._initialized = False

    async def initialize(self) -> None:
        if self._initialized:
            return

        with tracer().start_as_current_span("mcp.discover_tools", attributes={
            URL_FULL: self._mcp_url,
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

            self._server_instructions = await asyncio.to_thread(
                self._fetch_server_instructions
            )

            self._tools_by_name = {tool.name: tool for tool in adk_tools}
            logger.info("Retrieved tools from MCP: %s", list(self._tools_by_name.keys()))

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

    def _fetch_server_instructions(self) -> str:
        """Fetch MCP server instructions via a raw initialize call.

        The ADK MCPToolset does not expose the server's `instructions` field,
        so we make a lightweight Streamable HTTP request to get it. This is
        generic MCP behavior — any MCP server can provide instructions.
        """
        session_id: str | None = None
        headers = {
            "Content-Type": "application/json",
            "Accept": "application/json, text/event-stream",
        }
        try:
            with requests.post(
                self._mcp_url,
                json={
                    "jsonrpc": "2.0",
                    "id": "instructions-probe",
                    "method": "initialize",
                    "params": {
                        "protocolVersion": "2025-03-26",
                        "capabilities": {},
                        "clientInfo": {"name": "jaeger-sidecar-instructions-probe", "version": "1"},
                    },
                },
                headers=headers,
                timeout=self._timeout_sec,
            ) as resp:
                session_id = resp.headers.get("Mcp-Session-Id")
                for line in resp.text.splitlines():
                    if line.startswith("data: "):
                        payload = json.loads(line[6:])
                        instructions = payload.get("result", {}).get("instructions", "")
                        if instructions:
                            logger.info("Fetched MCP server instructions (%d chars)", len(instructions))
                            return instructions
        except Exception as e:
            logger.warning("Failed to fetch MCP server instructions: %s", e)
        finally:
            if session_id:
                try:
                    requests.delete(
                        self._mcp_url,
                        headers={**headers, "Mcp-Session-Id": session_id},
                        timeout=self._timeout_sec,
                    )
                except Exception:
                    pass
        return ""

    async def get_server_instructions(self) -> str:
        await self.initialize()
        return self._server_instructions

    async def get_gemini_tools(self) -> list[types.Tool]:
        await self.initialize()
        return self._gemini_tools

    async def call_tool(self, name: str, args: dict[str, Any]) -> Any:
        await self.initialize()

        with tracer().start_as_current_span("mcp.call_tool", attributes={
            GEN_AI_TOOL_NAME: name,
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
