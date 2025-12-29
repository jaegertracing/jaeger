// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/kr/pretty"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger-idl/model/v1"
)

// CompareSliceOfTraces compares two trace slices
func compareSliceOfTraces(expected []*model.Trace, actual []*model.Trace) error {
	if len(actual) != len(expected) {
		return fmt.Errorf("unequal number of expected vs actual traces: expected=%d actual=%d",
			len(expected), len(actual))
	}

	model.SortTraces(expected)
	model.SortTraces(actual)

	for i := range expected {
		if err := checkSizeLogic(expected[i], actual[i]); err != nil {
			return err
		}
	}

	if diff := pretty.Diff(expected, actual); len(diff) > 0 {
		outActual, err := json.Marshal(actual)
		if err != nil {
			return err
		}
		outExpected, err := json.Marshal(expected)
		if err != nil {
			return err
		}
		return fmt.Errorf(
			"expected and actual traces differ:\n%s\nactual=%s\nexpected=%s",
			diff, outActual, outExpected,
		)
	}

	return nil
}


// CompareTraces compares two traces
func compareTraces(expected *model.Trace, actual *model.Trace) error {
	if expected.Spans == nil {
		if actual.Spans != nil {
			return fmt.Errorf("expected nil spans, got non-nil")
		}
		return nil
	}

	if actual == nil || actual.Spans == nil {
		return fmt.Errorf("actual trace or spans is nil")
	}

	// some storage implementations may retry writing spans and end up with duplicates
	countBefore := len(actual.Spans)
	dedupeSpans(actual)
	_ = countBefore

	model.SortTrace(expected)
	model.SortTrace(actual)

	return checkSizeLogic(expected, actual)
}

func checkSizeLogic(expected *model.Trace, actual *model.Trace) error {
	if len(actual.Spans) != len(expected.Spans) {
		return fmt.Errorf("span count mismatch: expected=%d actual=%d",
			len(expected.Spans), len(actual.Spans))
	}

	for i := range expected.Spans {
		expectedSpan := expected.Spans[i]
		actualSpan := actual.Spans[i]

		if len(actualSpan.Tags) != len(expectedSpan.Tags) {
			return fmt.Errorf("tag count mismatch at span %d", i)
		}
		if len(actualSpan.Logs) != len(expectedSpan.Logs) {
			return fmt.Errorf("log count mismatch at span %d", i)
		}
		if expectedSpan.Process != nil && actualSpan.Process != nil {
			if len(actualSpan.Process.Tags) != len(expectedSpan.Process.Tags) {
				return fmt.Errorf("process tag count mismatch at span %d", i)
			}
		}
	}

	return nil
}

func CompareSliceOfTraces(t *testing.T, expected []*model.Trace, actual []*model.Trace) {
	require.NoError(t, compareSliceOfTraces(expected, actual))
}

func CompareTraces(t *testing.T, expected *model.Trace, actual *model.Trace) {
	require.NoError(t, compareTraces(expected, actual))
}

func checkSize(t *testing.T, expected *model.Trace, actual *model.Trace) {
	require.NoError(t, checkSizeLogic(expected, actual))
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
