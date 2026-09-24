#!/usr/bin/env python3
# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

"""Search a running Jaeger for slow or failed traces.

Start Jaeger with the all-in-one image and send it some traces, then run:

    .venv/bin/python examples/search.py --service cart

Passing no --address searches localhost:16685, the default gRPC port of the
query service.
"""

from __future__ import annotations

import argparse
import datetime as dt
import json

from jaeger_query import Query, QueryClient, event, resource, span


def build_query(args: argparse.Namespace) -> Query:
    """Ask for traces of one service that were slow, failed, or recorded an exception."""
    slow = span.duration > args.slower_than
    failed = span("http.status_code") >= 500
    threw = event.some(event.name == "exception")
    return (
        Query()
        .last(minutes=args.minutes)
        .where(resource.service == args.service)
        .where(slow | failed | threw)
        .limit(args.limit)
    )


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--address", default="localhost:16685")
    parser.add_argument("--service", default="cart")
    parser.add_argument("--minutes", type=int, default=15)
    parser.add_argument("--slower-than", default="2s")
    parser.add_argument("--limit", type=int, default=20)
    parser.add_argument("--full-traces", action="store_true", help="fetch whole spans, not summaries")
    args = parser.parse_args()

    query = build_query(args)
    # build_query always sets a filter; reading it through to_dict() needs no such
    # assumption, and is what the query itself sends.
    print("filter:", json.dumps(query.to_dict()["filter"], separators=(",", ":")))
    print("as a GET request: /api/v3/trace-summaries?" + query.to_query_string(), end="\n\n")

    with QueryClient(args.address) as client:
        if args.full_traces:
            for chunk in client.find_traces(query):
                for resource_spans in chunk.resource_spans:
                    for scope_spans in resource_spans.scope_spans:
                        for one in scope_spans.spans:
                            print(one.trace_id.hex(), one.name)
            return
        for summary in client.find_trace_summaries(query):
            started = dt.datetime.fromtimestamp(summary.min_start_time_unix_nano / 1e9, dt.UTC)
            duration_ms = (summary.max_end_time_unix_nano - summary.min_start_time_unix_nano) / 1e6
            print(
                f"{summary.trace_id}  {started:%H:%M:%S}  {duration_ms:8.1f}ms  "
                f"{summary.span_count:4d} spans  {summary.error_span_count:3d} errors  "
                f"{summary.root_service_name}: {summary.root_operation_name}"
            )


if __name__ == "__main__":
    main()
