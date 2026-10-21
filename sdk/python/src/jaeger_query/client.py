# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

"""A gRPC client for Jaeger's ``jaeger.api_v3.QueryService``.

:class:`QueryClient` sends the queries that :mod:`jaeger_query.query` builds and
returns the protobuf responses. It converts a :class:`~jaeger_query.query.Query`
by way of its proto3 JSON form, so the query builder never has to know about
protobuf or gRPC::

    with QueryClient("localhost:16685") as client:
        for summary in client.find_trace_summaries(
            Query().last(minutes=15).where(span.duration > "2s")
        ):
            print(summary.trace_id, summary.root_operation_name)

Importing this module needs grpcio and the generated api_v3 stubs, which are
committed under ``src/api_v3``. Install the ``grpc`` extra to get the runtime.
"""

from __future__ import annotations

from collections.abc import Iterator, Mapping, Sequence

import grpc
from google.protobuf import json_format
from opentelemetry.proto.trace.v1 import trace_pb2

from api_v3 import query_service_pb2 as pb
from api_v3 import query_service_pb2_grpc as pb_grpc

from ._time import to_utc
from .query import Query

__all__ = ["QueryClient"]

#: The port Jaeger's query service serves gRPC on by default.
DEFAULT_ADDRESS = "localhost:16685"


class QueryClient:
    """A connection to one Jaeger query service.

    Pass ``credentials`` for a TLS connection, or ``tls=True`` to use the
    system trust roots. Pass ``channel`` to supply a channel built elsewhere,
    in which case closing the client leaves it open.
    """

    def __init__(
        self,
        address: str = DEFAULT_ADDRESS,
        *,
        credentials: grpc.ChannelCredentials | None = None,
        tls: bool = False,
        channel: grpc.Channel | None = None,
        timeout: float | None = None,
        metadata: Mapping[str, str] | None = None,
        options: Sequence[tuple[str, object]] | None = None,
    ) -> None:
        self._timeout = timeout
        self._metadata = tuple((metadata or {}).items())
        self._owns_channel = channel is None
        if channel is not None:
            self._channel = channel
        elif credentials is not None or tls:
            self._channel = grpc.secure_channel(
                address, credentials or grpc.ssl_channel_credentials(), options
            )
        else:
            self._channel = grpc.insecure_channel(address, options)
        self._stub = pb_grpc.QueryServiceStub(self._channel)

    def close(self) -> None:
        """Close the channel, unless it was supplied by the caller."""
        if self._owns_channel:
            self._channel.close()

    def __enter__(self) -> QueryClient:
        return self

    def __exit__(self, *exc: object) -> None:
        self.close()

    # -- search --------------------------------------------------------------

    def find_traces(self, query: Query) -> Iterator[trace_pb2.TracesData]:
        """Search for traces, yielding the response chunks as they arrive.

        Each chunk carries whole spans. Use :meth:`find_trace_summaries` when a
        search-results listing is all that is needed.
        """
        request = pb.FindTracesRequest(query=self.to_proto(query))
        yield from self._stub.FindTraces(request, timeout=self._timeout, metadata=self._metadata)

    def find_trace_summaries(self, query: Query) -> Iterator[pb.TraceSummary]:
        """Search for traces, yielding one lightweight summary per match."""
        request = pb.FindTraceSummariesRequest(query=self.to_proto(query))
        stream = self._stub.FindTraceSummaries(request, timeout=self._timeout, metadata=self._metadata)
        for chunk in stream:
            yield from chunk.summaries

    def get_trace(
        self,
        trace_id: str,
        *,
        start_time: object = None,
        end_time: object = None,
        raw_traces: bool = False,
    ) -> Iterator[trace_pb2.TracesData]:
        """Fetch one trace by its hex-encoded ID, yielding the response chunks.

        The times narrow which storage partitions are read; they are optional
        but make the lookup cheaper on a time-partitioned backend.
        """
        request = pb.GetTraceRequest(trace_id=trace_id, raw_traces=raw_traces)
        if start_time is not None:
            request.start_time.FromDatetime(to_utc(start_time))
        if end_time is not None:
            request.end_time.FromDatetime(to_utc(end_time))
        yield from self._stub.GetTrace(request, timeout=self._timeout, metadata=self._metadata)

    # -- metadata ------------------------------------------------------------

    def get_services(self) -> list[str]:
        """List the service names the backend knows about."""
        response = self._stub.GetServices(
            pb.GetServicesRequest(), timeout=self._timeout, metadata=self._metadata
        )
        return list(response.services)

    def get_operations(self, service: str, span_kind: str = "") -> list[pb.Operation]:
        """List one service's operations, optionally narrowed to one span kind."""
        response = self._stub.GetOperations(
            pb.GetOperationsRequest(service=service, span_kind=span_kind),
            timeout=self._timeout,
            metadata=self._metadata,
        )
        return list(response.operations)

    def get_dependencies(self, start_time: object, end_time: object) -> list[pb.Dependency]:
        """Read the service dependency graph over a time range."""
        request = pb.GetDependenciesRequest()
        request.start_time.FromDatetime(to_utc(start_time))
        request.end_time.FromDatetime(to_utc(end_time))
        response = self._stub.GetDependencies(request, timeout=self._timeout, metadata=self._metadata)
        return list(response.dependencies)

    # -- conversion ----------------------------------------------------------

    @staticmethod
    def to_proto(query: Query) -> pb.TraceQueryParameters:
        """Convert a :class:`~jaeger_query.query.Query` to its protobuf form.

        The query renders itself as proto3 JSON, which the protobuf runtime
        parses into the message. That keeps the query builder free of any
        protobuf dependency, at the cost of one small serialization hop.
        """
        return json_format.ParseDict(query.to_dict(), pb.TraceQueryParameters())
