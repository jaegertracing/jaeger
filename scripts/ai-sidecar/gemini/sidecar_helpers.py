# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

import json
from typing import Any, cast

from google.genai import types


def _to_jsonable(value: Any) -> Any:
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


def _extract_function_declaration(tool: Any) -> types.FunctionDeclaration | None:
    """Return a Gemini function declaration from an ADK tool.

    Prefer a public API when available; fall back to ADK's internal method for
    compatibility with current tool implementations.
    """
    get_declaration = getattr(tool, "get_declaration", None)
    if callable(get_declaration):
        declaration = get_declaration()
        if declaration is not None:
            return cast(types.FunctionDeclaration, declaration)

    # ADK BaseTool currently exposes declaration via _get_declaration().
    # Keep this fallback isolated in one place to reduce breakage risk.
    private_get_declaration = getattr(tool, "_get_declaration", None)
    if callable(private_get_declaration):
        return cast(types.FunctionDeclaration, private_get_declaration())

    return None
