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

# URI of the skills index resource — the single entry point for skill discovery.
_SKILLS_INDEX_URI = "skill://skills-index"


def _extract_json(text: str) -> dict | None:
    """Extract the last JSON object from an MCP Streamable HTTP response.

    The response may be plain JSON or SSE (data: {...} lines). We scan
    backwards for the last line that looks like a JSON object.
    """
    for line in reversed(text.strip().splitlines()):
        line = line.strip()
        if line.startswith("data:"):
            line = line[5:].strip()
        if line.startswith("{"):
            try:
                return json.loads(line)
            except json.JSONDecodeError:
                continue
    try:
        return json.loads(text.strip())
    except json.JSONDecodeError:
        return None


class JaegerMCPBridge:
    """Loads MCP tools and skill resources once and exposes them for Gemini."""

    def __init__(self, mcp_url: str, timeout_sec: float):
        self._mcp_url = mcp_url
        self._timeout_sec = timeout_sec
        self._toolset = MCPToolset(
            connection_params=StreamableHTTPConnectionParams(url=mcp_url),
        )
        self._tools_by_name: dict[str, Any] = {}
        self._gemini_tools: list[types.Tool] = []
        self._skills_index: str | None = None  # cached skills index SKILL.md body
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

        # Discover skills index via resources/read — best-effort, never fails startup.
        self._skills_index = await self._fetch_skills_index()

    async def _fetch_skills_index(self) -> str | None:
        """Fetch the skills index via MCP resources/read using a throw-away session.

        Returns the SKILL.md body for skill://skills-index, or None if
        the server does not support resources or the call fails for any reason.
        """
        try:
            # Open a minimal MCP session (initialize → resources/read → delete).
            init_body = {
                "jsonrpc": "2.0",
                "id": 1,
                "method": "initialize",
                "params": {
                    "protocolVersion": "2025-03-26",
                    "capabilities": {},
                    "clientInfo": {"name": "jaeger-sidecar", "version": "1.0.0"},
                },
            }
            headers = {
                "Content-Type": "application/json",
                "Accept": "application/json, text/event-stream",
            }
            init_resp = await asyncio.to_thread(
                lambda: requests.post(self._mcp_url, json=init_body, headers=headers, timeout=10)
            )
            init_resp.raise_for_status()
            sid = init_resp.headers.get("Mcp-Session-Id")
            if not sid:
                logger.debug("MCP server did not return a session ID; skipping skills discovery")
                return None

            read_body = {
                "jsonrpc": "2.0",
                "id": 2,
                "method": "resources/read",
                "params": {"uri": _SKILLS_INDEX_URI},
            }
            read_headers = {**headers, "Mcp-Session-Id": sid}
            read_resp = await asyncio.to_thread(
                lambda: requests.post(self._mcp_url, json=read_body, headers=read_headers, timeout=10)
            )
            read_resp.raise_for_status()

            # Extract the JSON result from SSE or plain JSON response.
            result_json = _extract_json(read_resp.text)
            if result_json is None:
                logger.debug("Could not parse resources/read response for %s", _SKILLS_INDEX_URI)
                return None

            contents = result_json.get("result", {}).get("contents", [])
            if not contents:
                return None

            body = contents[0].get("text", "")
            logger.info("Loaded skills index from %s (%d bytes)", _SKILLS_INDEX_URI, len(body))
            return body or None

        except Exception as exc:
            logger.debug("Skills index discovery failed (non-fatal): %s", exc)
            return None
        finally:
            # Best-effort session cleanup.
            try:
                if "sid" in dir() and sid:
                    await asyncio.to_thread(
                        lambda: requests.delete(self._mcp_url, headers={"Mcp-Session-Id": sid}, timeout=5)
                    )
            except Exception:
                pass

    async def get_skills_index(self) -> str | None:
        """Return the cached skills index body, loading it lazily if needed."""
        await self.initialize()
        return self._skills_index

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
