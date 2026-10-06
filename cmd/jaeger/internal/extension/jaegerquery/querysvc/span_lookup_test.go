// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package querysvc

import (
	"context"
	"iter"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	"github.com/jaegertracing/jaeger/components/extension/jaegerquery/queryinterceptor"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	tracestoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore/mocks"
)

var (
	lookupTraceID = pcommon.TraceID{1, 2, 3}
	otherTraceID  = pcommon.TraceID{9, 9, 9}
	lookupSpanA   = pcommon.SpanID{0, 0, 0, 0, 0, 0, 0, 1}
	lookupSpanB   = pcommon.SpanID{0, 0, 0, 0, 0, 0, 0, 2}
)

// identityFilterCaps is a reader that declares span search and the operators the identity filter
// uses, so the fast path runs instead of refusing.
var identityFilterCaps = tracestore.SearchCapabilities{
	SpanSearch: true,
	Filter: &tracestore.FilterCapabilities{
		Levels:    []expression.Level{expression.LevelSpan},
		Operators: []expression.Operator{expression.OpAnd, expression.OpEq, expression.OpIn},
	},
}

// lookupTrace builds one trace holding a span for each given span ID.
func lookupTrace(traceID pcommon.TraceID, spanIDs ...pcommon.SpanID) ptrace.Traces {
	traces := ptrace.NewTraces()
	spans := traces.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans()
	for _, id := range spanIDs {
		span := spans.AppendEmpty()
		span.SetTraceID(traceID)
		span.SetSpanID(id)
	}
	return traces
}

func spanPageSeq(pages ...tracestore.PageChunk[ptrace.Traces]) iter.Seq2[tracestore.PageChunk[ptrace.Traces], error] {
	return func(yield func(tracestore.PageChunk[ptrace.Traces], error) bool) {
		for _, page := range pages {
			if !yield(page, nil) {
				return
			}
		}
	}
}

func spanErrSeq(err error) iter.Seq2[tracestore.PageChunk[ptrace.Traces], error] {
	return func(yield func(tracestore.PageChunk[ptrace.Traces], error) bool) {
		yield(tracestore.PageChunk[ptrace.Traces]{}, err)
	}
}

func wholeTraceSeq(traces ...ptrace.Traces) iter.Seq2[[]ptrace.Traces, error] {
	return func(yield func([]ptrace.Traces, error) bool) {
		yield(traces, nil)
	}
}

func lookupParams(spanIDs ...pcommon.SpanID) SpanLookupParams {
	return SpanLookupParams{TraceID: lookupTraceID, SpanIDs: spanIDs}
}

// TestLookupSpans_FindSpansFastPath pins RFC 0016 M7: a backend that declares span search answers
// from FindSpans, and GetTraces is never read when FindSpans already answered.
func TestLookupSpans_FindSpansFastPath(t *testing.T) {
	enableStructuredFilters(t)
	tqs := initializeBareTestQueryService()
	tqs.traceReader.On("SearchCapabilities", mock.Anything).Return(identityFilterCaps, nil)
	tqs.traceReader.On("FindSpans", mock.Anything, mock.Anything).
		Return(spanPageSeq(tracestore.PageChunk[ptrace.Traces]{Results: lookupTrace(lookupTraceID, lookupSpanA, lookupSpanB)})).
		Once()

	result, err := tqs.queryService.LookupSpans(context.Background(), lookupParams(lookupSpanA, lookupSpanB))

	require.NoError(t, err)
	assert.True(t, result.TraceFound)
	assert.Len(t, result.Spans, 2)
	tqs.traceReader.AssertNotCalled(t, "GetTraces", mock.Anything, mock.Anything)
}

// TestLookupSpans_FindSpansNoMatches pins that an empty fast-path result reports the trace as
// found with no spans. The fast path cannot tell "wrong span IDs" from "no such trace" without the
// whole-trace read it exists to avoid, so the handler reports both as spans not found.
func TestLookupSpans_FindSpansNoMatches(t *testing.T) {
	enableStructuredFilters(t)
	tqs := initializeBareTestQueryService()
	tqs.traceReader.On("SearchCapabilities", mock.Anything).Return(identityFilterCaps, nil)
	tqs.traceReader.On("FindSpans", mock.Anything, mock.Anything).
		Return(spanPageSeq(tracestore.PageChunk[ptrace.Traces]{Results: ptrace.NewTraces()})).Once()

	result, err := tqs.queryService.LookupSpans(context.Background(), lookupParams(lookupSpanA))

	require.NoError(t, err)
	assert.True(t, result.TraceFound)
	assert.Empty(t, result.Spans)
}

// TestLookupSpans_FindSpansStorageErrorIsNotFallenBack pins that a genuine storage error is returned
// rather than answered by GetTraces: only the sentinels that mean "this backend cannot take the fast
// path at all" trigger the fallback.
func TestLookupSpans_FindSpansStorageErrorIsNotFallenBack(t *testing.T) {
	enableStructuredFilters(t)
	tqs := initializeBareTestQueryService()
	tqs.traceReader.On("SearchCapabilities", mock.Anything).Return(identityFilterCaps, nil)
	tqs.traceReader.On("FindSpans", mock.Anything, mock.Anything).Return(spanErrSeq(assert.AnError)).Once()

	_, err := tqs.queryService.LookupSpans(context.Background(), lookupParams(lookupSpanA))

	require.ErrorIs(t, err, assert.AnError)
	tqs.traceReader.AssertNotCalled(t, "GetTraces", mock.Anything, mock.Anything)
}

// TestLookupSpans_FallsBackToGetTraces pins the three triggers that send the lookup to the
// whole-trace read: span search not declared, the structured-filter gate off, and a reader that
// refuses the identity filter's fields or operators (a distinct sentinel that still unwraps to
// errors.ErrUnsupported).
func TestLookupSpans_FallsBackToGetTraces(t *testing.T) {
	tests := map[string]struct {
		gateEnabled bool
		caps        tracestore.SearchCapabilities
		findErr     error
	}{
		"span search not declared": {
			gateEnabled: true,
			caps:        tracestore.SearchCapabilities{},
		},
		"filter gate disabled": {
			gateEnabled: false,
			caps:        identityFilterCaps,
		},
		"identity filter unsupported": {
			gateEnabled: true,
			caps:        identityFilterCaps,
			findErr:     tracestore.ErrFilterUnsupported,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			setStructuredFilters(t, tt.gateEnabled)
			tqs := initializeBareTestQueryService()
			tqs.traceReader.On("SearchCapabilities", mock.Anything).Return(tt.caps, nil)
			if tt.findErr != nil {
				tqs.traceReader.On("FindSpans", mock.Anything, mock.Anything).Return(spanErrSeq(tt.findErr)).Once()
			}
			tqs.traceReader.On("GetTraces", mock.Anything, mock.Anything).
				Return(wholeTraceSeq(lookupTrace(lookupTraceID, lookupSpanA))).Once()

			result, err := tqs.queryService.LookupSpans(context.Background(), lookupParams(lookupSpanA))

			require.NoError(t, err)
			assert.True(t, result.TraceFound)
			assert.Len(t, result.Spans, 1)
		})
	}
}

// TestLookupSpans_IdentityFilterRefusalNotRetriedPastInterceptor pins that a refusal of the identity
// filter is not retried once an interceptor has run: the interceptor may have narrowed the filter for
// a policy, and a retry would read the trace without that narrowing.
func TestLookupSpans_IdentityFilterRefusalNotRetriedPastInterceptor(t *testing.T) {
	enableStructuredFilters(t)
	reader := &tracestoremocks.Reader{}
	reader.On("SearchCapabilities", mock.Anything).Return(identityFilterCaps, nil)
	reader.On("FindSpans", mock.Anything, mock.Anything).Return(spanErrSeq(tracestore.ErrFilterUnsupported)).Once()
	qs := NewQueryService(reader, nil, QueryServiceOptions{
		Interceptors: []queryinterceptor.Interceptor{fakeInterceptor{}},
	})

	_, err := qs.LookupSpans(context.Background(), lookupParams(lookupSpanA))

	require.ErrorIs(t, err, tracestore.ErrFilterUnsupported)
	reader.AssertNotCalled(t, "GetTraces", mock.Anything, mock.Anything)
}

// TestLookupSpans_IgnoresSameSpanIDInAnotherTrace pins that span IDs are unique only within a
// trace: a span that shares the requested ID but belongs to another trace is not returned.
func TestLookupSpans_IgnoresSameSpanIDInAnotherTrace(t *testing.T) {
	enableStructuredFilters(t)
	tqs := initializeBareTestQueryService()
	tqs.traceReader.On("SearchCapabilities", mock.Anything).Return(identityFilterCaps, nil)
	tqs.traceReader.On("FindSpans", mock.Anything, mock.Anything).
		Return(spanPageSeq(tracestore.PageChunk[ptrace.Traces]{Results: lookupTrace(otherTraceID, lookupSpanA)})).Once()

	result, err := tqs.queryService.LookupSpans(context.Background(), lookupParams(lookupSpanA))

	require.NoError(t, err)
	assert.True(t, result.TraceFound)
	assert.Empty(t, result.Spans)
}

// TestLookupSpans_ReadsEveryPage pins that every page of a long result is read until the token runs
// out, and that the next page is requested with the token the previous one returned.
func TestLookupSpans_ReadsEveryPage(t *testing.T) {
	enablePagination(t)
	enableStructuredFilters(t)
	tqs := initializeBareTestQueryService()
	paginated := identityFilterCaps
	paginated.Paginated = true
	tqs.traceReader.On("SearchCapabilities", mock.Anything).Return(paginated, nil)
	tqs.traceReader.On("FindSpans", mock.Anything, mock.MatchedBy(func(q tracestore.SpanQueryParams) bool {
		return q.Pagination.PageToken == ""
	})).Return(spanPageSeq(tracestore.PageChunk[ptrace.Traces]{
		Results:       lookupTrace(lookupTraceID, lookupSpanA),
		NextPageToken: "next",
	})).Once()
	tqs.traceReader.On("FindSpans", mock.Anything, mock.MatchedBy(func(q tracestore.SpanQueryParams) bool {
		return q.Pagination.PageToken == "next"
	})).Return(spanPageSeq(tracestore.PageChunk[ptrace.Traces]{
		Results: lookupTrace(lookupTraceID, lookupSpanB),
	})).Once()

	result, err := tqs.queryService.LookupSpans(context.Background(), lookupParams(lookupSpanA, lookupSpanB))

	require.NoError(t, err)
	assert.Len(t, result.Spans, 2)
	tqs.traceReader.AssertExpectations(t)
}

// TestLookupSpans_ArchiveDisablesFastPath pins that with archive storage configured the span search is
// not used, since it cannot see a trace that exists only in the archive.
func TestLookupSpans_ArchiveDisablesFastPath(t *testing.T) {
	tqs := initializeTestService(withArchiveTraceReader())
	tqs.traceReader.On("GetTraces", mock.Anything, mock.Anything).Return(wholeTraceSeq()).Once()
	tqs.archiveTraceReader.On("GetTraces", mock.Anything, mock.Anything).
		Return(wholeTraceSeq(lookupTrace(lookupTraceID, lookupSpanA))).Once()

	result, err := tqs.queryService.LookupSpans(context.Background(), lookupParams(lookupSpanA))

	require.NoError(t, err)
	assert.True(t, result.TraceFound)
	assert.Len(t, result.Spans, 1)
	tqs.traceReader.AssertNotCalled(t, "FindSpans", mock.Anything, mock.Anything)
}

// TestLookupSpans_TraceNotFound pins that a whole-trace read that yields nothing reports the trace as
// missing, which the handler turns into its trace-not-found error.
func TestLookupSpans_TraceNotFound(t *testing.T) {
	tqs := initializeBareTestQueryService()
	tqs.traceReader.On("SearchCapabilities", mock.Anything).Return(tracestore.SearchCapabilities{}, nil)
	tqs.traceReader.On("GetTraces", mock.Anything, mock.Anything).Return(wholeTraceSeq()).Once()

	result, err := tqs.queryService.LookupSpans(context.Background(), lookupParams(lookupSpanA))

	require.NoError(t, err)
	assert.False(t, result.TraceFound)
	assert.Empty(t, result.Spans)
}

// TestLookupSpans_WholeTraceReadErrorIsWrapped pins that an error from the whole-trace read is
// reported with its context.
func TestLookupSpans_WholeTraceReadErrorIsWrapped(t *testing.T) {
	tqs := initializeBareTestQueryService()
	tqs.traceReader.On("SearchCapabilities", mock.Anything).Return(tracestore.SearchCapabilities{}, nil)
	tqs.traceReader.On("GetTraces", mock.Anything, mock.Anything).
		Return(iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
			yield(nil, assert.AnError)
		})).Once()

	_, err := tqs.queryService.LookupSpans(context.Background(), lookupParams(lookupSpanA))

	require.ErrorIs(t, err, assert.AnError)
	assert.ErrorContains(t, err, "failed to get trace")
}
