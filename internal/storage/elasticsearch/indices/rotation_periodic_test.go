// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package indices

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
)

func TestPeriodicRotation_WriteTarget(t *testing.T) {
	r := NewPeriodicRotation(config.SpanIndexName, "2006-01-02", 24*time.Hour)
	date := time.Date(1995, time.April, 21, 22, 8, 41, 0, time.UTC)
	assert.Equal(t, "jaeger-span-1995-04-21", r.WriteTarget(date))
}

func TestPeriodicRotation_WriteTarget_Hourly(t *testing.T) {
	r := NewPeriodicRotation(config.SpanIndexName, "2006-01-02-15", time.Hour)
	date := time.Date(2019, time.October, 10, 5, 0, 0, 0, time.UTC)
	assert.Equal(t, "jaeger-span-2019-10-10-05", r.WriteTarget(date))
}

func TestPeriodicRotation_ReadTargets(t *testing.T) {
	r := NewPeriodicRotation(config.SpanIndexName, "2006-01-02", 24*time.Hour)
	today := time.Date(1995, time.April, 21, 4, 12, 19, 95, time.UTC)
	yesterday := today.AddDate(0, 0, -1)

	tests := []struct {
		name     string
		start    time.Time
		end      time.Time
		expected []string
	}{
		{
			name:     "same day",
			start:    today.Add(-time.Millisecond),
			end:      today,
			expected: []string{"jaeger-span-1995-04-21"},
		},
		{
			name:  "spans two days",
			start: today.Add(-13 * time.Hour),
			end:   today,
			expected: []string{
				"jaeger-span-1995-04-21",
				"jaeger-span-1995-04-20",
			},
		},
		{
			name:  "spans three days",
			start: yesterday.Add(-24 * time.Hour),
			end:   today,
			expected: []string{
				"jaeger-span-1995-04-21",
				"jaeger-span-1995-04-20",
				"jaeger-span-1995-04-19",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, r.ReadTargets(tt.start, tt.end))
		})
	}
}

func TestPeriodicRotation_ReadTargets_Hourly(t *testing.T) {
	r := NewPeriodicRotation(config.SpanIndexName, "2006-01-02-15", time.Hour)
	end := time.Date(2019, time.October, 10, 1, 15, 0, 0, time.UTC)
	assert.Equal(t, []string{
		"jaeger-span-2019-10-10-01",
		"jaeger-span-2019-10-10-00",
		"jaeger-span-2019-10-09-23",
	}, r.ReadTargets(end.Add(-2*time.Hour), end))
}

func TestPeriodicRotation_ReadTargets_WideRange(t *testing.T) {
	daily := NewPeriodicRotation("prod-jaeger-span", "2006-01-02", 24*time.Hour)
	hourly := NewPeriodicRotation("prod-jaeger-span", "2006-01-02-15", time.Hour)
	end := time.Date(2026, time.October, 6, 12, 0, 0, 0, time.UTC)

	for _, tt := range []struct {
		name     string
		rotation *PeriodicRotation
		period   time.Duration
	}{
		{name: "hourly", rotation: hourly, period: time.Hour},
		{name: "daily", rotation: daily, period: 24 * time.Hour},
	} {
		t.Run(tt.name+" enumerates up to the length bound", func(t *testing.T) {
			maxNames := maxReadTargetsLen / (len(tt.rotation.WriteTarget(end)) + 1)
			// A range of n periods touches n+1 of them, so this is the longest range that is enumerated.
			start := end.Add(-time.Duration(maxNames-1) * tt.period)

			targets := tt.rotation.ReadTargets(start, end)
			require.Len(t, targets, maxNames)
			assert.Equal(t, tt.rotation.WriteTarget(end), targets[0])
			assert.Equal(t, tt.rotation.WriteTarget(start), targets[len(targets)-1])
			assert.LessOrEqual(t, len(strings.Join(targets, ",")), maxReadTargetsLen)

			wildcards := []string{"prod-jaeger-span-1*", "prod-jaeger-span-2*"}
			assert.Equal(t, wildcards, tt.rotation.ReadTargets(start.Add(-tt.period), end), "one period longer")
			assert.Equal(t, wildcards, tt.rotation.ReadTargets(start.Add(-time.Nanosecond), end), "one nanosecond longer")
		})
	}

	for _, tt := range []struct {
		name     string
		rotation *PeriodicRotation
		start    time.Time
		end      time.Time
	}{
		{
			name:     "hourly with the longest lookback",
			rotation: hourly,
			start:    end.Add(-time.Duration(math.MaxInt64)),
			end:      end,
		},
		{
			name:     "daily across all four-digit years",
			rotation: daily,
			start:    time.Date(1, time.January, 1, 0, 0, 0, 0, time.UTC),
			end:      time.Date(9999, time.December, 31, 23, 59, 59, 0, time.UTC),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, []string{"prod-jaeger-span-1*", "prod-jaeger-span-2*"}, tt.rotation.ReadTargets(tt.start, tt.end))
			allocs := testing.AllocsPerRun(1, func() {
				tt.rotation.ReadTargets(tt.start, tt.end)
			})
			assert.Less(t, allocs, 10.0, "allocations must not grow with the number of periods")
		})
	}

	t.Run("layout not starting with the year", func(t *testing.T) {
		dayFirst := NewPeriodicRotation("prod-jaeger-span", "02-01-2006", 24*time.Hour)
		start := end.AddDate(-1, 0, 0)
		assert.Equal(t, []string{"prod-jaeger-span-*"}, dayFirst.ReadTargets(start, end))
	})
}

func TestPeriodicRotation_ExactTargets(t *testing.T) {
	hourly := NewPeriodicRotation("prod-jaeger-span", "2006-01-02-15", time.Hour)
	daily := NewPeriodicRotation("prod-jaeger-span", "2006-01-02", 24*time.Hour)
	end := time.Date(2026, time.October, 6, 12, 0, 0, 0, time.UTC)

	t.Run("matches ReadTargets inside the request-line budget", func(t *testing.T) {
		for _, tt := range []struct {
			name     string
			rotation *PeriodicRotation
			period   time.Duration
		}{
			{name: "hourly", rotation: hourly, period: time.Hour},
			{name: "daily", rotation: daily, period: 24 * time.Hour},
		} {
			t.Run(tt.name, func(t *testing.T) {
				maxNames := maxReadTargetsLen / (len(tt.rotation.WriteTarget(end)) + 1)
				start := end.Add(-time.Duration(maxNames-1) * tt.period)
				assert.Equal(t, tt.rotation.ReadTargets(start, end), tt.rotation.ExactTargets(start, end))

				over := start.Add(-time.Nanosecond)
				assert.Contains(t, strings.Join(tt.rotation.ReadTargets(over, end), ","), "*")
				exact := tt.rotation.ExactTargets(over, end)
				require.NotEmpty(t, exact)
				assert.Equal(t, tt.rotation.WriteTarget(end), exact[0])
				assert.NotContains(t, strings.Join(exact, ","), "*")
				assert.LessOrEqual(t, len(exact), maxNames+1)
			})
		}
	})

	t.Run("past the request-line budget stays concrete", func(t *testing.T) {
		start := end.Add(-200 * time.Hour)
		assert.Contains(t, strings.Join(hourly.ReadTargets(start, end), ","), "*")
		var want []string
		for ts := end; !ts.Before(start); ts = ts.Add(-time.Hour) {
			name := hourly.WriteTarget(ts)
			if len(want) == 0 || want[len(want)-1] != name {
				want = append(want, name)
			}
		}
		assert.Equal(t, want, hourly.ExactTargets(start, end))
	})

	t.Run("caps a huge range at the newest names", func(t *testing.T) {
		got := hourly.ExactTargets(end.Add(-time.Duration(math.MaxInt64)), end)
		require.Len(t, got, maxExactTargets)
		assert.Equal(t, hourly.WriteTarget(end), got[0])
		assert.Equal(t, hourly.WriteTarget(end.Add(-time.Hour)), got[1])
		oldest := hourly.WriteTarget(end.Add(-time.Duration(maxExactTargets-1) * time.Hour))
		assert.Equal(t, oldest, got[len(got)-1])
		assert.NotContains(t, strings.Join(got, ","), "*")
	})

	t.Run("allocations do not grow with the range", func(t *testing.T) {
		long := end.Add(-time.Duration(math.MaxInt64))
		alsoLong := end.Add(-5 * 365 * 24 * time.Hour)
		allocsLong := testing.AllocsPerRun(1, func() { hourly.ExactTargets(long, end) })
		allocsAlso := testing.AllocsPerRun(1, func() { hourly.ExactTargets(alsoLong, end) })
		assert.InDelta(t, allocsLong, allocsAlso, 20)
		// Each concrete name allocates twice (the formatted date and the joined index
		// name). An uncapped walk of either range would allocate far more than this.
		assert.Less(t, allocsLong, float64(2*maxExactTargets+200))
	})

	t.Run("layout not starting with the year", func(t *testing.T) {
		dayFirst := NewPeriodicRotation("prod-jaeger-span", "02-01-2006", 24*time.Hour)
		start := end.AddDate(-1, 0, 0)
		assert.Equal(t, []string{"prod-jaeger-span-*"}, dayFirst.ReadTargets(start, end))
		got := dayFirst.ExactTargets(start, end)
		require.NotEmpty(t, got)
		assert.Equal(t, "prod-jaeger-span-06-10-2026", got[0])
		assert.Equal(t, dayFirst.WriteTarget(start), got[len(got)-1])
		assert.NotContains(t, strings.Join(got, ","), "*")
	})

	t.Run("collapses steps that stay on the same index", func(t *testing.T) {
		// Hourly steps with a daily layout repeat a name until the date changes.
		// The list is still one concrete name per day, newest first.
		stepped := NewPeriodicRotation("prod-jaeger-span", "2006-01-02", time.Hour)
		start := end.Add(-200 * 24 * time.Hour)
		got := stepped.ExactTargets(start, end)
		require.GreaterOrEqual(t, len(got), 2)
		assert.Equal(t, stepped.WriteTarget(end), got[0])
		assert.Equal(t, stepped.WriteTarget(end.Add(-24*time.Hour)), got[1])
		assert.Equal(t, stepped.WriteTarget(start), got[len(got)-1])
		assert.NotContains(t, strings.Join(got, ","), "*")
		assert.Less(t, len(got), 200*24)
	})

	t.Run("same index across a long range", func(t *testing.T) {
		yearly := NewPeriodicRotation("jaeger-span", "2006", 24*time.Hour)
		start := end.Add(-200 * 24 * time.Hour)
		assert.Equal(t, []string{"jaeger-span-2026"}, yearly.ExactTargets(start, end))
		assert.Contains(t, strings.Join(yearly.ReadTargets(start, end), ","), "*")
	})
}

func TestEnumerateTimeRangeStopsWhenTimeDoesNotMove(t *testing.T) {
	end := time.Date(2026, time.October, 10, 15, 0, 0, 0, time.UTC)
	start := end.Add(-24 * time.Hour)
	got := enumerateTimeRange("jaeger-sampling", "2006-01-02-15", start, end, 0, 10)
	require.NotEmpty(t, got)
	assert.Equal(t, "jaeger-sampling-2026-10-10-15", got[0])
	assert.Less(t, len(got), 10)
}

func TestPeriodicRotation_WriteOpType(t *testing.T) {
	r := NewPeriodicRotation(config.SpanIndexName, "2006-01-02", 24*time.Hour)
	assert.Equal(t, es.WriteOpIndex, r.WriteOpType())
}
