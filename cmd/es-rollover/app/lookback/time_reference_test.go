// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package lookback

import (
	"fmt"
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

func TestGetTimeReference_DefaultCase(t *testing.T) {
	now := time.Date(2021, time.October, 10, 10, 10, 10, 10, time.UTC)

	unknownUnit := "unknown-unit"
	unitCount := 30

	ref, err := getTimeReference(now, unknownUnit, unitCount)
	require.NoError(t, err)

	expectedTime := time.Date(2021, time.October, 10, 10, 9, 40, 0, time.UTC)
	assert.Equal(t, expectedTime, ref)

	anotherUnknownUnit := "milliseconds"
	ref2, err := getTimeReference(now, anotherUnknownUnit, unitCount)
	require.NoError(t, err)

	assert.Equal(t, expectedTime, ref2)
}

func TestGetTimeReference_NonPositiveUnitCount(t *testing.T) {
	now := time.Date(2021, time.October, 10, 10, 10, 10, 10, time.UTC)
	for _, unitCount := range []int{0, -1, -100} {
		t.Run(fmt.Sprintf("unitCount=%d", unitCount), func(t *testing.T) {
			ref, err := getTimeReference(now, "days", unitCount)
			require.Error(t, err)
			assert.Equal(t, fmt.Sprintf("unit-count must be greater than 0, got %d", unitCount), err.Error())
			assert.Equal(t, time.Time{}, ref)
		})
	}
}
