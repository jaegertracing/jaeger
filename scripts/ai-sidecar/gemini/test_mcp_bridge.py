# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

"""Unit tests for the MCP bridge, covering the contextual-tools session-id
forwarding contract that keeps per-turn snapshots correctly routed even when
multiple chat requests are in flight concurrently.
"""

from __future__ import annotations

import asyncio
from typing import Any

from google.genai import types

import mcp_bridge
from mcp_bridge import (
    CONTEXTUAL_TOOLS_SESSION_ARG,
    CONTEXTUAL_TOOLS_TOOL_NAME,
    JaegerMCPBridge,
    _strip_session_id_from_declaration,
)


class _FakeTool:
    """Minimal stand-in for an ADK tool capturing the args it is called with."""

    def __init__(self, name: str) -> None:
        self.name = name
        self.last_args: dict[str, Any] | None = None

    async def run_async(self, args: dict[str, Any], tool_context: Any) -> dict[str, Any]:
        self.last_args = args
        return {"called_with": args}


def _make_bridge(tools: dict[str, _FakeTool]) -> JaegerMCPBridge:
    """Build an already-initialized bridge so call_tool skips real discovery."""
    bridge = JaegerMCPBridge("http://unused", timeout_sec=0.1)
    bridge._tools_by_name = tools  # type: ignore[attr-defined]
    bridge._initialized = True  # type: ignore[attr-defined]
    return bridge


def test_strip_session_id_removes_property_and_required() -> None:
    declaration = types.FunctionDeclaration(
        name=CONTEXTUAL_TOOLS_TOOL_NAME,
        parameters=types.Schema(
            type="OBJECT",
            properties={
                CONTEXTUAL_TOOLS_SESSION_ARG: types.Schema(type="STRING"),
                "other": types.Schema(type="STRING"),
            },
            required=[CONTEXTUAL_TOOLS_SESSION_ARG, "other"],
        ),
    )

    _strip_session_id_from_declaration(declaration)

    assert CONTEXTUAL_TOOLS_SESSION_ARG not in (declaration.parameters.properties or {})
    assert "other" in (declaration.parameters.properties or {})
    assert declaration.parameters.required == ["other"], (
        "session_id must be removed from required so Gemini does not try to populate it"
    )


def test_strip_session_id_tolerates_missing_parameters() -> None:
    declaration = types.FunctionDeclaration(name=CONTEXTUAL_TOOLS_TOOL_NAME)
    # Should not raise even when parameters/properties/required are absent.
    _strip_session_id_from_declaration(declaration)


def test_call_tool_injects_acp_session_id_for_contextual_tools() -> None:
    fake = _FakeTool(CONTEXTUAL_TOOLS_TOOL_NAME)
    bridge = _make_bridge({CONTEXTUAL_TOOLS_TOOL_NAME: fake})

    asyncio.run(bridge.call_tool(CONTEXTUAL_TOOLS_TOOL_NAME, {}, acp_session_id="sess-42"))

    assert fake.last_args == {CONTEXTUAL_TOOLS_SESSION_ARG: "sess-42"}, (
        "bridge must inject the authoritative ACP session id into the MCP call"
    )


def test_call_tool_overrides_model_supplied_session_id() -> None:
    fake = _FakeTool(CONTEXTUAL_TOOLS_TOOL_NAME)
    bridge = _make_bridge({CONTEXTUAL_TOOLS_TOOL_NAME: fake})

    asyncio.run(
        bridge.call_tool(
            CONTEXTUAL_TOOLS_TOOL_NAME,
            {CONTEXTUAL_TOOLS_SESSION_ARG: "hallucinated"},
            acp_session_id="sess-real",
        )
    )

    assert fake.last_args == {CONTEXTUAL_TOOLS_SESSION_ARG: "sess-real"}, (
        "a model-chosen session_id must be overridden so snapshots cannot be misrouted"
    )


def test_call_tool_leaves_other_tools_untouched() -> None:
    fake = _FakeTool("search_traces")
    bridge = _make_bridge({"search_traces": fake})

    asyncio.run(
        bridge.call_tool("search_traces", {"service_name": "checkout"}, acp_session_id="sess-1")
    )

    assert fake.last_args == {"service_name": "checkout"}, (
        "non-contextual tools must not receive the ACP session id"
    )


def test_call_tool_without_acp_session_id_is_noop_injection() -> None:
    fake = _FakeTool(CONTEXTUAL_TOOLS_TOOL_NAME)
    bridge = _make_bridge({CONTEXTUAL_TOOLS_TOOL_NAME: fake})

    asyncio.run(bridge.call_tool(CONTEXTUAL_TOOLS_TOOL_NAME, {"other": "v"}))

    # No injection when the caller did not supply an ACP session id — the
    # backend will return an empty tools list, which is the intended safe mode.
    assert fake.last_args == {"other": "v"}


def test_call_tool_unknown_tool_returns_error() -> None:
    bridge = _make_bridge({})

    result = asyncio.run(bridge.call_tool("nope", {}))

    assert result == {"error": "unsupported tool: nope"}


def test_constants_match_backend_contract() -> None:
    # These constants are the public contract with the Go backend's
    # jaegermcp.ListContextualToolsInput and the MCP tool registration.
    assert mcp_bridge.CONTEXTUAL_TOOLS_TOOL_NAME == "list_contextual_tools"
    assert mcp_bridge.CONTEXTUAL_TOOLS_SESSION_ARG == "session_id"
