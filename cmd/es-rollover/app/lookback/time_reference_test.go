// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package lookback

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetTimeReference(t *testing.T) {
	now := time.Date(2021, time.October, 10, 10, 10, 10, 10, time.UTC)

	tests := []struct {
		name         string
		unit         string
		unitCount    int
		expectedTime time.Time
	}{
		{
			name:         "seconds unit",
			unit:         "seconds",
			unitCount:    30,
			expectedTime: time.Date(2021, time.October, 10, 10, 9, 40, 0, time.UTC),
		},
		{
			name:         "minutes unit",
			unit:         "minutes",
			unitCount:    30,
			expectedTime: time.Date(2021, time.October, 10, 9, 40, 0, 0, time.UTC),
		},
		{
			name:         "hours unit",
			unit:         "hours",
			unitCount:    2,
			expectedTime: time.Date(2021, time.October, 10, 8, 0, 0, 0, time.UTC),
		},
		{
			name:         "days unit",
			unit:         "days",
			unitCount:    2,
			expectedTime: time.Date(2021, 10, 9, 0, 0, 0, 0, time.UTC),
		},
		{
			name:         "weeks unit",
			unit:         "weeks",
			unitCount:    2,
			expectedTime: time.Date(2021, time.September, 27, 0, 0, 0, 0, time.UTC),
		},
		{
			name:         "months unit",
			unit:         "months",
			unitCount:    2,
			expectedTime: time.Date(2021, time.August, 10, 0, 0, 0, 0, time.UTC),
		},
		{
			name:         "years unit",
			unit:         "years",
			unitCount:    2,
			expectedTime: time.Date(2019, time.October, 10, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ref, err := getTimeReference(now, test.unit, test.unitCount)
			require.NoError(t, err)
			assert.Equal(t, test.expectedTime, ref)
		})
	}
}

func TestGetTimeReference_UnknownUnit(t *testing.T) {
	now := time.Date(2021, time.October, 10, 10, 10, 10, 10, time.UTC)
	unitCount := 30

	for _, unknownUnit := range []string{"unknown-unit", "milliseconds", "day", "dayss"} {
		t.Run(unknownUnit, func(t *testing.T) {
			ref, err := getTimeReference(now, unknownUnit, unitCount)
			require.Error(t, err)
			assert.Contains(t, err.Error(), unknownUnit)
			assert.Equal(t, time.Time{}, ref)
		})
	}
}
