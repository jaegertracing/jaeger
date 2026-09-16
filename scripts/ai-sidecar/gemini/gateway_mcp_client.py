# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

import asyncio
import logging
from contextlib import AsyncExitStack
from typing import Any

from google.genai import types
from mcp import ClientSession
from mcp.client.streamable_http import streamablehttp_client
from opentelemetry.semconv._incubating.attributes.gen_ai_attributes import GEN_AI_TOOL_NAME
from opentelemetry.semconv.attributes.url_attributes import URL_FULL
from opentelemetry.trace import Status, StatusCode

from sidecar_helpers import _to_jsonable
from tracing import tracer


logger = logging.getLogger(__name__)


class GatewayMCPClient:
    """MCP client for one chat turn, dialing the endpoint the gateway announced.

    The gateway hosts every tool this sidecar may call — Jaeger's telemetry tools
    and the browser's per-turn UI tools alike — behind a single turn-scoped MCP
    endpoint, and announces its URL in ``session/new``. So the sidecar holds no
    Jaeger address of its own and needs no second dispatch path: whatever the
    endpoint advertises is the whole tool surface for the turn.

    The instance is per-turn because the URL is: the announced endpoint carries a
    turn-scoped route id and is only valid while that turn is live. ``close``
    releases the session; the caller runs it in a ``finally`` so a failed turn
    does not leak the connection.
    """

    def __init__(self, url: str, headers: dict[str, str], timeout_sec: float):
        self._url = url
        # headers carries whatever the announcement attached — under multi-tenancy
        # that is the tenant header the endpoint requires, without which every
        # call comes back 401.
        self._headers = headers
        self._timeout_sec = timeout_sec
        self._stack: AsyncExitStack | None = None
        self._session: ClientSession | None = None
        self._tools_by_name: dict[str, Any] = {}
        self._gemini_tools: list[types.Tool] = []

    async def initialize(self) -> None:
        """Open the MCP session and discover the turn's tool surface, once."""
        if self._session is not None:
            return

        with tracer().start_as_current_span("mcp.discover_tools", attributes={
            URL_FULL: self._url,
        }) as span:
            logger.info(
                "Connecting to gateway MCP endpoint %s (timeout %.1fs)",
                self._url,
                self._timeout_sec,
            )
            stack = AsyncExitStack()
            try:
                read, write, _ = await stack.enter_async_context(
                    streamablehttp_client(url=self._url, headers=self._headers)
                )
                session = await stack.enter_async_context(ClientSession(read, write))
                await asyncio.wait_for(session.initialize(), timeout=self._timeout_sec)
                listed = await asyncio.wait_for(session.list_tools(), timeout=self._timeout_sec)
            except asyncio.CancelledError:
                await stack.aclose()
                span.set_status(Status(StatusCode.ERROR, description="cancelled"))
                logger.warning(
                    "MCP tool discovery cancelled before completion "
                    "(client likely disconnected mid-turn)."
                )
                raise
            except Exception as exc:
                await stack.aclose()
                message = f"Unable to connect to the gateway MCP endpoint at {self._url}."
                span.record_exception(exc)
                span.set_status(Status(StatusCode.ERROR, description=message))
                logger.error("%s Error: %s", message, exc)
                raise RuntimeError(message) from exc

            self._stack = stack
            self._session = session
            self._tools_by_name = {tool.name: tool for tool in listed.tools}
            logger.info("Retrieved tools from the gateway: %s", list(self._tools_by_name))

            declarations = [_to_function_declaration(tool) for tool in listed.tools]
            self._gemini_tools = (
                [types.Tool(function_declarations=declarations)] if declarations else []
            )

    async def get_gemini_tools(self) -> list[types.Tool]:
        await self.initialize()
        return self._gemini_tools

    async def call_tool(self, name: str, args: dict[str, Any]) -> Any:
        await self.initialize()

        with tracer().start_as_current_span("mcp.call_tool", attributes={
            GEN_AI_TOOL_NAME: name,
        }) as span:
            if name not in self._tools_by_name:
                span.set_status(Status(StatusCode.ERROR, description=f"unsupported tool: {name}"))
                return {"error": f"unsupported tool: {name}"}

            session = self._session
            if session is None:  # unreachable: initialize() above sets it or raises
                raise RuntimeError("MCP session is not initialized")

            try:
                result = await session.call_tool(name, args or {})
                return _to_jsonable(result)
            except Exception as e:
                span.record_exception(e)
                span.set_status(Status(StatusCode.ERROR, description=str(e)))
                raise

    async def close(self) -> None:
        """Release the MCP session. Idempotent, so a turn that failed before
        connecting closes to nothing."""
        stack, self._stack = self._stack, None
        self._session = None
        self._tools_by_name = {}
        self._gemini_tools = []
        if stack is not None:
            await stack.aclose()


def _to_function_declaration(tool: Any) -> types.FunctionDeclaration:
    """Translate an MCP tool into the Gemini function declaration that offers it.

    MCP publishes each tool's arguments as a JSON Schema, which is exactly what
    ``parameters_json_schema`` takes, so the shape passes through unchanged and
    the gateway stays the only definition of what a tool accepts.
    """
    schema = getattr(tool, "inputSchema", None)
    if not isinstance(schema, dict):
        schema = {"type": "object"}
    return types.FunctionDeclaration(
        name=tool.name,
        description=getattr(tool, "description", None) or "",
        parameters_json_schema=schema,
    )
