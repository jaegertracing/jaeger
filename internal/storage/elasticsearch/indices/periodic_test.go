// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package indices

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestPeriodicRotation_WriteTarget(t *testing.T) {
	r := NewPeriodicRotation("jaeger-span-", "2006-01-02", 24*time.Hour)
	date := time.Date(1995, time.April, 21, 22, 8, 41, 0, time.UTC)
	assert.Equal(t, "jaeger-span-1995-04-21", r.WriteTarget(date))
}

func TestPeriodicRotation_WriteTarget_Hourly(t *testing.T) {
	r := NewPeriodicRotation("jaeger-span-", "2006-01-02-15", time.Hour)
	date := time.Date(2019, time.October, 10, 5, 0, 0, 0, time.UTC)
	assert.Equal(t, "jaeger-span-2019-10-10-05", r.WriteTarget(date))
}

func TestPeriodicRotation_ReadTargets(t *testing.T) {
	r := NewPeriodicRotation("jaeger-span-", "2006-01-02", 24*time.Hour)
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

func TestPeriodicRotation_WriteOpType(t *testing.T) {
	r := NewPeriodicRotation("jaeger-span-", "2006-01-02", 24*time.Hour)
	assert.Equal(t, WriteOpIndex, r.WriteOpType())
}
