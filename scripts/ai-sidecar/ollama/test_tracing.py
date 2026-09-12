# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

from __future__ import annotations

from opentelemetry import trace

from tracing import extract_trace_context

VALID_TRACE_ID = "4bf92f3577b34da6a3ce929d0e0e4736"
VALID_SPAN_ID = "00f067aa0ba902b7"


def _traceparent(trace_id: str = VALID_TRACE_ID, span_id: str = VALID_SPAN_ID) -> str:
    return f"00-{trace_id}-{span_id}-01"


def test_extract_trace_context_with_valid_traceparent_parents_under_it() -> None:
    meta = {"traceparent": _traceparent()}

    ctx = extract_trace_context(meta)
    span_context = trace.get_current_span(ctx).get_span_context()

    assert span_context.is_valid
    assert format(span_context.trace_id, "032x") == VALID_TRACE_ID
    assert format(span_context.span_id, "016x") == VALID_SPAN_ID


def test_extract_trace_context_with_no_meta_returns_current_context() -> None:
    # kwargs the Python ACP router passes when a Prompt request carries no
    # _meta at all — must not raise, and must not fabricate a valid span.
    ctx = extract_trace_context(None)
    assert not trace.get_current_span(ctx).get_span_context().is_valid


def test_extract_trace_context_with_non_dict_meta_returns_current_context() -> None:
    ctx = extract_trace_context("not-a-dict")
    assert not trace.get_current_span(ctx).get_span_context().is_valid


def test_extract_trace_context_with_empty_meta_returns_current_context() -> None:
    ctx = extract_trace_context({})
    assert not trace.get_current_span(ctx).get_span_context().is_valid


def test_extract_trace_context_ignores_unrelated_meta_keys() -> None:
    # Same dict shape _extract_contextual_tools reads from — must not choke
    # on sibling keys it doesn't recognize.
    meta = {
        "jaegertracing.io/contextual-tools": {"tools": []},
        "traceparent": _traceparent(),
    }

    ctx = extract_trace_context(meta)
    span_context = trace.get_current_span(ctx).get_span_context()

    assert span_context.is_valid
    assert format(span_context.trace_id, "032x") == VALID_TRACE_ID
