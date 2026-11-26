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
		name           string
		numOfDays      int
		expectedCutoff time.Time
	}{
		{
			name:           "get exactly 24 hours before execution time",
			numOfDays:      1,
			expectedCutoff: time.Date(2025, time.March, 8, 16, 30, 53, 0, time.UTC),
		},
		{
			name:           "get the current time if numOfDays is 0",
			numOfDays:      0,
			expectedCutoff: time20250309163053,
		},
		{
			name:           "get exactly (24*numOfDays) hours before execution time",
			numOfDays:      3,
			expectedCutoff: time.Date(2025, time.March, 6, 16, 30, 53, 0, time.UTC),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cutoffTime := CalculateDeletionCutoff(time20250309163053, test.numOfDays)
			assert.Equal(t, test.expectedCutoff, cutoffTime)
		})
	}
}
