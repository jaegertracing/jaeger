// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/jptrace"
)

// dateOffsetNormalizer normalizes timestamps by replacing their date
// (year, month, day) with a fixed date computed using a day offset
// from the current UTC date, while preserving the original time.
// This is required in integration tests because the fixtures have
// hardcoded start time and other timestamps and we need to make them
// recent to fetch from the reader. Timestamps whose original UTC date
// is 2017-01-25 use a -2 day offset; all other timestamps use a -1 day
// offset. These offsets are selected to keep the upgrade from v1 to v2 consistent.
type dateOffsetNormalizer struct {
	tm time.Time
}

func newDateOffsetNormalizer(tm time.Time) dateOffsetNormalizer {
	return dateOffsetNormalizer{tm: tm}
}

func (d dateOffsetNormalizer) normalizeTrace(td ptrace.Traces) {
	for _, span := range jptrace.SpanIter(td) {
		span.SetStartTimestamp(d.normalizeTime(span.StartTimestamp()))
		span.SetEndTimestamp(d.normalizeTime(span.EndTimestamp()))
		for _, event := range span.Events().All() {
			event.SetTimestamp(d.normalizeTime(event.Timestamp()))
		}
	}
}

func (d dateOffsetNormalizer) normalizeTime(t pcommon.Timestamp) pcommon.Timestamp {
	tm := t.AsTime().UTC()
	offset := -1
	// Apply a -2 day offset for any timestamp whose UTC date is 2017-01-25.
	// All other timestamps use the default -1 day offset to preserve existing behavior.
	yearOrig, monthOrig, dayOrig := tm.Date()
	if yearOrig == 2017 && monthOrig == time.January && dayOrig == 25 {
		offset = -2
	}
	year, month, day := d.tm.UTC().AddDate(0, 0, offset).Date()
	newTm := time.Date(
		year,
		month,
		day,
		tm.Hour(),
		tm.Minute(),
		tm.Second(),
		tm.Nanosecond(),
		tm.Location(),
	)
	return pcommon.NewTimestampFromTime(newTm)
}
