# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

"""Small utilities shared between the sidecar agent and the MCP client."""

from __future__ import annotations

import json
from typing import Any


def _validate_function_call(tool_name: str, args: Any, tool_call_id: str) -> None:
    """Validate that a Gemini-emitted function_call is dispatchable.

    Raises ValueError when ``tool_name`` is empty or ``args`` is not a
    dict. The agentic loop calls this on entry so a malformed
    function_call from Gemini cannot dispatch an empty name into MCP
    (or feed non-dict args into a tool) — either would mask real
    integration bugs as silent successes.
    """
    if not tool_name:
        raise ValueError(
            f"function_call has no name (call_id={tool_call_id})"
        )
    if not isinstance(args, dict):
        raise ValueError(
            f"function_call '{tool_name}' has non-dict args "
            f"(type={type(args).__name__}, call_id={tool_call_id})"
        )


def _to_jsonable(value: Any) -> Any:
    """Reduce a pydantic-ish object to its plain dict form. Used both
    to flatten MCP CallToolResult payloads for the Gemini agentic loop
    and to coerce arbitrary tool outputs into JSON-encodable shapes."""
    if hasattr(value, "model_dump"):
        return value.model_dump()
    if hasattr(value, "dict"):
        return value.dict()
    return value


def _to_tool_text(value: Any) -> str:
    if isinstance(value, str):
        return value
    try:
        return json.dumps(_to_jsonable(value), ensure_ascii=False)
    except Exception:
        return str(value)
