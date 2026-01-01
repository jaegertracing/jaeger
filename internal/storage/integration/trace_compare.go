// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"bytes"
	"sort"
	"strings"
	"testing"

	"github.com/kr/pretty"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger-idl/model/v1"
)

// CompareSliceOfTraces compares two trace slices
func CompareSliceOfTraces(t *testing.T, expected []ptrace.Traces, actual []ptrace.Traces) {
	require.Len(t, actual, len(expected), "Unequal number of expected vs. actual traces")
	sortTraces(actual)
	sortTraces(expected)
	for i := range expected {
		checkSize(t, expected[i], actual[i])
	}
	for i, actualTrace := range actual {
		expectedTrace := expected[i]
		marshaller := ptrace.JSONMarshaler{}
		actualBytes, err := marshaller.MarshalTraces(actualTrace)
		require.NoError(t, err)
		expectedBytes, err := marshaller.MarshalTraces(expectedTrace)
		require.NoError(t, err)
		// The reason to compare the bytes not the traces is because there is no way to
		// sort attributes, marshaller handles this and keeps attributes consistent while
		// converting
		if diff := pretty.Diff(expectedBytes, actualBytes); len(diff) > 0 {
			for _, d := range diff {
				t.Logf("Expected and actual differ: %s\n", d)
			}
			t.Logf("Actual traces: %s", string(actualBytes))
			t.Logf("Expected traces: %s", string(expectedBytes))
			t.Fail()
		}
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

func checkSize(t *testing.T, expected ptrace.Traces, actual ptrace.Traces) {
	require.Equal(t, actual.SpanCount(), expected.SpanCount())
	require.Equal(t, expected.ResourceSpans().Len(), actual.ResourceSpans().Len())
	for i, expectedResourceSpan := range expected.ResourceSpans().All() {
		actualResourceSpan := actual.ResourceSpans().At(i)
		require.Equal(t, expectedResourceSpan.ScopeSpans().Len(), actualResourceSpan.ScopeSpans().Len())
		require.Equal(t, expectedResourceSpan.Resource().Attributes().Len(), actualResourceSpan.Resource().Attributes().Len())
		for j, expectedScopeSpan := range expectedResourceSpan.ScopeSpans().All() {
			actualScopeSpan := actualResourceSpan.ScopeSpans().At(j)
			require.Equal(t, expectedScopeSpan.Spans().Len(), actualScopeSpan.Spans().Len())
			require.Equal(t, expectedScopeSpan.Scope().Attributes().Len(), actualScopeSpan.Scope().Attributes().Len())
			for k, expectedSpan := range expectedScopeSpan.Spans().All() {
				actualSpan := actualScopeSpan.Spans().At(k)
				require.Equal(t, expectedSpan.Attributes().Len(), actualSpan.Attributes().Len())
				require.Equal(t, expectedSpan.Events().Len(), actualSpan.Events().Len())
				require.Equal(t, expectedSpan.Links().Len(), actualSpan.Links().Len())
			}
		}
	}
}

func sortTraces(traces []ptrace.Traces) {
	for _, trace := range traces {
		normalizeTrace(trace)
	}
	sort.Slice(traces, func(i, j int) bool {
		return compareTraces(traces[i], traces[j]) < 0
	})
}

func normalizeTrace(td ptrace.Traces) {
	for i := 0; i < td.ResourceSpans().Len(); i++ {
		resourceSpan := td.ResourceSpans().At(i)
		for j := 0; j < resourceSpan.ScopeSpans().Len(); j++ {
			scopeSpan := resourceSpan.ScopeSpans().At(j)
			scopeSpan.Spans().Sort(func(a, b ptrace.Span) bool {
				return compareSpans(a, b) < 0
			})
		}
		resourceSpan.ScopeSpans().Sort(func(a, b ptrace.ScopeSpans) bool {
			return compareScopeSpans(a, b) < 0
		})
	}
	td.ResourceSpans().Sort(func(a, b ptrace.ResourceSpans) bool {
		return compareResourceSpans(a, b) < 0
	})
}

func compareTraces(a, b ptrace.Traces) int {
	spanDiff := a.SpanCount() - b.SpanCount()
	if spanDiff != 0 {
		return spanDiff
	}
	resourceSpansLenComp := a.ResourceSpans().Len() - b.ResourceSpans().Len()
	if resourceSpansLenComp != 0 {
		return resourceSpansLenComp
	}
	for i := 0; i < a.ResourceSpans().Len(); i++ {
		aResourceSpan := a.ResourceSpans().At(i)
		bResourceSpan := b.ResourceSpans().At(i)
		resourceSpanComp := compareResourceSpans(aResourceSpan, bResourceSpan)
		if resourceSpanComp != 0 {
			return resourceSpanComp
		}
	}
	return 0
}

func compareResourceSpans(a, b ptrace.ResourceSpans) int {
	lenComp := a.ScopeSpans().Len() - b.ScopeSpans().Len()
	if lenComp != 0 {
		return lenComp
	}
	for i := 0; i < a.ScopeSpans().Len(); i++ {
		aSpan := a.ScopeSpans().At(i)
		bSpan := b.ScopeSpans().At(i)
		spanComp := compareScopeSpans(aSpan, bSpan)
		if spanComp != 0 {
			return spanComp
		}
	}
	return 0
}

func compareScopeSpans(a, b ptrace.ScopeSpans) int {
	aScope := a.Scope()
	bScope := b.Scope()
	nameComp := strings.Compare(aScope.Name(), bScope.Name())
	if nameComp != 0 {
		return nameComp
	}
	versionComp := strings.Compare(aScope.Version(), bScope.Version())
	if versionComp != 0 {
		return versionComp
	}
	lenComp := a.Spans().Len() - b.Spans().Len()
	if lenComp != 0 {
		return lenComp
	}
	for i := 0; i < a.Spans().Len(); i++ {
		aSpan := a.Spans().At(i)
		bSpan := b.Spans().At(i)
		spanComp := compareSpans(aSpan, bSpan)
		if spanComp != 0 {
			return spanComp
		}
	}
	return 0
}

func compareSpans(a, b ptrace.Span) int {
	traceIdComp := compareTraceIDs(a.TraceID(), b.TraceID())
	if traceIdComp != 0 {
		return traceIdComp
	}
	return compareSpanIDs(a.SpanID(), b.SpanID())
}

func compareTraceIDs(a, b pcommon.TraceID) int {
	return bytes.Compare(a[:], b[:])
}

func compareSpanIDs(a, b pcommon.SpanID) int {
	return bytes.Compare(a[:], b[:])
}
