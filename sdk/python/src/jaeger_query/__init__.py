# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

"""A prototype Python SDK for Jaeger trace search with RFC 0005 query filters.

Two pieces, usable apart:

* :mod:`jaeger_query.expression` and :mod:`jaeger_query.query` build a query.
  They are pure Python and render proto3 JSON, so they need nothing installed
  and suit the HTTP API as well as gRPC.
* :mod:`jaeger_query.client` executes one over gRPC. It needs grpcio, which the
  ``grpc`` extra installs.

``QueryClient`` is imported on first use, so the query builders stay importable
without grpcio present.

    from jaeger_query import Query, QueryClient, span, resource, some, event

    query = (
        Query()
        .last(minutes=15)
        .where(span.duration > "2s")
        .where(resource.service.one_of(["cart", "checkout"]))
        .limit(20)
    )
    with QueryClient() as client:
        for summary in client.find_trace_summaries(query):
            print(summary.trace_id, summary.root_operation_name)
"""

from __future__ import annotations

from typing import TYPE_CHECKING

from .expression import (
    AttributeRef,
    Call,
    Expr,
    FieldRef,
    ListValue,
    NestedRef,
    OrderedRef,
    Scalar,
    SpanKind,
    SpanStatus,
    ValueRef,
    and_,
    attr,
    event,
    link,
    not_,
    or_,
    resource,
    scope,
    some,
    span,
)
from .query import Query

if TYPE_CHECKING:
    from .client import QueryClient

__all__ = [
    "AttributeRef",
    "Call",
    "Expr",
    "FieldRef",
    "ListValue",
    "NestedRef",
    "OrderedRef",
    "Query",
    "QueryClient",
    "Scalar",
    "SpanKind",
    "SpanStatus",
    "ValueRef",
    "and_",
    "attr",
    "event",
    "link",
    "not_",
    "or_",
    "resource",
    "scope",
    "some",
    "span",
]

__version__ = "0.0.1"


def __getattr__(name: str) -> object:
    """Import the gRPC client on first use, so grpcio stays optional."""
    if name == "QueryClient":
        from .client import QueryClient

        return QueryClient
    raise AttributeError(f"module {__name__!r} has no attribute {name!r}")
