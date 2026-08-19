# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

"""Search a real Jaeger with a real backend, through the SDK.

The rest of the suite stops at the wire: test_conformance.py proves the builder's
JSON parses into the generated messages, and nothing more. Three things only a
running Jaeger can answer — whether the server's validator accepts what the builder
emits, whether the backend's declared capabilities admit it, and whether the filter
matches the spans it should.

Skipped unless JAEGER_QUERY_ADDR names a query service, so `make test` stays
hermetic. `make e2e` supplies one, along with the OpenSearch behind it: see
e2e/run.sh. OpenSearch because Elasticsearch/OpenSearch is the only backend that
declares filter capabilities today, so it is the only one that evaluates a filter
rather than having it down-converted to the legacy predicate fields.
"""

from __future__ import annotations

import datetime as dt
import json
import os
import secrets
import time
import urllib.error
import urllib.request

import pytest

from jaeger_query import Query, QueryClient, attr, event, resource, span

pytestmark = pytest.mark.e2e

QUERY_ADDR = os.environ.get("JAEGER_QUERY_ADDR", "")
OTLP_HTTP = os.environ.get("JAEGER_OTLP_HTTP", "http://localhost:4318")

if not QUERY_ADDR:
    pytest.skip("set JAEGER_QUERY_ADDR to run the end-to-end test", allow_module_level=True)

# How long to wait for the query service to come up, and for OpenSearch to index what
# was sent. Both are generous: a slow CI runner is not a failure.
STARTUP_TIMEOUT = dt.timedelta(seconds=90)
INDEX_TIMEOUT = dt.timedelta(seconds=60)

# Elasticsearch/OpenSearch declares the span, resource and event levels, and every
# operator except `some`. A filter naming anything else is refused by the query
# service before it reaches storage, which is the contract rather than a defect, so
# this test stays inside what the backend advertises.
SERVICE = f"sdk-e2e-{secrets.token_hex(4)}"
OPERATION = "GET /basket"
SLOW_SPAN_NANOS = 2_500_000_000  # 2.5s, so `duration > 2s` matches and `> 5s` does not


def _otlp_payload(trace_id: str, span_id: str, start_nanos: int) -> dict:
    """One trace carrying something to match at each level the backend serves."""
    return {
        "resourceSpans": [
            {
                "resource": {
                    "attributes": [
                        {"key": "service.name", "value": {"stringValue": SERVICE}},
                        {"key": "deployment.environment", "value": {"stringValue": "staging"}},
                    ]
                },
                "scopeSpans": [
                    {
                        "scope": {"name": "sdk-e2e", "version": "0.0.1"},
                        "spans": [
                            {
                                "traceId": trace_id,
                                "spanId": span_id,
                                "name": OPERATION,
                                "kind": 2,  # server
                                "startTimeUnixNano": str(start_nanos),
                                "endTimeUnixNano": str(start_nanos + SLOW_SPAN_NANOS),
                                "attributes": [
                                    {"key": "http.status_code", "value": {"stringValue": "500"}},
                                    {"key": "http.method", "value": {"stringValue": "GET"}},
                                ],
                                "status": {"code": 2},  # error
                                "events": [
                                    {
                                        "timeUnixNano": str(start_nanos + 1_000_000),
                                        "name": "exception",
                                    }
                                ],
                            }
                        ],
                    }
                ],
            }
        ]
    }


def _send(payload: dict) -> None:
    """Post OTLP/HTTP JSON, which needs no dependency beyond the standard library."""
    request = urllib.request.Request(
        f"{OTLP_HTTP}/v1/traces",
        data=json.dumps(payload).encode(),
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    with urllib.request.urlopen(request, timeout=10) as response:  # noqa: S310 - a fixed local URL
        assert response.status < 300, f"the collector refused the spans: {response.status}"


def _wait_until(deadline: dt.timedelta, attempt):
    """Retry `attempt` until it returns something truthy, or give up at the deadline."""
    give_up = time.monotonic() + deadline.total_seconds()
    last: Exception | None = None
    while time.monotonic() < give_up:
        try:
            result = attempt()
            if result:
                return result
        except Exception as error:  # noqa: BLE001 - the point is to keep retrying
            last = error
        time.sleep(1)
    if last is not None:
        raise AssertionError(f"gave up after {deadline}: {last}") from last
    return None


@pytest.fixture(scope="module")
def client():
    with QueryClient(QUERY_ADDR, timeout=30) as connected:
        _wait_until(STARTUP_TIMEOUT, lambda: connected.get_services() is not None)
        yield connected


@pytest.fixture(scope="module")
def trace_id(client):
    """Send one trace and wait for the backend to index it."""
    identifier = secrets.token_hex(16)
    start = time.time_ns() - SLOW_SPAN_NANOS
    _send(_otlp_payload(identifier, secrets.token_hex(8), start))
    found = _wait_until(
        INDEX_TIMEOUT,
        lambda: [s for s in client.find_trace_summaries(_recent().service(SERVICE))],
    )
    assert found, f"the trace never turned up in storage for service {SERVICE}"
    return identifier


def _recent() -> Query:
    return Query().last(minutes=5).limit(20)


def _matches(client: QueryClient, *predicates) -> list[str]:
    query = _recent()
    for predicate in predicates:
        query = query.where(predicate)
    return [summary.trace_id for summary in client.find_trace_summaries(query)]


@pytest.mark.parametrize(
    ("name", "predicate"),
    [
        ("resource field", resource.service == SERVICE),
        ("span field", span.name == OPERATION),
        ("span attribute", span.attr("http.status_code") == "500"),
        ("unqualified attribute", attr("http.status_code") == "500"),
        ("resource attribute", resource.attr("deployment.environment") == "staging"),
        ("duration ordering", span.duration > "2s"),
        ("regex", span.name.matches("GET.*")),
        ("membership against a field", resource.service.one_of([SERVICE, "other"])),
        ("existence", span.attr("http.method").exists()),
        ("negation", ~(span.name == "something-else")),
        ("event level", event.name == "exception"),
    ],
)
def test_a_filter_the_backend_serves_finds_the_trace(client, trace_id, name, predicate):
    """Each predicate is one the backend advertises, so it must be evaluated, not refused."""
    found = _wait_until(INDEX_TIMEOUT, lambda: _matches(client, resource.service == SERVICE, predicate))
    assert trace_id in found, f"the {name} predicate did not match the trace"


@pytest.mark.parametrize(
    ("name", "predicate"),
    [
        ("wrong attribute value", span.attr("http.status_code") == "404"),
        ("duration above the span's", span.duration > "5s"),
        ("regex that cannot match", span.name.matches("no-such-operation")),
        ("absent attribute", span.attr("no.such.attribute").exists()),
    ],
)
def test_a_filter_that_should_not_match_returns_nothing(client, trace_id, name, predicate):
    """A filter that finds everything is as broken as one that finds nothing."""
    found = _matches(client, resource.service == SERVICE, predicate)
    assert trace_id not in found, f"the {name} predicate matched a trace it should not"


def test_a_conjunction_and_a_disjunction_both_reach_the_backend(client, trace_id):
    conjunction = (span.duration > "2s") & (span.attr("http.status_code") == "500")
    assert trace_id in _wait_until(
        INDEX_TIMEOUT, lambda: _matches(client, resource.service == SERVICE, conjunction)
    )
    disjunction = (span.name == "something-else") | (span.attr("http.status_code") == "500")
    assert trace_id in _matches(client, resource.service == SERVICE, disjunction)


# Each of these is refused rather than served, and the reason is the contract rather
# than a defect. RFC 0005 §7 promises a refusal over a silently widened answer, so
# what this asserts is that the refusal arrives — and being explicit about which
# filters cannot be served is half of what the test is for.
REFUSED = {
    # OpenSearch declares every operator except `some`, so quantifying over events
    # asks for what it does not advertise.
    "an operator the backend does not declare": event.some(event.name == "exception"),
    # A list compared against an attribute has to declare its element type, because an
    # attribute declares nothing itself. OpenSearch serves untyped constants only, so
    # the two requirements cannot both be met and membership against an attribute is
    # unserviceable there. Against a built-in field, which supplies the type, it works
    # — see the served cases above.
    "membership against an attribute": span.attr("http.method").one_of(["GET", "POST"]),
    "negated membership against an attribute": span.attr("http.method").not_one_of(["GET"]),
    # `regex` matches anywhere in the value, so a pattern that anchors itself is asking
    # for semantics the operator does not have.
    "an anchored pattern": span.name.matches("^GET"),
}


@pytest.mark.parametrize("reason", REFUSED.keys())
def test_a_filter_the_backend_cannot_serve_is_refused(client, trace_id, reason):
    """The refusal contract of RFC 0005 §7: an error, never a quietly widened answer."""
    import grpc

    query = _recent().where(resource.service == SERVICE).where(REFUSED[reason])
    with pytest.raises(grpc.RpcError) as refusal:
        list(client.find_trace_summaries(query))
    assert refusal.value.code() == grpc.StatusCode.INVALID_ARGUMENT, refusal.value.details()
