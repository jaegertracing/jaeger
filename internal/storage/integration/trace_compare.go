// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"bytes"
	"sort"
	"strings"
	"testing"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/pdatatest/ptracetest"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/pdatautil"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger-idl/model/v1"
)

// CompareSliceOfTraces compares two trace slices
func CompareSliceOfTraces(t *testing.T, expected []ptrace.Traces, actual []ptrace.Traces) {
	require.Len(t, expected, len(actual))
	sortSliceOfTraces(expected)
	sortSliceOfTraces(actual)
	for i, trace := range actual {
		if err := compareTraces(expected[i], trace); err != nil {
			t.Logf("Actual trace and expected traces are not equal at index %d: %v", i, err)
			t.Fail()
		}
	}
}

// CompareTraces compares two traces
func CompareTraces(t *testing.T, expected ptrace.Traces, actual ptrace.Traces) {
	if err := compareTraces(expected, actual); err != nil {
		t.Logf("Actual trace and expected traces are not equal: %v", err)
		t.Fail()
	}
}

func compareTraces(expected ptrace.Traces, actual ptrace.Traces) error {
	// CompareTracesOption also sort the resource, scope and ptrace.Spans but while sorting
	// the resource and scope spans, it only compares the resource and scope. For example
	// there could be two resource spans whose resource and scope are same but the spans
	// of those resource spans differ. The ptracetest.IgnoreResourceSpansOrder() and
	// ptracetest.IgnoreScopeSpansOrder() will not be able to sort these kinds of resource spans
	// properly. From OTEL, this is logical because they expect resource span to differ only on
	// basis of resource and attributes but in Jaeger, some backends like Elasticsearch assign one
	// resource span per ptrace.Span, so there could be some resource spans which only differ
	// on the underlying ptrace.Span. sortTrace sorts on the basis of the underlying spans too.
	sortTrace(expected)
	sortTrace(actual)
	options := []ptracetest.CompareTracesOption{
		ptracetest.IgnoreResourceSpansOrder(),
		ptracetest.IgnoreScopeSpansOrder(),
		ptracetest.IgnoreSpansOrder(),
	}
	return ptracetest.CompareTraces(expected, actual, options...)
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

func sortTrace(td ptrace.Traces) {
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

func compareResourceSpans(a, b ptrace.ResourceSpans) int {
	if lenComp := a.ScopeSpans().Len() - b.ScopeSpans().Len(); lenComp != 0 {
		return lenComp
	}
	if attrComp := compareAttributes(a.Resource().Attributes(), b.Resource().Attributes()); attrComp != 0 {
		return attrComp
	}
	for i := 0; i < a.ScopeSpans().Len(); i++ {
		aSpan := a.ScopeSpans().At(i)
		bSpan := b.ScopeSpans().At(i)
		if spanComp := compareScopeSpans(aSpan, bSpan); spanComp != 0 {
			return spanComp
		}
	}
	return 0
}

func compareScopeSpans(a, b ptrace.ScopeSpans) int {
	aScope := a.Scope()
	bScope := b.Scope()
	if nameComp := strings.Compare(aScope.Name(), bScope.Name()); nameComp != 0 {
		return nameComp
	}
	if versionComp := strings.Compare(aScope.Version(), bScope.Version()); versionComp != 0 {
		return versionComp
	}
	if lenComp := a.Spans().Len() - b.Spans().Len(); lenComp != 0 {
		return lenComp
	}
	if attrComp := compareAttributes(aScope.Attributes(), bScope.Attributes()); attrComp != 0 {
		return attrComp
	}
	for i := 0; i < a.Spans().Len(); i++ {
		aSpan := a.Spans().At(i)
		bSpan := b.Spans().At(i)
		if spanComp := compareSpans(aSpan, bSpan); spanComp != 0 {
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
	if spanIdComp := compareSpanIDs(a.SpanID(), b.SpanID()); spanIdComp != 0 {
		return spanIdComp
	}
	return compareTimestamps(a.StartTimestamp(), b.StartTimestamp())
}

func compareTraceIDs(a, b pcommon.TraceID) int {
	return bytes.Compare(a[:], b[:])
}

func compareSpanIDs(a, b pcommon.SpanID) int {
	return bytes.Compare(a[:], b[:])
}

func compareAttributes(a, b pcommon.Map) int {
	aAttrs := pdatautil.MapHash(a)
	bAttrs := pdatautil.MapHash(b)
	return bytes.Compare(aAttrs[:], bAttrs[:])
}

func compareTimestamps(a, b pcommon.Timestamp) int {
	if a == b {
		return 0
	}
	if a > b {
		return 1
	}
	return -1
}

func sortSliceOfTraces(traces []ptrace.Traces) {
	sort.Slice(traces, func(i, j int) bool {
		a := traces[i].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).TraceID()
		b := traces[j].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).TraceID()
		return compareTraceIDs(a, b) < 0
	})
}
