// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jptrace

import "time"

// TimeToUnixNano converts t to Unix nanoseconds as uint64.
// Returns 0 for zero or pre-epoch times so the proto3 default value is used
// (field omitted), which is consistent with OTLP JSON encoding.
func TimeToUnixNano(t time.Time) uint64 {
	if t.IsZero() {
		return 0
	}
	nano := t.UnixNano()
	if nano < 0 {
		return 0
	}
	return uint64(nano)
}

// UnixNanoToTime converts Unix nanoseconds encoded as uint64 to time.Time.
// Returns the zero time for a zero input.
func UnixNanoToTime(nano uint64) time.Time {
	if nano == 0 {
		return time.Time{}
	}
	return time.Unix(0, int64(nano)).UTC() //nolint:gosec // G115
}
