package spanstore

import (
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestSpanReaderFindIndices(t *testing.T) {
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
				indexWithDate(spanIndexBaseName, dateLayout, today),
			},
		},
		{
			startTime: today.Add(-13 * time.Hour),
			endTime:   today,
			expected: []string{
				indexWithDate(spanIndexBaseName, dateLayout, today),
				indexWithDate(spanIndexBaseName, dateLayout, yesterday),
			},
		},
		{
			startTime: today.Add(-48 * time.Hour),
			endTime:   today,
			expected: []string{
				indexWithDate(spanIndexBaseName, dateLayout, today),
				indexWithDate(spanIndexBaseName, dateLayout, yesterday),
				indexWithDate(spanIndexBaseName, dateLayout, twoDaysAgo),
			},
		},
	}
	for _, testCase := range testCases {
		actual := timeRangeIndices(spanIndexBaseName, dateLayout, testCase.startTime, testCase.endTime, -24*time.Hour)
		assert.Equal(t, testCase.expected, actual)
	}
}

func TestSpanReader_indexWithDate(t *testing.T) {
	actual := indexWithDate(spanIndexBaseName, "2006-01-02", time.Date(1995, time.April, 21, 4, 21, 19, 95, time.UTC))
	assert.Equal(t, "jaeger-span-1995-04-21", actual)
}
