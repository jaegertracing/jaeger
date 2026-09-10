# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

import json
from typing import Any, cast

from google.genai import types


# CONTEXTUAL_TOOLS_META_KEY is the namespaced key the Jaeger AI gateway uses
# under NewSessionRequest._meta to attach the frontend-provided AG-UI tool
# snapshot for the turn. The value at this key is shaped as
# {"tools": [{"name": ..., "description": ..., "parameters": ...}, ...]}.
CONTEXTUAL_TOOLS_META_KEY = "jaegertracing.io/contextual-tools"


def _validate_function_call(tool_name: str, args: Any, tool_call_id: str) -> None:
    """Validate that a Gemini-emitted function_call is dispatchable.

    Raises ValueError when ``tool_name`` is empty or ``args`` is not a dict.
    Both execute paths call this on entry so a malformed function_call from
    Gemini cannot dispatch an empty name into MCP / the ext_method, or feed
    non-dict args into a tool — either would mask real integration bugs as
    silent successes.
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


def _extract_contextual_tools(meta: Any) -> list[dict[str, Any]]:
    """Pull AG-UI tools out of NewSessionRequest._meta. Returns an empty
    list if the meta is absent, the namespaced key is missing, or the
    payload is malformed — the gateway populates this only when the
    frontend actually attached tools to the chat request.

    The Python ACP router spreads ``_meta`` into the handler's ``**kwargs``
    by inner key, so callers pass the handler ``kwargs`` dict directly
    rather than a separate ``field_meta`` argument."""
    if not isinstance(meta, dict):
        return []
    payload = meta.get(CONTEXTUAL_TOOLS_META_KEY)
    if not isinstance(payload, dict):
        return []
    tools = payload.get("tools")
    if not isinstance(tools, list):
        return []
    return [t for t in tools if isinstance(t, dict) and isinstance(t.get("name"), str)]


def _build_gemini_contextual_tool(contextual_tools: list[dict[str, Any]]) -> types.Tool | None:
    """Translate AG-UI tool entries into a single Gemini Tool wrapping a
    list of FunctionDeclarations. Returns None when no tools are supplied
    so the caller doesn't have to guard against an empty Tool."""
    if not contextual_tools:
        return None
    declarations: list[types.FunctionDeclaration] = []
    for tool in contextual_tools:
        name = tool.get("name")
        if not isinstance(name, str) or not name:
            continue
        params = tool.get("parameters")
        if not isinstance(params, dict):
            params = {"type": "object"}
        description = tool.get("description")
        if not isinstance(description, str):
            description = ""
        declarations.append(
            types.FunctionDeclaration(
                name=name,
                description=description,
                parameters_json_schema=params,
            )
        )
    if not declarations:
        return None
    return types.Tool(function_declarations=declarations)


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


# MAX_SPAN_ATTR_CHARS caps gen_ai.tool.call.arguments/result span attribute
# values. Tool payloads (e.g. search_traces results, or a full read_skill
# load) can be arbitrarily large; an uncapped value risks tripping OTLP
# exporter/backend attribute-size limits and failing export for the whole
# span batch. Set comfortably above the ~5000-token (~20-25k char) skill
# budget so a legitimate full skill load is never truncated — this is meant
# to catch pathological payloads (e.g. a huge search_traces dump), not normal
# skill/tool content. Truncation only affects what lands in the trace
# attribute — the untruncated value still reaches the AG-UI wire / LLM
# context via the normal session_update / ext_method paths.
MAX_SPAN_ATTR_CHARS = 65536


def _truncate_for_span(text: str, max_chars: int = MAX_SPAN_ATTR_CHARS) -> str:
    if len(text) <= max_chars:
        return text
    suffix = f"... [truncated, {len(text)} chars total]"
    if max_chars <= len(suffix):
        return suffix[:max_chars]
    keep = max_chars - len(suffix)
    return f"{text[:keep]}{suffix}"


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
