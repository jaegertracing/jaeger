// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
)

// dateOffsetNormalizer normalizes timestamps by replacing their date
// (year, month, day) with a fixed date computed using a day offset
// from the current UTC date, while preserving the original time
// This is required in integration tests because the fixtures have
// hardcoded start time and other time stamps and we need to make
// recent to fetch from the reader.
type dateOffsetNormalizer struct {
	year, day int
	month     time.Month
}

func newDateOffsetNormalizer(dayOffset int) dateOffsetNormalizer {
	now := time.Now().UTC()
	d := dateOffsetNormalizer{}
	d.year, d.month, d.day = now.AddDate(0, 0, dayOffset).Date()
	return d
}

func (d dateOffsetNormalizer) normalize(t pcommon.Timestamp) pcommon.Timestamp {
	tm := t.AsTime()
	newTm := time.Date(
		d.year,
		d.month,
		d.day,
		tm.Hour(),
		tm.Minute(),
		tm.Second(),
		tm.Nanosecond(),
		tm.Location(),
	)
	return pcommon.NewTimestampFromTime(newTm)
}
