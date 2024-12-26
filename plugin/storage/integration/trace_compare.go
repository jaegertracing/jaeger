// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"encoding/json"
	"testing"

	"github.com/kr/pretty"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/model"
)

// CompareSliceOfTraces compares two trace slices
func CompareSliceOfTraces(t *testing.T, expected []*model.Trace, actual []*model.Trace) {
	require.Equal(t, len(expected), len(actual), "Unequal number of expected vs. actual traces")
	model.SortTraces(expected)
	model.SortTraces(actual)
	for i := range expected {
		checkSize(t, expected[i], actual[i])
	}
	if diff := pretty.Diff(expected, actual); len(diff) > 0 {
		for _, d := range diff {
			t.Logf("Expected and actual differ: %s\n", d)
		}
		out, err := json.Marshal(actual)
		out2, err2 := json.Marshal(expected)
		require.NoError(t, err)
		require.NoError(t, err2)
		t.Logf("Actual traces: %s", string(out))
		t.Logf("Expected traces: %s", string(out2))
		t.Fail()
	}
}

// trace.Spans may contain spans with the same SpanID. Remove duplicates
// and keep the first one. Use a map to keep track of the spans we've seen.
func dedupeSpans(trace *model.Trace) {
	seen := make(map[model.SpanID]bool)
	var newSpans []*model.Span
	for _, span := range trace.Spans {
		if !seen[span.SpanID] {
			seen[span.SpanID] = true
			newSpans = append(newSpans, span)
		}
	}
	trace.Spans = newSpans
}

// CompareTraces compares two traces
func CompareTraces(t *testing.T, expected *model.Trace, actual *model.Trace) {
	if expected.Spans == nil {
		require.Nil(t, actual.Spans)
		return
	}
	require.NotNil(t, actual)
	require.NotNil(t, actual.Spans)

	// some storage implementation may retry writing of spans and end up with duplicates.
	countBefore := len(actual.Spans)
	dedupeSpans(actual)
	if countAfter := len(actual.Spans); countAfter != countBefore {
		t.Logf("Removed spans with duplicate span IDs; before=%d, after=%d", countBefore, countAfter)
	}

	model.SortTrace(expected)
	model.SortTrace(actual)
	checkSize(t, expected, actual)

	if diff := pretty.Diff(expected, actual); len(diff) > 0 {
		for _, d := range diff {
			t.Logf("Expected and actual differ: %v\n", d)
		}
		out, err := json.Marshal(actual)
		require.NoError(t, err)
		t.Logf("Actual trace: %s", string(out))
		t.Fail()
	}
}

func checkSize(t *testing.T, expected *model.Trace, actual *model.Trace) {
	require.Equal(t, len(expected.Spans), len(actual.Spans))
	for i := range expected.Spans {
		expectedSpan := expected.Spans[i]
		actualSpan := actual.Spans[i]
		require.Equal(t, len(expectedSpan.Tags), len(actualSpan.Tags))
		require.Equal(t, len(expectedSpan.Logs), len(actualSpan.Logs))
		if expectedSpan.Process != nil && actualSpan.Process != nil {
			require.Equal(t, len(expectedSpan.Process.Tags), len(actualSpan.Process.Tags))
		}
	}
}
