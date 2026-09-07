# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

"""Fluent construction of ``jaeger.api_v3.TraceQueryParameters``.

A :class:`Query` carries the time range that every trace search needs plus one
of the two ways to say what to match: the RFC 0005 structured filter built with
:mod:`jaeger_query.expression`, or the legacy predicate fields the API has
always had. Each method returns a new :class:`Query`, so a partly built one can
be reused::

    recent = Query().last(minutes=15).limit(50)
    slow = recent.where(span.duration > "2s")
    failed = recent.where(span("http.status_code") >= 500)

The rendered form is the proto3 JSON of ``TraceQueryParameters``, which
:mod:`jaeger_query.client` turns into a protobuf message. Rendering it as URL
parameters instead drives the HTTP GET binding of the same API.
"""

from __future__ import annotations

import datetime as dt
import json
from collections.abc import Mapping
from urllib.parse import urlencode

from ._time import to_duration, to_rfc3339, to_utc
from .expression import Call, and_

__all__ = ["Query"]

# The predicate fields that predate the structured filter, and what counts as
# having set each one. RFC 0005 §7 makes them mutually exclusive with the filter:
# a request is either legacy-style or filter-style, and the server rejects a mix.
#
# "Set" has to mean exactly what to_dict() renders, or the two disagree and a
# request goes out carrying both. A zero duration is the case that separates them:
# `duration(minimum=0)` renders `durationMin: "0s"`, so it is set, while a plain
# truthiness test reads 0 as absent and lets it travel beside a filter.
_LEGACY_FIELDS = {
    "service_name": bool,
    "operation_name": bool,
    "attributes": bool,
    "duration_min": lambda v: v is not None,
    "duration_max": lambda v: v is not None,
}


class Query:
    """The parameters of a trace search."""

    __slots__ = (
        "_attributes",
        "_duration_max",
        "_duration_min",
        "_filter",
        "_operation_name",
        "_raw_traces",
        "_search_depth",
        "_service_name",
        "_start_time_max",
        "_start_time_min",
    )

    def __init__(
        self,
        *,
        service_name: str = "",
        operation_name: str = "",
        attributes: Mapping[str, str] | None = None,
        start_time_min: object = None,
        start_time_max: object = None,
        duration_min: object = None,
        duration_max: object = None,
        search_depth: int = 0,
        raw_traces: bool = False,
        filter: Call | None = None,
    ) -> None:
        self._service_name = service_name
        self._operation_name = operation_name
        self._attributes = dict(attributes) if attributes else None
        self._start_time_min = start_time_min
        self._start_time_max = start_time_max
        self._duration_min = duration_min
        self._duration_max = duration_max
        self._search_depth = search_depth
        self._raw_traces = raw_traces
        self._filter = filter

    # -- structured filter ---------------------------------------------------

    def where(self, *predicates: Call) -> Query:
        """Add predicates to the structured filter, conjoined with any already there."""
        existing = [self._filter] if self._filter is not None else []
        return self._with(filter=and_(*existing, *predicates))._reject_mixed_filter()

    # -- legacy predicate fields ---------------------------------------------

    def service(self, name: str) -> Query:
        """Filter by service name. Legacy; ``resource.service`` is the filter form."""
        return self._with(service_name=name)._reject_mixed_filter()

    def operation(self, name: str) -> Query:
        """Filter by operation name. Legacy; ``span.name`` is the filter form."""
        return self._with(operation_name=name)._reject_mixed_filter()

    def attributes(self, attributes: Mapping[str, str]) -> Query:
        """Filter by unqualified attribute equality. Legacy; :func:`~.expression.attr` is the filter form."""
        merged = {**(self._attributes or {}), **attributes}
        return self._with(attributes=merged)._reject_mixed_filter()

    def duration(self, minimum: object = None, maximum: object = None) -> Query:
        """Bound span duration inclusively. Legacy; ``span.duration`` is the filter form."""
        return self._with(
            duration_min=self._duration_min if minimum is None else minimum,
            duration_max=self._duration_max if maximum is None else maximum,
        )._reject_mixed_filter()

    # -- envelope ------------------------------------------------------------

    def time_range(self, start: object, end: object) -> Query:
        """Set the required search window: start inclusive, end exclusive.

        Accepts a :class:`~datetime.datetime`, epoch seconds, or an RFC 3339
        string. A naive datetime is read as UTC.
        """
        return self._with(start_time_min=start, start_time_max=end)

    def last(self, **delta: float) -> Query:
        """Search a window ending now, sized by :class:`~datetime.timedelta` arguments.

        For example ``last(minutes=15)`` or ``last(hours=1, minutes=30)``.
        """
        end = dt.datetime.now(dt.UTC)
        return self.time_range(end - dt.timedelta(**delta), end)

    def limit(self, search_depth: int) -> Query:
        """Cap how deep the backend searches, roughly an SQL ``LIMIT``."""
        return self._with(search_depth=search_depth)

    def raw(self, raw_traces: bool = True) -> Query:
        """Return traces as stored, skipping enrichment such as clock-skew adjustment."""
        return self._with(raw_traces=raw_traces)

    # -- rendering -----------------------------------------------------------

    @property
    def filter(self) -> Call | None:
        """The structured filter, or None for a legacy-style query."""
        return self._filter

    def to_dict(self) -> dict:
        """Render as the proto3 JSON of ``TraceQueryParameters``."""
        start = to_utc(self._require_time("_start_time_min", "start_time_min"))
        end = to_utc(self._require_time("_start_time_max", "start_time_max"))
        if start >= end:
            raise ValueError(f"start_time_min ({start}) must be before start_time_max ({end})")
        params: dict = {"startTimeMin": to_rfc3339(start), "startTimeMax": to_rfc3339(end)}
        if self._service_name:
            params["serviceName"] = self._service_name
        if self._operation_name:
            params["operationName"] = self._operation_name
        if self._attributes:
            params["attributes"] = dict(self._attributes)
        if self._duration_min is not None:
            params["durationMin"] = to_duration(self._duration_min)
        if self._duration_max is not None:
            params["durationMax"] = to_duration(self._duration_max)
        if self._search_depth:
            params["searchDepth"] = self._search_depth
        if self._raw_traces:
            params["rawTraces"] = True
        if self._filter is not None:
            params["filter"] = self._filter.to_dict()
        return params

    def to_url_params(self) -> dict[str, str]:
        """Render as the ``query.*`` parameters of the HTTP GET binding.

        The attributes map and the filter travel as JSON strings, as that
        binding has no shape for a map or a nested message.
        """
        params: dict[str, str] = {}
        for key, value in self.to_dict().items():
            if key in ("attributes", "filter"):
                value = json.dumps(value, separators=(",", ":"))
            elif not isinstance(value, str):
                value = json.dumps(value)
            params[f"query.{key}"] = value
        return params

    def to_query_string(self) -> str:
        """Render as a percent-encoded query string for ``GET /api/v3/traces``."""
        return urlencode(self.to_url_params())

    def __repr__(self) -> str:
        set_fields = [(s.lstrip("_"), getattr(self, s)) for s in self.__slots__ if getattr(self, s)]
        return f"Query({', '.join(f'{name}={value!r}' for name, value in set_fields)})"

    # -- internals -----------------------------------------------------------

    def _with(self, **changes: object) -> Query:
        current = {slot.lstrip("_"): getattr(self, slot) for slot in self.__slots__}
        return Query(**{**current, **changes})  # type: ignore[arg-type]

    def _reject_mixed_filter(self) -> Query:
        if self._filter is None:
            return self
        set_legacy = [f for f, is_set in _LEGACY_FIELDS.items() if is_set(getattr(self, f"_{f}"))]
        if set_legacy:
            raise ValueError(
                f"the structured filter is mutually exclusive with the legacy predicate fields, "
                f"but {', '.join(set_legacy)} is also set — express it in the filter instead "
                f"(RFC 0005 §7)"
            )
        return self

    def _require_time(self, slot: str, name: str) -> object:
        value = getattr(self, slot)
        if value is None:
            raise ValueError(f"{name} is required — set a time range with time_range() or last()")
        return value
