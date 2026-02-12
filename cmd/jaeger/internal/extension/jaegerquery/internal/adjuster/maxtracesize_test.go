// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adjuster

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TODO: add real test cases
func TestMaxTraceSizeAdjuster(t *testing.T) {
	testCases := []struct {
		description  string
		trace        []testSpan
		err          string
		maxAdjust    time.Duration
		maxTraceSize string // max trace size in format "512Mi"
	}{
		{
			description: "foo",
			trace: []testSpan{
				{id: [8]byte{1}, parent: [8]byte{0}, startTime: 10, duration: 100, host: "a", adjusted: 10},
			},
			maxAdjust: time.Second,
		},
	}

	for _, tt := range testCases {
		testCase := tt
		t.Run(testCase.description, func(t *testing.T) {
			trace := makeTrace(testCase.trace)
			adjuster := CorrectMaxSize(tt.maxTraceSize)
			adjuster.Adjust(trace)
			// some asserts
			assert.Equal(t, 1, 2)
		})
	}
}
