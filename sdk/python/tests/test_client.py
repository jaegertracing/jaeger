# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

"""The client is exercised against a real gRPC server that records what it received."""

import datetime as dt
from concurrent import futures

import grpc
import pytest
from opentelemetry.proto.trace.v1 import trace_pb2

from api_v3 import query_service_pb2 as pb
from api_v3 import query_service_pb2_grpc as pb_grpc
from jaeger_query import Query, QueryClient, attr, span

START = dt.datetime(2026, 8, 11, 12, 0, tzinfo=dt.UTC)
END = dt.datetime(2026, 8, 11, 13, 0, tzinfo=dt.UTC)


class RecordingQueryService(pb_grpc.QueryServiceServicer):
    """A stand-in for Jaeger's query service, answering with fixed data."""

    def __init__(self):
        self.received = []

    def FindTraces(self, request, context):
        self.received.append(request)
        for name in ("first", "second"):
            yield _traces_data(name)

    def FindTraceSummaries(self, request, context):
        self.received.append(request)
        yield pb.FindTraceSummariesResponse(
            summaries=[pb.TraceSummary(trace_id="aaaa"), pb.TraceSummary(trace_id="bbbb")]
        )
        yield pb.FindTraceSummariesResponse(summaries=[pb.TraceSummary(trace_id="cccc")])

    def GetTrace(self, request, context):
        self.received.append(request)
        yield _traces_data(request.trace_id)

    def GetServices(self, request, context):
        self.received.append(request)
        return pb.GetServicesResponse(services=["cart", "checkout"])

    def GetOperations(self, request, context):
        self.received.append(request)
        return pb.GetOperationsResponse(
            operations=[pb.Operation(name="GET /basket", span_kind=request.span_kind or "server")]
        )

    def GetDependencies(self, request, context):
        self.received.append(request)
        return pb.DependenciesResponse(
            dependencies=[pb.Dependency(parent="cart", child="checkout", call_count=7)]
        )


def _traces_data(span_name: str) -> trace_pb2.TracesData:
    return trace_pb2.TracesData(
        resource_spans=[
            trace_pb2.ResourceSpans(
                scope_spans=[trace_pb2.ScopeSpans(spans=[trace_pb2.Span(name=span_name)])]
            )
        ]
    )


@pytest.fixture
def service():
    servicer = RecordingQueryService()
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=2))
    pb_grpc.add_QueryServiceServicer_to_server(servicer, server)
    port = server.add_insecure_port("localhost:0")
    server.start()
    with QueryClient(f"localhost:{port}", timeout=10, metadata={"x-tenant": "acme"}) as client:
        yield client, servicer
    server.stop(grace=None)


def test_find_traces_streams_every_chunk(service):
    client, servicer = service
    chunks = list(client.find_traces(Query().time_range(START, END).where(span.duration > "2s")))
    assert [c.resource_spans[0].scope_spans[0].spans[0].name for c in chunks] == ["first", "second"]
    assert servicer.received[0].query.filter.op == "gt"


def test_the_filter_reaches_the_server_intact(service):
    client, servicer = service
    query = Query().time_range(START, END).where(attr("http.status_code").one_of([500, 503]))
    list(client.find_traces(query))
    sent = servicer.received[0].query
    assert sent.start_time_min.ToDatetime(dt.UTC) == START
    assert sent.start_time_max.ToDatetime(dt.UTC) == END
    assert sent.filter.op == "in"
    assert sent.filter.args[0].attr.key == "http.status_code"
    assert sent.filter.args[0].attr.level == ""
    assert list(sent.filter.args[1].list.values) == ["500", "503"]


def test_a_legacy_query_reaches_the_server_as_the_scalar_fields(service):
    client, servicer = service
    query = Query().time_range(START, END).service("cart").attributes({"error": "true"}).limit(5)
    list(client.find_traces(query))
    sent = servicer.received[0].query
    assert sent.service_name == "cart"
    assert dict(sent.attributes) == {"error": "true"}
    assert sent.search_depth == 5
    assert not sent.HasField("filter")


def test_find_trace_summaries_flattens_the_chunks(service):
    client, _ = service
    summaries = list(client.find_trace_summaries(Query().time_range(START, END).service("cart")))
    assert [s.trace_id for s in summaries] == ["aaaa", "bbbb", "cccc"]


def test_get_trace_passes_the_optional_time_bounds(service):
    client, servicer = service
    chunks = list(client.get_trace("abc123", start_time=START, end_time=END, raw_traces=True))
    assert chunks[0].resource_spans[0].scope_spans[0].spans[0].name == "abc123"
    sent = servicer.received[0]
    assert sent.raw_traces is True
    assert sent.start_time.ToDatetime(dt.UTC) == START


def test_get_trace_without_time_bounds_leaves_them_unset(service):
    client, servicer = service
    list(client.get_trace("abc123"))
    sent = servicer.received[0]
    assert not sent.HasField("start_time")
    assert not sent.HasField("end_time")


def test_the_metadata_endpoints_unwrap_their_responses(service):
    client, servicer = service
    assert client.get_services() == ["cart", "checkout"]
    assert [o.name for o in client.get_operations("cart")] == ["GET /basket"]
    assert client.get_operations("cart", "client")[0].span_kind == "client"
    dependencies = client.get_dependencies(START, END)
    assert (dependencies[0].parent, dependencies[0].child, dependencies[0].call_count) == (
        "cart",
        "checkout",
        7,
    )
    assert servicer.received[-1].start_time.ToDatetime(dt.UTC) == START


def test_a_caller_supplied_channel_outlives_the_client():
    servicer = RecordingQueryService()
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=2))
    pb_grpc.add_QueryServiceServicer_to_server(servicer, server)
    port = server.add_insecure_port("localhost:0")
    server.start()
    channel = grpc.insecure_channel(f"localhost:{port}")
    with QueryClient(channel=channel) as client:
        assert client.get_services() == ["cart", "checkout"]
    # The client did not open the channel, so it did not close it either.
    assert QueryClient(channel=channel).get_services() == ["cart", "checkout"]
    channel.close()
    server.stop(grace=None)


def test_a_tls_client_builds_a_secure_channel():
    with QueryClient("localhost:16685", tls=True) as client:
        assert isinstance(client._channel, grpc.Channel)


def test_a_query_without_a_time_range_fails_before_any_request(service):
    client, servicer = service
    with pytest.raises(ValueError, match="start_time_min is required"):
        list(client.find_traces(Query().service("cart")))
    assert servicer.received == []
