# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

"""Reading times and durations from the several forms a caller may hold them in."""

from __future__ import annotations

import datetime as dt

UTC = dt.UTC


def to_utc(value: object) -> dt.datetime:
    """Read a time from a datetime, epoch seconds, or an RFC 3339 string, as UTC.

    A datetime without a timezone is read as UTC rather than local time, so that
    the same query means the same thing wherever it runs.
    """
    if isinstance(value, dt.datetime):
        return value.astimezone(UTC) if value.tzinfo else value.replace(tzinfo=UTC)
    if isinstance(value, str):
        return to_utc(dt.datetime.fromisoformat(value))
    if isinstance(value, (int, float)):
        return dt.datetime.fromtimestamp(value, UTC)
    raise TypeError(f"cannot use {type(value).__name__} as a timestamp")


def to_rfc3339(value: dt.datetime) -> str:
    """Render a time as the RFC 3339 string a proto3 Timestamp uses."""
    return value.isoformat().replace("+00:00", "Z")


def to_duration(value: object) -> str:
    """Render a duration as the seconds-with-suffix string a proto3 Duration uses.

    A string passes through unchanged, so it has to suit its destination: the
    proto3 JSON encoding accepts only seconds (``"90s"``), while the HTTP GET
    binding parses Go's syntax (``"1h30m"``). A timedelta always renders right.
    """
    if isinstance(value, str):
        return value
    if isinstance(value, dt.timedelta):
        return f"{value.total_seconds()}s"
    if isinstance(value, (int, float)):
        return f"{value}s"
    raise TypeError(f"cannot use {type(value).__name__} as a duration")
