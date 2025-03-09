// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCalculateDeletionCutoff(t *testing.T) {
	time20250309163053 := time.Date(2025, time.March, 9, 16, 30, 53, 0, time.UTC)
	tests := []struct {
		name                 string
		currTime             time.Time
		numOfDays            int
		relativeIndexEnabled bool
		expectedCutoff       time.Time
	}{
		{
			name:                 "get today's midnight",
			currTime:             time20250309163053,
			numOfDays:            1,
			relativeIndexEnabled: false,
			expectedCutoff:       time.Date(2025, time.March, 9, 0, 0, 0, 0, time.UTC),
		},
		{
			name:                 "get exactly 24 hours before execution time",
			currTime:             time20250309163053,
			numOfDays:            1,
			relativeIndexEnabled: true,
			expectedCutoff:       time.Date(2025, time.March, 8, 16, 30, 53, 0, time.UTC),
		},
		{
			name:                 "get the current time if numOfDays is 0 and relativeIndexEnabled is true",
			currTime:             time20250309163053,
			numOfDays:            0,
			relativeIndexEnabled: true,
			expectedCutoff:       time20250309163053,
		},
		{
			name:                 "get tomorrow's midnight if numOfDays is 0  relativeIndexEnabled is False",
			currTime:             time20250309163053,
			numOfDays:            0,
			relativeIndexEnabled: false,
			expectedCutoff:       time.Date(2025, time.March, 10, 0, 0, 0, 0, time.UTC),
		},
		{
			name:                 "get (numOfDays-1)'s midnight",
			currTime:             time20250309163053,
			numOfDays:            3,
			relativeIndexEnabled: false,
			expectedCutoff:       time.Date(2025, time.March, 7, 0, 0, 0, 0, time.UTC),
		},
		{
			name:                 "get exactly (24*numOfDays) hours before execution time",
			currTime:             time20250309163053,
			numOfDays:            3,
			relativeIndexEnabled: true,
			expectedCutoff:       time.Date(2025, time.March, 6, 16, 30, 53, 0, time.UTC),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cutoffTime := CalculateDeletionCutoff(time20250309163053, test.numOfDays, test.relativeIndexEnabled)
			assert.Equal(t, test.expectedCutoff, cutoffTime)
		})
	}
}
