# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

"""Minimal MCP client over streamable HTTP.

The Gemini sidecar reaches MCP through google-adk's MCPToolset, which drags in
the whole ADK stack. This uses the reference `mcp` SDK directly, so nothing here
is tied to a model vendor.

Everything the server tells us is passed through untouched: tool JSON Schemas go
straight into the OpenAI `parameters` field with no massaging, and the server's
`instructions` text is handed to the caller verbatim. If a model chokes on a
shape jaegermcp emits, that is a finding about the model, not a bug to paper
over here.
"""

import asyncio
import logging
from contextlib import AsyncExitStack
from typing import Any

import httpx2
from mcp import ClientSession

# The SDK exports `streamable_http_client`. There is no `streamablehttp_client`
# in mcp>=2.0 — reaching for that name is the single most common wrong guess.
from mcp.client.streamable_http import streamable_http_client

logger = logging.getLogger(__name__)


class JaegerMCPClient:
    """Discovers Jaeger's MCP tools once per session and calls them on demand.

    One instance per ACP session. The gateway mints a fresh, turn-scoped MCP
    route per chat request and the skills behind it can change between sessions,
    so a process-wide cache would serve a stale catalog.
    """

    def __init__(self, mcp_url: str, headers: dict[str, str] | None = None, timeout_sec: float = 30.0):
        self._url = mcp_url
        self._headers = headers or {}
        self._timeout = timeout_sec
        self._session: ClientSession | None = None
        self._tools: list[Any] = []
        self._instructions: str | None = None
        # See _serve: the connection is owned by its own task.
        self._task: asyncio.Task[None] | None = None
        self._ready: asyncio.Future[None] | None = None
        self._close_requested: asyncio.Event | None = None

    @property
    def url(self) -> str:
        return self._url

    @property
    def instructions(self) -> str | None:
        """Server-provided usage guidance from the MCP initialize result.

        jaegermcp uses this to explain how to enter the skills catalog. It is
        the server talking to the model, so the caller must put it in front of
        the model rather than dropping it — see sidecar.py's system prompt.
        """
        return self._instructions

    async def _serve(self) -> None:
        """Own the MCP connection for its whole lifetime, in one task.

        The transport is built on anyio task groups, whose cancel scopes must be
        exited by the same task that entered them. The ACP runtime dispatches
        every request in its own task, so a connection opened during
        `session/prompt` and closed during `session/close` crosses tasks and
        anyio rejects it with "Attempted to exit cancel scope in a different
        task". Holding the whole stack open inside this one task keeps entry and
        exit together; other tasks only ever send requests through the session,
        which is safe.
        """
        assert self._ready is not None and self._close_requested is not None
        try:
            async with AsyncExitStack() as stack:
                http_client = await stack.enter_async_context(
                    httpx2.AsyncClient(headers=self._headers, timeout=self._timeout)
                )
                streams = await stack.enter_async_context(
                    streamable_http_client(self._url, http_client=http_client)
                )
                read, write = streams[0], streams[1]
                session = await stack.enter_async_context(ClientSession(read, write))
                await session.initialize()
                listed = await session.list_tools()

                self._session = session
                self._tools = list(listed.tools)
                self._instructions = session.instructions
                logger.info(
                    "MCP connected: url=%s tools=%d names=%s instructions=%s",
                    self._url,
                    len(self._tools),
                    [t.name for t in self._tools],
                    "yes" if self._instructions else "none",
                )
                if not self._ready.done():
                    self._ready.set_result(None)

                await self._close_requested.wait()
        except asyncio.CancelledError:
            raise
        except BaseException as exc:
            # Before ready: hand the failure to connect()'s caller. After ready:
            # the connection dropped mid-session, so log it and let the next
            # call_tool fail on its own terms rather than retrying here.
            if not self._ready.done():
                self._ready.set_exception(exc)
            else:
                logger.exception("MCP session for %s ended unexpectedly", self._url)
        finally:
            self._session = None

    async def connect(self) -> None:
        if self._task is not None:
            return

        self._ready = asyncio.get_running_loop().create_future()
        self._close_requested = asyncio.Event()
        self._task = asyncio.create_task(self._serve(), name=f"mcp-session:{self._url}")
        try:
            await self._ready
        except BaseException:
            self._task = None
            raise

    async def close(self) -> None:
        if self._task is None:
            return
        task, self._task = self._task, None
        if self._close_requested is not None:
            self._close_requested.set()
        try:
            await task
        except asyncio.CancelledError:
            raise
        except Exception:
            logger.exception("Error closing MCP session for %s", self._url)
        finally:
            self._session, self._tools, self._instructions = None, [], None

    def openai_tools(self) -> list[dict[str, Any]]:
        """MCP tool definitions in OpenAI function-calling shape.

        `input_schema` is the field name on mcp.types.Tool (the wire alias is
        `inputSchema`); the SDK model exposes the snake_case attribute. The
        schema is forwarded as-is — no coercion, no defaults filled in.
        """
        tools: list[dict[str, Any]] = []
        for t in self._tools:
            tools.append(
                {
                    "type": "function",
                    "function": {
                        "name": t.name,
                        "description": t.description or "",
                        "parameters": t.input_schema,
                    },
                }
            )
        return tools

    async def call_tool(self, name: str, args: dict[str, Any]) -> str:
        """Execute one MCP tool call and join its text content parts.

        No retry: a retried tool call is a repair, and repairs are exactly what
        this agent must not perform if its transcripts are to mean anything.
        """
        if self._session is None:
            raise RuntimeError("MCP client is not connected")

        result = await self._session.call_tool(name, args or {}, read_timeout_seconds=self._timeout)
        parts: list[str] = []
        for block in getattr(result, "content", []) or []:
            text = getattr(block, "text", None)
            if text:
                parts.append(text)
        return "\n".join(parts) if parts else "(empty result)"
