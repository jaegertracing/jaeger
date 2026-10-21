# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

import datetime as dt
from urllib.parse import parse_qs

import pytest

from jaeger_query import Query, attr, resource, span

START = dt.datetime(2026, 8, 11, 12, 0, tzinfo=dt.UTC)
END = dt.datetime(2026, 8, 11, 13, 0, tzinfo=dt.UTC)


def a_query() -> Query:
    return Query().time_range(START, END)


def test_a_query_renders_its_envelope_and_filter():
    query = (
        a_query()
        .where(span.duration > "2s")
        .where(resource.service.one_of(["cart", "checkout"]))
        .limit(20)
        .raw()
    )
    assert query.to_dict() == {
        "startTimeMin": "2026-08-11T12:00:00Z",
        "startTimeMax": "2026-08-11T13:00:00Z",
        "searchDepth": 20,
        "rawTraces": True,
        "filter": {
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
                            {"field": {"name": "service", "level": "resource"}},
                            {"list": {"values": ["cart", "checkout"]}},
                        ],
                    }
                },
            ],
        },
    }


def test_repeated_where_calls_are_conjoined_once():
    query = a_query().where(attr("a").eq("1")).where(attr("b").eq("2")).where(attr("c").eq("3"))
    flt = query.filter
    assert flt is not None
    assert flt.op == "and"
    assert len(flt.args) == 3


def test_a_single_predicate_is_the_filter_itself():
    flt = a_query().where(attr("a").eq("1")).filter
    assert flt is not None
    assert flt.op == "eq"


def test_a_query_is_reusable_because_each_step_returns_a_new_one():
    recent = a_query().limit(10)
    slow = recent.where(span.duration > "2s")
    failed = recent.where(span("http.status_code") >= 500)
    assert recent.filter is None
    assert slow.filter is not None and slow.filter.op == "gt"
    assert failed.filter is not None and failed.filter.op == "gte"


def test_the_legacy_predicate_fields_still_render():
    query = (
        a_query()
        .service("cart")
        .operation("GET /basket")
        .attributes({"http.status_code": "500"})
        .attributes({"error": "true"})
        .duration(minimum=dt.timedelta(seconds=2), maximum="10s")
    )
    assert query.to_dict() == {
        "startTimeMin": "2026-08-11T12:00:00Z",
        "startTimeMax": "2026-08-11T13:00:00Z",
        "serviceName": "cart",
        "operationName": "GET /basket",
        "attributes": {"http.status_code": "500", "error": "true"},
        "durationMin": "2.0s",
        "durationMax": "10s",
    }


@pytest.mark.parametrize(
    "legacy",
    [
        lambda q: q.service("cart"),
        lambda q: q.operation("GET /basket"),
        lambda q: q.attributes({"error": "true"}),
        lambda q: q.duration(minimum="2s"),
        lambda q: q.duration(maximum="2s"),
    ],
)
def test_a_filter_cannot_be_mixed_with_a_legacy_predicate_field(legacy):
    with pytest.raises(ValueError, match="mutually exclusive"):
        legacy(a_query().where(attr("error").eq(True)))
    with pytest.raises(ValueError, match="mutually exclusive"):
        legacy(a_query()).where(attr("error").eq(True))


def test_a_zero_duration_bound_still_counts_as_a_legacy_field():
    # to_dict() renders durationMin: "0s", so the exclusivity check has to see it;
    # reading the bound for truthiness instead would let it travel beside a filter.
    for bound in ({"minimum": 0}, {"maximum": 0}, {"minimum": dt.timedelta(0)}):
        with pytest.raises(ValueError, match="mutually exclusive"):
            a_query().where(attr("error").eq(True)).duration(**bound)
    assert a_query().duration(minimum=0).to_dict()["durationMin"] == "0s"


def test_an_empty_attributes_map_sets_nothing():
    # Unlike a zero duration, an empty map renders nothing, so there is nothing to
    # reject and no disagreement with to_dict().
    query = a_query().where(attr("error").eq(True)).attributes({})
    assert "attributes" not in query.to_dict()


def test_the_envelope_fields_are_not_predicates_and_mix_freely():
    query = a_query().where(attr("error").eq(True)).limit(5).raw()
    assert query.to_dict()["searchDepth"] == 5


def test_a_time_range_is_required():
    with pytest.raises(ValueError, match="start_time_min is required"):
        Query().to_dict()
    with pytest.raises(ValueError, match="start_time_max is required"):
        Query(start_time_min=START).to_dict()
    with pytest.raises(ValueError, match="must be before"):
        Query().time_range(END, START).to_dict()


def test_last_ends_the_window_at_the_current_time():
    query = Query().last(minutes=15)
    rendered = query.to_dict()
    span_seconds = (
        dt.datetime.fromisoformat(rendered["startTimeMax"])
        - dt.datetime.fromisoformat(rendered["startTimeMin"])
    ).total_seconds()
    assert span_seconds == 15 * 60
    assert dt.datetime.fromisoformat(rendered["startTimeMax"]) <= dt.datetime.now(dt.UTC)


@pytest.mark.parametrize(
    "moment",
    [START, "2026-08-11T12:00:00+00:00", START.timestamp(), dt.datetime(2026, 8, 11, 12, 0)],
)
def test_a_time_is_accepted_in_several_forms_and_read_as_utc(moment):
    assert Query().time_range(moment, END).to_dict()["startTimeMin"] == "2026-08-11T12:00:00Z"


def test_a_numeric_duration_is_read_as_seconds():
    assert a_query().duration(minimum=2, maximum=10.5).to_dict()["durationMin"] == "2s"
    assert a_query().duration(minimum=2, maximum=10.5).to_dict()["durationMax"] == "10.5s"


def test_an_unusable_time_or_duration_is_refused():
    with pytest.raises(TypeError, match="cannot use list as a timestamp"):
        Query().time_range([], END).to_dict()
    with pytest.raises(TypeError, match="cannot use list as a duration"):
        a_query().duration(minimum=[]).to_dict()


def test_the_url_form_carries_the_filter_as_json():
    query = a_query().where(attr("http.status_code") == 500).limit(20)
    params = parse_qs(query.to_query_string())
    assert params["query.startTimeMin"] == ["2026-08-11T12:00:00Z"]
    assert params["query.searchDepth"] == ["20"]
    assert params["query.filter"] == [
        '{"op":"eq","args":[{"attr":{"key":"http.status_code"}},{"scalar":{"value":"500"}}]}'
    ]


def test_the_url_form_carries_the_legacy_attributes_as_json():
    query = a_query().attributes({"http.status_code": "200"}).raw()
    params = query.to_url_params()
    assert params["query.attributes"] == '{"http.status_code":"200"}'
    assert params["query.rawTraces"] == "true"


def test_a_query_is_readable_when_printed():
    assert "cart" in repr(a_query().service("cart"))


def test_the_package_reports_an_unknown_attribute():
    import jaeger_query

    with pytest.raises(AttributeError, match="has no attribute 'Nonesuch'"):
        getattr(jaeger_query, "Nonesuch")  # noqa: B009 - the point is the failure
