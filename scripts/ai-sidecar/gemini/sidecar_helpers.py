# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

import json
from typing import Any


def _validate_function_call(tool_name: str, args: Any, tool_call_id: str) -> None:
    """Validate that a Gemini-emitted function_call is dispatchable.

    Raises ValueError when ``tool_name`` is empty or ``args`` is not a dict.
    ``_execute_tool`` calls this on entry so a malformed function_call from
    Gemini cannot dispatch an empty name into MCP, or feed non-dict args into a
    tool — either would mask real integration bugs as silent successes.
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
# attribute — the untruncated value still reaches the LLM context, and the
# gateway emits the untruncated value on the AG-UI wire.
MAX_SPAN_ATTR_CHARS = 65536


def _truncate_for_span(text: str, max_chars: int = MAX_SPAN_ATTR_CHARS) -> str:
    if len(text) <= max_chars:
        return text
    suffix = f"... [truncated, {len(text)} chars total]"
    if max_chars <= len(suffix):
        return suffix[:max_chars]
    keep = max_chars - len(suffix)
    return f"{text[:keep]}{suffix}"


def _announced_mcp_endpoint(mcp_servers: Any) -> tuple[str, dict[str, str]] | None:
    """Pull the turn-scoped MCP endpoint out of ``session/new``'s ``mcpServers``.

    Returns the URL and the headers to send with every request to it, or None
    when the gateway announced no HTTP endpoint — which is what a deployment
    without the MCP config block looks like from here.

    The headers are not decoration: the endpoint sits behind jaeger-query's
    tenancy extraction, so under multi-tenancy the announcement carries the
    turn's tenant header and dropping it would make every tool call 401.

    Entries arrive either as parsed ACP models or as raw dicts depending on how
    the router decoded them, so both shapes are read the same way. Only the HTTP
    variant is understood: an SSE announcement carries the same url/headers
    shape but speaks a different protocol, so it is skipped rather than dialed
    with the streamable-HTTP client.
    """
    if not isinstance(mcp_servers, list):
        return None
    for server in mcp_servers:
        if _field(server, "type") != "http":
            continue
        url = _field(server, "url")
        if not isinstance(url, str) or not url:
            continue
        headers: dict[str, str] = {}
        for header in _field(server, "headers") or []:
            name, value = _field(header, "name"), _field(header, "value")
            if isinstance(name, str) and name and isinstance(value, str):
                headers[name] = value
        return url, headers
    return None


def _field(obj: Any, name: str) -> Any:
    """Read one field from an ACP model or the equivalent raw dict."""
    if isinstance(obj, dict):
        return obj.get(name)
    return getattr(obj, name, None)
