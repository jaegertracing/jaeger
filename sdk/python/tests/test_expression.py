# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

"""The expected trees here follow jaeger.expression.v1, which is the contract.

test_conformance.py then feeds these same shapes through the real protobuf
messages, so a change in the IDL shows up as a failure rather than as a filter the
server quietly rejects.
"""

import json

import pytest

from jaeger_query import (
    Call,
    Expr,
    ListValue,
    Scalar,
    SpanKind,
    SpanStatus,
    ValueRef,
    and_,
    attr,
    event,
    link,
    or_,
    resource,
    scope,
    some,
    span,
)


def test_an_unqualified_attribute_is_an_attr_term():
    assert (attr("http.status_code") == 500).to_dict() == {
        "op": "eq",
        "args": [
            {"attr": {"key": "http.status_code"}},
            {"scalar": {"value": "500"}},
        ],
    }


def test_a_level_qualified_attribute_carries_its_level():
    assert span.attr("http.status_code").exists().to_dict() == {
        "op": "exists",
        "args": [{"attr": {"key": "http.status_code", "level": "span"}}],
    }
    assert span("http.status_code").exists().to_dict() == span.attr("http.status_code").exists().to_dict()


def test_a_builtin_field_is_a_field_term_and_always_carries_a_level():
    flt = (span.duration > "2s") & attr("http.status_code").one_of([500, 503])
    assert flt.to_dict() == {
        "op": "and",
        "args": [
            {
                "call": {
                    "op": "gt",
                    "args": [
                        {"field": {"name": "duration", "level": "span"}},
                        {"scalar": {"value": "2s"}},
                    ],
                }
            },
            {
                "call": {
                    "op": "in",
                    "args": [
                        {"attr": {"key": "http.status_code"}},
                        {"list": {"values": ["500", "503"], "type": "int"}},
                    ],
                }
            },
        ],
    }


def test_a_comparison_reads_two_references_as_readily_as_one():
    assert (span.startTime < span.endTime).to_dict() == {
        "op": "lt",
        "args": [
            {"field": {"name": "startTime", "level": "span"}},
            {"field": {"name": "endTime", "level": "span"}},
        ],
    }


def test_attributes_of_two_levels_compare():
    assert (span.attr("enduser.id") != resource.attr("enduser.id")).to_dict() == {
        "op": "ne",
        "args": [
            {"attr": {"key": "enduser.id", "level": "span"}},
            {"attr": {"key": "enduser.id", "level": "resource"}},
        ],
    }


def test_a_correlated_event_query_quantifies_over_a_nested_term():
    flt = event.some((event.name == "exception") & (event.timeSinceStart > "50us"))
    assert flt.to_dict() == {
        "op": "some",
        "args": [
            {"nested": {"level": "event"}},
            {
                "call": {
                    "op": "and",
                    "args": [
                        {
                            "call": {
                                "op": "eq",
                                "args": [
                                    {"field": {"name": "name", "level": "event"}},
                                    {"scalar": {"value": "exception"}},
                                ],
                            }
                        },
                        {
                            "call": {
                                "op": "gt",
                                "args": [
                                    {"field": {"name": "timeSinceStart", "level": "event"}},
                                    {"scalar": {"value": "50us"}},
                                ],
                            }
                        },
                    ],
                }
            },
        ],
    }
    assert some(event, event.name == "exception").to_dict() == event.some(event.name == "exception").to_dict()
    assert some(link.nested(), link.traceID.exists()).to_dict()["args"][0] == {"nested": {"level": "link"}}


@pytest.mark.parametrize(
    ("predicate", "op"),
    [
        (span.duration == "2s", "eq"),
        (span.duration != "2s", "ne"),
        (span.duration > "2s", "gt"),
        (span.duration < "2s", "lt"),
        (span.duration >= "2s", "gte"),
        (span.duration <= "2s", "lte"),
        (span.duration.eq("2s"), "eq"),
        (span.duration.ne("2s"), "ne"),
        (span.duration.gt("2s"), "gt"),
        (span.duration.lt("2s"), "lt"),
        (span.duration.gte("2s"), "gte"),
        (span.duration.lte("2s"), "lte"),
        (span.name.matches("GET|POST"), "regex"),
        (attr("k8s.pod.name").exists(), "exists"),
        (resource.service.one_of(["cart"]), "in"),
        (resource.service.not_one_of(["cart"]), "not_in"),
        (span.kind == "server", "eq"),
        (span.status.one_of(["error"]), "in"),
    ],
)
def test_every_operator_has_a_spelling(predicate, op):
    assert predicate.op == op


def test_boolean_combinators_nest():
    flt = ((span.duration > "2s") | attr("error").eq(True)) & ~resource.service.eq("healthcheck")
    assert flt.to_dict() == {
        "op": "and",
        "args": [
            {
                "call": {
                    "op": "or",
                    "args": [
                        {
                            "call": {
                                "op": "gt",
                                "args": [
                                    {"field": {"name": "duration", "level": "span"}},
                                    {"scalar": {"value": "2s"}},
                                ],
                            }
                        },
                        {
                            "call": {
                                "op": "eq",
                                "args": [
                                    {"attr": {"key": "error"}},
                                    {"scalar": {"value": "true"}},
                                ],
                            }
                        },
                    ],
                }
            },
            {
                "call": {
                    "op": "not",
                    "args": [
                        {
                            "call": {
                                "op": "eq",
                                "args": [
                                    {"field": {"name": "service", "level": "resource"}},
                                    {"scalar": {"value": "healthcheck"}},
                                ],
                            }
                        }
                    ],
                }
            },
        ],
    }


def test_a_single_predicate_needs_no_boolean_wrapper():
    predicate = span.duration > "2s"
    assert and_(predicate) is predicate
    assert or_(predicate) is predicate


def test_repeating_one_combinator_builds_a_flat_n_ary_call():
    a, b, c = attr("a").eq("1"), attr("b").eq("2"), attr("c").eq("3")
    for flat in (a | b | c, or_(a, or_(b, c)), or_(or_(a, b), c)):
        assert flat.op == "or"
        assert [arg["call"]["args"][0]["attr"]["key"] for arg in flat.to_dict()["args"]] == ["a", "b", "c"]


def test_a_mix_of_combinators_still_nests():
    a, b, c = attr("a").eq("1"), attr("b").eq("2"), attr("c").eq("3")
    mixed = (a & b) | c
    assert mixed.op == "or"
    assert [arg.op for arg in mixed.args if isinstance(arg, Call)] == ["and", "eq"]


# -- the vocabulary, which the levels carry rather than validate ---------------


def test_every_level_is_reachable_and_names_itself():
    assert [lvl.level for lvl in (span, resource, scope, event, link)] == [
        "span",
        "resource",
        "scope",
        "event",
        "link",
    ]


@pytest.mark.parametrize(
    ("level", "names"),
    [
        (
            span,
            [
                "traceID",
                "spanID",
                "parentSpanID",
                "traceState",
                "name",
                "kind",
                "startTime",
                "endTime",
                "duration",
                "status",
                "statusMessage",
            ],
        ),
        (resource, ["service", "schemaURL"]),
        (scope, ["name", "version", "schemaURL"]),
        (event, ["name", "time", "timeSinceStart"]),
        (link, ["traceID", "spanID", "traceState"]),
    ],
)
def test_each_level_carries_exactly_the_fields_the_data_model_defines(level, names):
    for name in names:
        ref = getattr(level, name)
        assert ref.to_expression() == {"field": {"name": name, "level": level.level}}


def test_a_field_the_data_model_does_not_define_cannot_be_written():
    # A type checker reports both of these too, which is the first line of defence;
    # the ignores below are what let the runtime behaviour be asserted here.
    with pytest.raises(AttributeError):
        span.durationn  # type: ignore[attr-defined]  # noqa: B018
    with pytest.raises(AttributeError):
        resource.name  # type: ignore[attr-defined]  # noqa: B018 - a span field, not a resource one


def test_a_field_with_no_ordering_has_no_ordering_operators():
    for unordered in (span.kind, span.status):
        with pytest.raises(TypeError):
            unordered > "x"  # type: ignore[operator]  # noqa: B015
        with pytest.raises(AttributeError):
            unordered.gt("x")  # type: ignore[attr-defined]


def test_the_closed_value_sets_are_published():
    assert SpanStatus == ("unset", "ok", "error")
    assert "server" in SpanKind and "consumer" in SpanKind
    assert span.kind.one_of(SpanKind).to_dict()["args"][1]["list"]["values"] == list(SpanKind)


def test_only_a_collection_level_can_be_quantified_over():
    assert hasattr(event, "some") and hasattr(link, "some")
    for single in (span, resource, scope):
        assert not hasattr(single, "some")
        assert not hasattr(single, "nested")


# -- constants -----------------------------------------------------------------


def test_a_scalar_declares_no_type_unless_a_caller_sets_one():
    # A type that is set is authoritative, so guessing one narrows the match.
    assert (attr("http.response.size") > 500).args[1].to_expression() == {"scalar": {"value": "500"}}
    assert (attr("sampling.probability") > 0.5).args[1].to_expression() == {"scalar": {"value": "0.5"}}
    assert span.duration.gt(Scalar("2000", "int")).args[1].to_expression() == {
        "scalar": {"value": "2000", "type": "int"}
    }


def test_a_list_declares_its_element_type_only_against_an_attribute():
    # The field supplies the type; an attribute declares nothing, so the list must.
    assert resource.service.one_of(["cart"]).args[1].to_expression() == {"list": {"values": ["cart"]}}
    assert attr("code").one_of([500]).args[1].to_expression() == {"list": {"values": ["500"], "type": "int"}}
    assert attr("ratio").one_of([0.5]).args[1].to_expression() == {
        "list": {"values": ["0.5"], "type": "double"}
    }
    assert attr("on").one_of([True]).args[1].to_expression() == {"list": {"values": ["true"], "type": "bool"}}
    assert attr("name").one_of(["a"]).args[1].to_expression() == {"list": {"values": ["a"], "type": "string"}}


def test_a_mixed_list_is_read_as_text():
    assert attr("code").one_of([500, "unknown"]).args[1].to_expression() == {
        "list": {"values": ["500", "unknown"], "type": "string"}
    }


def test_a_list_can_state_its_own_type():
    assert attr("code").one_of(ListValue(["500"], "int")).args[1].to_expression() == {
        "list": {"values": ["500"], "type": "int"}
    }


def test_an_empty_list_is_refused():
    with pytest.raises(ValueError, match="membership in nothing"):
        attr("code").one_of([])


def test_an_unsupported_python_value_is_refused():
    with pytest.raises(TypeError, match="cannot use dict"):
        attr("code").eq({})


# -- rendering -----------------------------------------------------------------


def test_the_filter_is_a_bare_call_while_its_arguments_are_wrapped():
    flt = and_(span.duration > "2s", attr("error").eq(True))
    assert set(flt.to_dict()) == {"op", "args"}
    assert set(flt.to_expression()) == {"call"}
    assert json.loads(flt.to_json()) == flt.to_dict()


def test_the_terms_are_readable_when_printed():
    assert repr(span) == "SpanLevel('span')"
    assert repr(attr("k")) == "AttributeRef(key='k', level='')"
    assert repr(span.duration) == "FieldRef(name='duration', level='span')"
    assert repr(span.kind) == "UnorderedFieldRef(name='kind', level='span')"
    assert repr(event.nested()) == "NestedRef(level='event')"
    assert "http.status_code" in repr(attr("http.status_code") == 500)


def test_the_base_nodes_leave_their_hooks_to_subclasses():
    with pytest.raises(NotImplementedError):
        Expr().to_expression()
    with pytest.raises(NotImplementedError):
        ValueRef()._list_needs_type()


def test_a_combinator_needs_a_predicate():
    with pytest.raises(ValueError, match="at least one predicate"):
        and_()
