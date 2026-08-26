# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

"""Check the builder's output against the generated protobuf messages.

The rest of the suite asserts the JSON the builder produces, which pins it to what
this SDK believes the contract is. That is worth little on its own: when the filter
AST changed shape in jaeger-idl, every one of those assertions still passed while
the server rejected every filter.

So these tests parse each shape with the real ``jaeger.expression.v1`` messages in
strict mode, where an unknown or misspelled field is an error. If the AST drops a
term, renames a field, or narrows a level, that shows up here as a failure, rather
than as a filter the server refuses at runtime.
"""

import pytest
from google.protobuf import json_format

from expression.v1 import expression_pb2 as ex
from jaeger_query import (
    Query,
    Scalar,
    attr,
    event,
    link,
    resource,
    scope,
    span,
)

# One filter per shape the builder can produce. The names describe the shape, since
# a failure here means that shape no longer exists in the IDL.
FILTERS = {
    "unqualified attribute": attr("http.status_code") == 500,
    "attribute at a level": span.attr("http.status_code").exists(),
    "attribute at every level": (
        span.attr("a").exists()
        & resource.attr("b").exists()
        & scope.attr("c").exists()
        & event.attr("d").exists()
        & link.attr("e").exists()
    ),
    "built-in field": span.duration > "2s",
    "field at every level": (
        span.name.exists()
        & resource.service.exists()
        & scope.version.exists()
        & event.timeSinceStart.exists()
        & link.traceID.exists()
    ),
    "unordered field": span.kind == "server",
    "field against field": span.startTime < span.endTime,
    "attribute against attribute": span.attr("enduser.id") != resource.attr("enduser.id"),
    "typed scalar": span.duration.gt(Scalar("2000", "int")),
    "list against an attribute": attr("http.status_code").one_of([500, 503]),
    "list against a field": resource.service.one_of(["cart", "checkout"]),
    "negated membership": attr("code").not_one_of([500]),
    "regex": span.name.matches("GET|POST"),
    "conjunction": (span.duration > "2s") & attr("error").eq(True),
    "disjunction": (span.duration > "2s") | attr("error").eq(True),
    "negation": ~resource.service.eq("healthcheck"),
    "nested quantifier over events": event.some(
        (event.name == "exception") & (event.timeSinceStart > "50us")
    ),
    "nested quantifier over links": link.some(link.traceID.exists()),
    "every operator at once": (
        (span.duration >= "1s")
        & (span.duration <= "9s")
        & (span.name != "health")
        & span.attr("k").exists()
        & attr("code").one_of([500])
        & span.name.matches("a.*")
        & ~(span.status == "error")
    ),
}


@pytest.mark.parametrize("shape", FILTERS.keys())
def test_the_builder_emits_a_call_the_idl_recognises(shape):
    """Parse strictly, so a field the AST does not define fails rather than being dropped."""
    parsed = json_format.ParseDict(FILTERS[shape].to_dict(), ex.Call(), ignore_unknown_fields=False)
    # Round-tripping proves the parse kept every term, rather than accepting the
    # message and quietly leaving fields unset.
    assert json_format.MessageToDict(parsed) == FILTERS[shape].to_dict()


@pytest.mark.parametrize("shape", FILTERS.keys())
def test_every_argument_sets_one_of_the_oneof_terms(shape):
    """A term the oneof does not have parses into an unset oneof, which nothing catches."""
    parsed = json_format.ParseDict(FILTERS[shape].to_dict(), ex.Call())
    _assert_terms_set(parsed)


def _assert_terms_set(call: ex.Call) -> None:
    assert call.op, "a call always names an operator"
    assert call.args, "a call always has arguments"
    for arg in call.args:
        term = arg.WhichOneof("term")
        assert term is not None, f"argument of {call.op!r} sets no term of the Expression oneof"
        if term == "call":
            _assert_terms_set(arg.call)


def test_the_query_carries_the_filter_as_the_shared_expression_call():
    """The filter field is jaeger.expression.v1.Call, so a query parses whole."""
    from api_v3 import query_service_pb2 as pb

    query = Query().time_range("2026-08-19T12:00:00Z", "2026-08-19T13:00:00Z").where(span.duration > "2s")
    parsed = json_format.ParseDict(query.to_dict(), pb.TraceQueryParameters(), ignore_unknown_fields=False)
    assert parsed.filter.op == "gt"
    assert parsed.filter.args[0].field.name == "duration"
    assert parsed.filter.args[0].field.level == "span"


def test_the_levels_the_builder_uses_are_the_levels_the_schema_publishes():
    """The level vocabulary lives in the OpenAPI annotations, not in the proto type.

    A plain string field cannot carry an enum, so the published set is in the
    schema. What this can check is that every level the builder emits survives a
    strict parse, and that the builder has one object per level.
    """
    levels = {span.level, resource.level, scope.level, event.level, link.level}
    assert levels == {"span", "resource", "scope", "event", "link"}
    for level in levels:
        shape = {"op": "exists", "args": [{"attr": {"key": "k", "level": level}}]}
        assert json_format.MessageToDict(json_format.ParseDict(shape, ex.Call())) == shape


def test_a_term_outside_the_oneof_is_rejected():
    """Proof that the parse above is strict enough to be worth running.

    This is the shape the builder emitted against an earlier draft of the AST, where
    a single `ref` term covered attributes, fields and collections alike. Every
    assertion in test_expression.py passed while the server rejected it; a strict
    parse does not.
    """
    superseded = {
        "op": "eq",
        "args": [
            {"ref": {"name": "http.status_code", "level": "span", "attr": True}},
            {"scalar": {"value": "500"}},
        ],
    }
    with pytest.raises(json_format.ParseError, match="has no field named"):
        json_format.ParseDict(superseded, ex.Call(), ignore_unknown_fields=False)
