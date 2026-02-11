// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"bytes"
	"sort"
	"strings"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/pdatatest/ptracetest"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/pdatautil"
	"github.com/pmezard/go-difflib/difflib"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/jptrace"
)

// CompareTraceSlices compares two trace slices
func CompareTraceSlices(t *testing.T, expected []ptrace.Traces, actual []ptrace.Traces) {
	require.Len(t, actual, len(expected))
	sortTracesByTraceID(expected)
	sortTracesByTraceID(actual)
	for i, trace := range actual {
		makeTraceReadyForComparison(trace)
		makeTraceReadyForComparison(expected[i])
		if err := ptracetest.CompareTraces(expected[i], trace); err != nil {
			t.Logf("Actual trace and expected traces are not equal at index %d: %v", i, err)
			t.Log(getDiff(t, expected[i], trace))
			t.Fail()
		}
	}
}

// CompareTraces compares two traces
func CompareTraces(t *testing.T, expected ptrace.Traces, actual ptrace.Traces) {
	makeTraceReadyForComparison(expected)
	makeTraceReadyForComparison(actual)
	if err := ptracetest.CompareTraces(expected, actual); err != nil {
		t.Logf("Actual trace and expected traces are not equal: %v", err)
		t.Log(getDiff(t, expected, actual))
		t.Fail()
	}
}

func makeTraceReadyForComparison(td ptrace.Traces) {
	normalizeTrace(td)
	sortTrace(td)
	dedupeSpans(td)
}

// spans may contain spans with the same SpanID. Remove duplicates
// and keep the first one. Use a map to keep track of the spans we've seen.
func dedupeSpans(trace ptrace.Traces) {
	seen := make(map[pcommon.SpanID]bool)
	newSpans := ptrace.NewResourceSpansSlice()
	for _, resourceSpan := range trace.ResourceSpans().All() {
		newResourceSpan := newSpans.AppendEmpty()
		resourceSpan.Resource().CopyTo(newResourceSpan.Resource())
		for _, scopeSpan := range resourceSpan.ScopeSpans().All() {
			newScopeSpan := newResourceSpan.ScopeSpans().AppendEmpty()
			scopeSpan.Scope().CopyTo(newScopeSpan.Scope())
			for _, span := range scopeSpan.Spans().All() {
				if !seen[span.SpanID()] {
					seen[span.SpanID()] = true
					span.CopyTo(newScopeSpan.Spans().AppendEmpty())
				}
			}
		}
	}
	newSpans.CopyTo(trace.ResourceSpans())
}

// sortTrace sorts the spans of a trace on the basis of resource,
// scope, trace id, span id and start time of span. The limitation
// of using sorting provided by ptracetest is: It can't sort
// those resource and scope spans which have same pcommon.Resource
// and pcommon.InstrumentationScope but different ptrace.Span
func sortTrace(td ptrace.Traces) {
	for _, resourceSpan := range td.ResourceSpans().All() {
		sortAttributes(resourceSpan.Resource().Attributes())
		for _, scopeSpan := range resourceSpan.ScopeSpans().All() {
			sortAttributes(scopeSpan.Scope().Attributes())
			scopeSpan.Spans().Sort(func(a, b ptrace.Span) bool {
				return compareSpans(a, b) < 0
			})
			for _, span := range scopeSpan.Spans().All() {
				sortAttributes(span.Attributes())
				for _, events := range span.Events().All() {
					sortAttributes(events.Attributes())
				}
				for _, link := range span.Links().All() {
					sortAttributes(link.Attributes())
				}
			}
		}
		resourceSpan.ScopeSpans().Sort(func(a, b ptrace.ScopeSpans) bool {
			return compareScopeSpans(a, b) < 0
		})
	}
	td.ResourceSpans().Sort(compareResourceSpans)
}

func compareResourceSpans(a, b ptrace.ResourceSpans) bool {
	if lenComp := a.ScopeSpans().Len() - b.ScopeSpans().Len(); lenComp != 0 {
		return lenComp < 0
	}
	if attrComp := compareAttributes(a.Resource().Attributes(), b.Resource().Attributes()); attrComp != 0 {
		return attrComp < 0
	}
	for i := 0; i < a.ScopeSpans().Len(); i++ {
		aSpan := a.ScopeSpans().At(i)
		bSpan := b.ScopeSpans().At(i)
		if scopeComp := compareScopeSpans(aSpan, bSpan); scopeComp != 0 {
			return scopeComp < 0
		}
	}
	return false
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
	if attrComp := compareAttributes(aScope.Attributes(), bScope.Attributes()); attrComp != 0 {
		return attrComp
	}
	if lenComp := a.Spans().Len() - b.Spans().Len(); lenComp != 0 {
		return lenComp
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

// compareSpans compares two spans on the basis of trace id, span id and
// start time. It should not be used to directly compare spans because it
// leaves some top level fields like status, kind and attributes. In integration
// tests it is used for sorting spans only, not for span comparison. For span
// comparison, ptracetest.CompareTraces is used.
func compareSpans(a, b ptrace.Span) int {
	if traceIdComp := compareTraceIDs(a.TraceID(), b.TraceID()); traceIdComp != 0 {
		return traceIdComp
	}
	if spanIdComp := compareSpanIDs(a.SpanID(), b.SpanID()); spanIdComp != 0 {
		return spanIdComp
	}
	if timeStampComp := compareTimestamps(a.StartTimestamp(), b.StartTimestamp()); timeStampComp != 0 {
		return timeStampComp
	}
	return 0
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

func sortTracesByTraceID(traces []ptrace.Traces) {
	sort.Slice(traces, func(i, j int) bool {
		a := traces[i].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).TraceID()
		b := traces[j].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).TraceID()
		return compareTraceIDs(a, b) < 0
	})
}

func sortAttributes(attr pcommon.Map) {
	keys := make([]string, 0, attr.Len())
	keyVal := make(map[string]pcommon.Value, attr.Len())
	attr.Range(func(k string, v pcommon.Value) bool {
		keys = append(keys, k)
		keyVal[k] = v
		return true
	})
	sort.Strings(keys)
	newMap := pcommon.NewMap()
	for _, k := range keys {
		val, _ := newMap.GetOrPutEmpty(k)
		keyVal[k].CopyTo(val)
	}
	newMap.CopyTo(attr)
}

// normalizeTrace assigns one resource span to one span. The fixtures
// can have multiple spans under one resource/scope span but some
// backends normalize traces for reducing complexity
// (elasticsearch is one of the examples). The writer can
// write traces without any normalization but reader will always
// return normalized traces. Therefore, for comparing two spans
// we need to normalize the expected fixtures.
func normalizeTrace(td ptrace.Traces) {
	normalizedResourceSpans := ptrace.NewResourceSpansSlice()
	normalizedResourceSpans.EnsureCapacity(td.SpanCount())
	for pos, span := range jptrace.SpanIter(td) {
		resource := pos.Resource.Resource()
		scope := pos.Scope.Scope()
		normalizedResourceSpan := normalizedResourceSpans.AppendEmpty()
		resource.CopyTo(normalizedResourceSpan.Resource())
		normalizedScopeSpan := normalizedResourceSpan.ScopeSpans().AppendEmpty()
		scope.CopyTo(normalizedScopeSpan.Scope())
		normalizedSpan := normalizedScopeSpan.Spans().AppendEmpty()
		span.CopyTo(normalizedSpan)
	}
	normalizedResourceSpans.CopyTo(td.ResourceSpans())
}

func getDiff(t *testing.T, expected ptrace.Traces, actual ptrace.Traces) string {
	spewConfig := spew.ConfigState{
		Indent:                  " ",
		DisablePointerAddresses: true,
		DisableCapacities:       true,
		SortKeys:                true,
		DisableMethods:          true,
		MaxDepth:                10,
	}
	e := spewConfig.Sdump(expected)
	a := spewConfig.Sdump(actual)
	diff, err := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A:        difflib.SplitLines(e),
		B:        difflib.SplitLines(a),
		FromFile: "Expected",
		FromDate: "",
		ToFile:   "Actual",
		ToDate:   "",
		Context:  1,
	})
	require.NoError(t, err)
	return "\n\nDiff:\n" + diff
}
