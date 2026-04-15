# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

import asyncio
import logging
from typing import Any

from google.adk.tools.mcp_tool import MCPToolset, StreamableHTTPConnectionParams
from google.genai import types

from sidecar_helpers import _extract_function_declaration, _to_jsonable


logger = logging.getLogger(__name__)

# Name of the MCP tool whose `session_id` argument must be supplied by the
# sidecar (from the in-flight ACP session) rather than by the model.
CONTEXTUAL_TOOLS_TOOL_NAME = "list_contextual_tools"
CONTEXTUAL_TOOLS_SESSION_ARG = "session_id"


def _strip_session_id_from_declaration(declaration: types.FunctionDeclaration) -> None:
    """Hide the `session_id` parameter from the Gemini-facing declaration.

    The MCP tool `list_contextual_tools` requires `session_id` so the backend
    can return the correct per-turn snapshot, but that value is owned by the
    sidecar (it is the ACP session id) and must never be chosen by the model.
    Removing it from `properties` and `required` keeps Gemini from trying to
    populate it; the bridge injects the real value on call.
    """
    params = getattr(declaration, "parameters", None)
    if params is None:
        return

    properties = getattr(params, "properties", None)
    if isinstance(properties, dict):
        properties.pop(CONTEXTUAL_TOOLS_SESSION_ARG, None)

    required = getattr(params, "required", None)
    if isinstance(required, list):
        params.required = [name for name in required if name != CONTEXTUAL_TOOLS_SESSION_ARG]


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
            logger.error("%s Error: %s", message, exc)
            raise RuntimeError(message) from exc

        self._tools_by_name = {tool.name: tool for tool in adk_tools}
        logger.info("Retrieved tools from MCP: %s", list(self._tools_by_name.keys()))

        function_declarations: list[types.FunctionDeclaration] = []
        for tool in adk_tools:
            declaration = _extract_function_declaration(tool)
            if declaration is None:
                continue
            if declaration.name == CONTEXTUAL_TOOLS_TOOL_NAME:
                _strip_session_id_from_declaration(declaration)
            function_declarations.append(declaration)

        if function_declarations:
            self._gemini_tools = [types.Tool(function_declarations=function_declarations)]
        else:
            self._gemini_tools = []

        self._initialized = True

    async def get_gemini_tools(self) -> list[types.Tool]:
        await self.initialize()
        return self._gemini_tools

    async def call_tool(
        self,
        name: str,
        args: dict[str, Any],
        acp_session_id: str | None = None,
    ) -> Any:
        await self.initialize()

        tool = self._tools_by_name.get(name)
        if tool is None:
            return {"error": f"unsupported tool: {name}"}

        # The backend MCP tool `list_contextual_tools` keys its snapshot on the
        # ACP session id. The sidecar is the only component that knows that id
        # for the in-flight turn, so we always override whatever the model may
        # have supplied with the authoritative value from the ACP handler.
        call_args: dict[str, Any] = dict(args or {})
        if name == CONTEXTUAL_TOOLS_TOOL_NAME and acp_session_id:
            call_args[CONTEXTUAL_TOOLS_SESSION_ARG] = acp_session_id

        result = await tool.run_async(args=call_args, tool_context=None)
        return _to_jsonable(result)
