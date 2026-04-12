// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package indices

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

const spanIndexBaseName = "jaeger-span-"

func TestTimeRangeIndices(t *testing.T) {
	today := time.Date(1995, time.April, 21, 4, 12, 19, 95, time.UTC)
	yesterday := today.AddDate(0, 0, -1)
	twoDaysAgo := today.AddDate(0, 0, -2)
	dateLayout := "2006-01-02"

	testCases := []struct {
		startTime time.Time
		endTime   time.Time
		expected  []string
	}{
		{
			startTime: today.Add(-time.Millisecond),
			endTime:   today,
			expected: []string{
				IndexWithDate(spanIndexBaseName, dateLayout, today),
			},
		},
		{
			startTime: today.Add(-13 * time.Hour),
			endTime:   today,
			expected: []string{
				IndexWithDate(spanIndexBaseName, dateLayout, today),
				IndexWithDate(spanIndexBaseName, dateLayout, yesterday),
			},
		},
		{
			startTime: today.Add(-48 * time.Hour),
			endTime:   today,
			expected: []string{
				IndexWithDate(spanIndexBaseName, dateLayout, today),
				IndexWithDate(spanIndexBaseName, dateLayout, yesterday),
				IndexWithDate(spanIndexBaseName, dateLayout, twoDaysAgo),
			},
		},
	}
	for _, testCase := range testCases {
		actual := timeRangeIndices(spanIndexBaseName, dateLayout, testCase.startTime, testCase.endTime, -24*time.Hour)
		assert.Equal(t, testCase.expected, actual)
	}
}

func TestIndexWithDate(t *testing.T) {
	actual := IndexWithDate(spanIndexBaseName, "2006-01-02", time.Date(1995, time.April, 21, 4, 21, 19, 95, time.UTC))
	assert.Equal(t, "jaeger-span-1995-04-21", actual)
}

func TestLoggingTimeRangeIndexFn_NoDebug(t *testing.T) {
	logger := zap.NewNop()
	fn := TimeRangeIndicesFn(false, "", nil)
	wrapped := LoggingTimeRangeIndexFn(logger, fn)
	// should return the same function without debug logging
	start := time.Date(1995, time.April, 21, 0, 0, 0, 0, time.UTC)
	end := time.Date(1995, time.April, 21, 0, 0, 0, 0, time.UTC)
	result := wrapped(spanIndexBaseName, "2006-01-02", start, end, -24*time.Hour)
	assert.NotEmpty(t, result)
}

func TestLoggingTimeRangeIndexFn_WithDebug(t *testing.T) {
	logger := zaptest.NewLogger(t, zaptest.Level(zap.DebugLevel))
	fn := TimeRangeIndicesFn(false, "", nil)
	wrapped := LoggingTimeRangeIndexFn(logger, fn)
	start := time.Date(1995, time.April, 21, 0, 0, 0, 0, time.UTC)
	end := time.Date(1995, time.April, 21, 0, 0, 0, 0, time.UTC)
	result := wrapped(spanIndexBaseName, "2006-01-02", start, end, -24*time.Hour)
	assert.NotEmpty(t, result)
}

func TestTimeRangeIndicesFn_ReadWriteAliases(t *testing.T) {
	fn := TimeRangeIndicesFn(true, "", nil)
	result := fn(spanIndexBaseName, "2006-01-02", time.Now(), time.Now(), -24*time.Hour)
	assert.Equal(t, []string{spanIndexBaseName + "read"}, result)
}

func TestTimeRangeIndicesFn_ReadWriteAliasesCustomSuffix(t *testing.T) {
	fn := TimeRangeIndicesFn(true, "custom", nil)
	result := fn(spanIndexBaseName, "2006-01-02", time.Now(), time.Now(), -24*time.Hour)
	assert.Equal(t, []string{spanIndexBaseName + "custom"}, result)
}

func TestAddRemoteReadClusters(t *testing.T) {
	fn := TimeRangeIndicesFn(false, "", []string{"cluster1", "cluster2"})
	start := time.Date(1995, time.April, 21, 0, 0, 0, 0, time.UTC)
	end := time.Date(1995, time.April, 21, 0, 0, 0, 0, time.UTC)
	result := fn(spanIndexBaseName, "2006-01-02", start, end, -24*time.Hour)
	assert.Contains(t, result, spanIndexBaseName+"1995-04-21")
	assert.Contains(t, result, "cluster1:"+spanIndexBaseName+"1995-04-21")
	assert.Contains(t, result, "cluster2:"+spanIndexBaseName+"1995-04-21")
}
