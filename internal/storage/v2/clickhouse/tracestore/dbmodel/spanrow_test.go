// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGetEventsFromRaw(t *testing.T) {
	tests := []struct {
		name     string
		raw      []map[string]any
		expected []Event
	}{
		{
			name: "valid raw data",
			raw: []map[string]any{
				{"name": "event1", "timestamp": time.Date(2023, 10, 1, 12, 0, 0, 0, time.UTC)},
				{"name": "event2", "timestamp": time.Date(2023, 10, 2, 12, 0, 0, 0, time.UTC)},
			},
			expected: []Event{
				{Name: "event1", Timestamp: time.Date(2023, 10, 1, 12, 0, 0, 0, time.UTC)},
				{Name: "event2", Timestamp: time.Date(2023, 10, 2, 12, 0, 0, 0, time.UTC)},
			},
		},
		{
			name:     "empty raw data",
			raw:      []map[string]any{},
			expected: []Event{},
		},
		{
			name: "invalid raw data",
			raw: []map[string]any{
				{"name": 123, "timestamp": "invalid"},
			},
			expected: []Event{
				{},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := getEventsFromRaw(test.raw)
			require.Equal(t, test.expected, result)
		})
	}
}

func TestGetLinksFromRaw(t *testing.T) {
	tests := []struct {
		name     string
		raw      []map[string]any
		expected []Link
	}{
		{
			name: "valid raw data",
			raw: []map[string]any{
				{"trace_id": "trace1", "span_id": "span1", "trace_state": "state1"},
				{"trace_id": "trace2", "span_id": "span2", "trace_state": "state2"},
			},
			expected: []Link{
				{TraceID: "trace1", SpanID: "span1", TraceState: "state1"},
				{TraceID: "trace2", SpanID: "span2", TraceState: "state2"},
			},
		},
		{
			name:     "empty raw data",
			raw:      []map[string]any{},
			expected: []Link{},
		},
		{
			name: "invalid raw data",
			raw: []map[string]any{
				{"trace_id": 123, "span_id": nil, "trace_state": true},
			},
			expected: []Link{
				{},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := getLinksFromRaw(test.raw)
			require.Equal(t, test.expected, result)
		})
	}
}
