// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func Test_dateOffsetNormalizer(t *testing.T) {
	origTime := time.Date(
		2024, time.January, 10,
		14, 30, 45, 123456789,
		time.UTC,
	)
	origStartTime := time.Date(
		2017, time.January, 25,
		23, 56, 31, 639875000,
		time.UTC,
	)
	origTs := pcommon.NewTimestampFromTime(origTime)
	origStartTs := pcommon.NewTimestampFromTime(origStartTime)
	twoDaysAgo := -2
	oneDayAgo := -1
	now := time.Now()
	expectedTwoDaysAgo := now.UTC().AddDate(0, 0, twoDaysAgo)
	expectedOneDayAgo := now.UTC().AddDate(0, 0, oneDayAgo)
	normalizer := newDateOffsetNormalizer(now)
	td := ptrace.NewTraces()
	span := td.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span.SetStartTimestamp(origStartTs)
	span.SetEndTimestamp(origTs)
	event := span.Events().AppendEmpty()
	event.SetTimestamp(origTs)
	normalizer.normalizeTrace(td)
	expectedTwoDaysAgoTime := time.Date(
		expectedTwoDaysAgo.Year(),
		expectedTwoDaysAgo.Month(),
		expectedTwoDaysAgo.Day(),
		origStartTime.Hour(),
		origStartTime.Minute(),
		origStartTime.Second(),
		origStartTime.Nanosecond(),
		time.UTC,
	)
	expectedOneDayAgoTime := time.Date(
		expectedOneDayAgo.Year(),
		expectedOneDayAgo.Month(),
		expectedOneDayAgo.Day(),
		origTime.Hour(),
		origTime.Minute(),
		origTime.Second(),
		origTime.Nanosecond(),
		time.UTC,
	)
	assert.Equal(t, expectedTwoDaysAgoTime, span.StartTimestamp().AsTime())
	assert.Equal(t, expectedOneDayAgoTime, span.EndTimestamp().AsTime())
	assert.Equal(t, expectedOneDayAgoTime, event.Timestamp().AsTime())
}
