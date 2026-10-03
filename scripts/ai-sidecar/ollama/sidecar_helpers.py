# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

import json
from typing import Any

# CONTEXTUAL_TOOLS_META_KEY is the namespaced key the Jaeger AI gateway uses
# under NewSessionRequest._meta to attach the frontend-provided AG-UI tool
# snapshot for the turn. The value at this key is shaped as
# {"tools": [{"name": ..., "description": ..., "parameters": ...}, ...]}.
CONTEXTUAL_TOOLS_META_KEY = "jaegertracing.io/contextual-tools"


def _validate_function_call(tool_name: str, args: Any, tool_call_id: str) -> None:
    """Validate that a model-emitted tool call is dispatchable.

    Raises ValueError when ``tool_name`` is empty or ``args`` is not a mapping.
    Both execute paths call this on entry so a malformed tool call from the
    model cannot dispatch an empty name into MCP / the ext_method, or feed
    non-dict args into a tool — either would mask real integration bugs as
    silent successes. Local models are more prone to this than a hosted API,
    which is exactly why the check stays.
    """
    if not tool_name:
        raise ValueError(
            f"tool call has no name (call_id={tool_call_id})"
        )
    if not isinstance(args, dict):
        raise ValueError(
            f"tool call '{tool_name}' has non-dict args "
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


def _build_ollama_tool(name: str, description: str, parameters: Any) -> dict[str, Any]:
    """Wrap one tool declaration in Ollama's OpenAI-style envelope.

    ``parameters`` is passed through as-is when it's a JSON Schema object,
    which it always is on both inbound paths: MCP advertises ``inputSchema``
    as JSON Schema, and the gateway's contextual snapshot carries the
    frontend's JSON Schema verbatim. That's the reason this sidecar needs no
    schema translation layer at all — unlike the Gemini one, whose SDK has its
    own declaration type.
    """
    if not isinstance(parameters, dict):
        parameters = {"type": "object", "properties": {}}
    return {
        "type": "function",
        "function": {
            "name": name,
            "description": description,
            "parameters": parameters,
        },
    }


def _build_contextual_ollama_tools(contextual_tools: list[dict[str, Any]]) -> list[dict[str, Any]]:
    """Translate the gateway's AG-UI tool snapshot into Ollama tools."""
    declarations: list[dict[str, Any]] = []
    for tool in contextual_tools:
        name = tool.get("name")
        if not isinstance(name, str) or not name:
            continue
        description = tool.get("description")
        declarations.append(
            _build_ollama_tool(
                name=name,
                description=description if isinstance(description, str) else "",
                parameters=tool.get("parameters"),
            )
        )
    return declarations


def _to_jsonable(value: Any) -> Any:
    if hasattr(value, "model_dump"):
        return value.model_dump(mode="json")
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
