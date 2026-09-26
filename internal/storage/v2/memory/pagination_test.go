// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package memory

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// findSpansPage returns the single chunk one FindSpans call yields.
func findSpansPage(t *testing.T, store *Store, query tracestore.SpanQueryParams) (tracestore.PageChunk[ptrace.Traces], error) {
	t.Helper()
	var chunks []tracestore.PageChunk[ptrace.Traces]
	var errs []error
	for chunk, err := range store.FindSpans(context.Background(), query) {
		chunks = append(chunks, chunk)
		errs = append(errs, err)
	}
	require.Len(t, chunks, 1, "FindSpans yields one chunk per call")
	return chunks[0], errs[0]
}

// findTraceIDsPage returns the single chunk one FindTraceIDs call yields.
func findTraceIDsPage(t *testing.T, store *Store, query tracestore.TraceQueryParams) (tracestore.PageChunk[[]tracestore.FoundTraceID], error) {
	t.Helper()
	var chunks []tracestore.PageChunk[[]tracestore.FoundTraceID]
	var errs []error
	for chunk, err := range store.FindTraceIDs(context.Background(), query) {
		chunks = append(chunks, chunk)
		errs = append(errs, err)
	}
	require.Len(t, chunks, 1, "FindTraceIDs yields one chunk per call")
	return chunks[0], errs[0]
}

func TestFindSpans_PagesInStartTimeOrder(t *testing.T) {
	store, _ := writeTwoTraceStore(t)
	query := tracestore.SpanQueryParams{Pagination: tracestore.Pagination{PageSize: 2}}

	first, err := findSpansPage(t, store, query)
	require.NoError(t, err)
	assert.Equal(t, []string{"handle-request", "call-backend"}, spanNames(first.Results), "latest start first")
	require.NotEmpty(t, first.NextPageToken, "a full page with a span left over carries a token")

	query.Pagination.PageToken = first.NextPageToken
	second, err := findSpansPage(t, store, query)
	require.NoError(t, err)
	assert.Equal(t, []string{"GET /"}, spanNames(second.Results))
	assert.Empty(t, second.NextPageToken, "the last page carries no token")
}

func TestFindSpans_PagesWithinTheTimeRange(t *testing.T) {
	store, base := writeTwoTraceStore(t)
	chunk, err := findSpansPage(t, store, tracestore.SpanQueryParams{
		StartTimeMin: base.Add(time.Millisecond),
		StartTimeMax: base.Add(time.Millisecond),
		Pagination:   tracestore.Pagination{PageSize: 1},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"call-backend"}, spanNames(chunk.Results), "each bound excludes one span")
	assert.Empty(t, chunk.NextPageToken, "nothing in the range follows the page")
}

// TestFindSpans_CursorSurvivesWritesBetweenPages pins that a page resumes after the cursor's
// key even when the store changed since: a span written later, and later-starting, is not
// pulled into the continuation, and nothing is repeated or skipped (RFC 0014 §3.4).
func TestFindSpans_CursorSurvivesWritesBetweenPages(t *testing.T) {
	store, base := writeTwoTraceStore(t)
	query := tracestore.SpanQueryParams{Pagination: tracestore.Pagination{PageSize: 1}}
	first, err := findSpansPage(t, store, query)
	require.NoError(t, err)
	assert.Equal(t, []string{"handle-request"}, spanNames(first.Results))

	newer := ptrace.NewTraces()
	span := newer.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span.SetTraceID(pcommon.TraceID{7})
	span.SetSpanID(pcommon.SpanID{7})
	span.SetName("written-later")
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(base.Add(48 * time.Hour)))
	require.NoError(t, store.WriteTraces(context.Background(), newer))

	query.Pagination.PageToken = first.NextPageToken
	second, err := findSpansPage(t, store, query)
	require.NoError(t, err)
	assert.Equal(t, []string{"call-backend"}, spanNames(second.Results))
}

func TestFindSpans_PageThatEndsExactlyCarriesNoToken(t *testing.T) {
	store, _ := writeTwoTraceStore(t)
	chunk, err := findSpansPage(t, store, tracestore.SpanQueryParams{Pagination: tracestore.Pagination{PageSize: 3}})
	require.NoError(t, err)
	assert.Len(t, spanNames(chunk.Results), 3)
	assert.Empty(t, chunk.NextPageToken)
}

// TestFindSpans_TieBreaksOnTraceAndSpanID pins that spans with the same start time are sorted
// by trace ID, then span ID, and that a cursor inside a tie resumes at the right span.
func TestFindSpans_TieBreaksOnTraceAndSpanID(t *testing.T) {
	store, err := NewStore(Configuration{MaxTraces: 10})
	require.NoError(t, err)
	at := pcommon.NewTimestampFromTime(time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC))
	traces := ptrace.NewTraces()
	spans := traces.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans()
	for _, id := range []struct {
		trace byte
		span  byte
	}{{2, 1}, {1, 2}, {1, 1}} {
		span := spans.AppendEmpty()
		span.SetTraceID(pcommon.TraceID{id.trace})
		span.SetSpanID(pcommon.SpanID{id.span})
		span.SetName(fmt.Sprintf("t%d/s%d", id.trace, id.span))
		span.SetStartTimestamp(at)
	}
	require.NoError(t, store.WriteTraces(context.Background(), traces))

	var names []string
	query := tracestore.SpanQueryParams{Pagination: tracestore.Pagination{PageSize: 1}}
	for range 4 {
		chunk, err := findSpansPage(t, store, query)
		require.NoError(t, err)
		names = append(names, spanNames(chunk.Results)...)
		if chunk.NextPageToken == "" {
			break
		}
		query.Pagination.PageToken = chunk.NextPageToken
	}
	assert.Equal(t, []string{"t1/s1", "t1/s2", "t2/s1"}, names)
}

func TestFindSpans_RefusesATokenItDidNotReturnForThisQuery(t *testing.T) {
	store, base := writeTwoTraceStore(t)
	paged := tracestore.SpanQueryParams{Pagination: tracestore.Pagination{PageSize: 1}}
	first, err := findSpansPage(t, store, paged)
	require.NoError(t, err)
	require.NotEmpty(t, first.NextPageToken)

	fingerprint, err := paged.Fingerprint()
	require.NoError(t, err)
	notASpanPosition, err := tracestore.NewPageToken(fingerprint, []byte{1, 2, 3})
	require.NoError(t, err)

	tests := []struct {
		name  string
		query tracestore.SpanQueryParams
		want  string
	}{
		{
			name: "another query's token",
			query: tracestore.SpanQueryParams{
				StartTimeMin: base.Add(time.Hour),
				Pagination:   tracestore.Pagination{PageSize: 1, PageToken: first.NextPageToken},
			},
			want: "different query",
		},
		{
			name:  "not a token",
			query: tracestore.SpanQueryParams{Pagination: tracestore.Pagination{PageSize: 1, PageToken: "not-a-token!"}},
			want:  "base64",
		},
		{
			name:  "a cursor that is not a span position",
			query: tracestore.SpanQueryParams{Pagination: tracestore.Pagination{PageSize: 1, PageToken: notASpanPosition}},
			want:  "span position",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			chunk, err := findSpansPage(t, store, tc.query)
			require.ErrorIs(t, err, tracestore.ErrPaginationInvalid)
			require.ErrorContains(t, err, tc.want)
			assert.Zero(t, chunk)
		})
	}
}

// TestFindSpans_FilterThatCannotBeFingerprintedIsRefused covers the one way Fingerprint fails:
// a filter whose argument is a nil call. Such a query is refused before any span is evaluated.
func TestFindSpans_FilterThatCannotBeFingerprintedIsRefused(t *testing.T) {
	store, _ := writeTwoTraceStore(t)
	query := tracestore.SpanQueryParams{
		Filter: &expression.Call{Op: expression.OpAnd, Args: []expression.Expression{(*expression.Call)(nil)}},
	}
	chunk, err := findSpansPage(t, store, query)
	require.Error(t, err)
	require.NotErrorIs(t, err, tracestore.ErrPaginationInvalid, "the filter is refused, not the token")
	assert.Zero(t, chunk)
}

func TestFindSpans_MalformedFilterIsRefusedOnEveryPath(t *testing.T) {
	store, _ := writeTwoTraceStore(t)
	malformed := &expression.Call{Op: expression.OpNot}

	chunk, err := findSpansPage(t, store, tracestore.SpanQueryParams{Filter: malformed})
	require.ErrorIs(t, err, tracestore.ErrFilterInvalid)
	assert.Zero(t, chunk)

	ids, err := findTraceIDsPage(t, store, tracestore.TraceQueryParams{Filter: malformed, SearchDepth: 10})
	require.ErrorIs(t, err, tracestore.ErrFilterInvalid)
	assert.Zero(t, ids)

	ids, err = findTraceIDsPage(t, store, tracestore.TraceQueryParams{Filter: malformed, Pagination: &tracestore.Pagination{PageSize: 10}})
	require.ErrorIs(t, err, tracestore.ErrFilterInvalid)
	assert.Zero(t, ids)

	for _, err := range store.FindTraces(context.Background(), tracestore.TraceQueryParams{Filter: malformed, SearchDepth: 10}) {
		require.ErrorIs(t, err, tracestore.ErrFilterInvalid)
	}
}

// writeTracesStartingAt writes n single-span traces whose start times increase with their
// trace ID, so a paginated search returns them in descending ID order.
func writeTracesStartingAt(t *testing.T, store *Store, n int, base time.Time) {
	t.Helper()
	for i := 1; i <= n; i++ {
		traces := ptrace.NewTraces()
		rs := traces.ResourceSpans().AppendEmpty()
		rs.Resource().Attributes().PutStr("service.name", "svc")
		span := rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
		span.SetTraceID(pcommon.TraceID{byte(i)})
		span.SetSpanID(pcommon.SpanID{1})
		span.SetName("op")
		span.SetStartTimestamp(pcommon.NewTimestampFromTime(base.Add(time.Duration(i) * time.Minute)))
		require.NoError(t, store.WriteTraces(context.Background(), traces))
	}
}

func traceIDBytes(ids []tracestore.FoundTraceID) []byte {
	out := make([]byte, len(ids))
	for i, id := range ids {
		out[i] = id.TraceID[0]
	}
	return out
}

func TestFindTraceIDs_PagesInStartTimeOrder(t *testing.T) {
	store, err := NewStore(Configuration{MaxTraces: 10})
	require.NoError(t, err)
	base := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	writeTracesStartingAt(t, store, 5, base)
	other := ptrace.NewTraces()
	other.ResourceSpans().AppendEmpty().Resource().Attributes().PutStr("service.name", "other")
	other.ResourceSpans().At(0).ScopeSpans().AppendEmpty().Spans().AppendEmpty().SetTraceID(pcommon.TraceID{9})
	require.NoError(t, store.WriteTraces(context.Background(), other))

	query := tracestore.TraceQueryParams{ServiceName: "svc", Attributes: pcommon.NewMap(), Pagination: &tracestore.Pagination{PageSize: 2}}
	var got [][]byte
	for range 4 {
		chunk, err := findTraceIDsPage(t, store, query)
		require.NoError(t, err)
		got = append(got, traceIDBytes(chunk.Results))
		if chunk.NextPageToken == "" {
			break
		}
		query.Pagination.PageToken = chunk.NextPageToken
	}
	assert.Equal(t, [][]byte{{5, 4}, {3, 2}, {1}}, got, "latest first, and the other service's trace is not among them")
}

// TestFindTraceIDs_PositionIsTheLatestMatchingSpan pins that a trace is sorted by the latest
// start time among its matching spans, not among all its spans (RFC 0014 §3.3).
func TestFindTraceIDs_PositionIsTheLatestMatchingSpan(t *testing.T) {
	store, err := NewStore(Configuration{MaxTraces: 10})
	require.NoError(t, err)
	base := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	write := func(traceID byte, spans ...struct {
		op string
		at time.Duration
	},
	) {
		traces := ptrace.NewTraces()
		rs := traces.ResourceSpans().AppendEmpty()
		rs.Resource().Attributes().PutStr("service.name", "svc")
		ss := rs.ScopeSpans().AppendEmpty()
		for i, s := range spans {
			span := ss.Spans().AppendEmpty()
			span.SetTraceID(pcommon.TraceID{traceID})
			span.SetSpanID(pcommon.SpanID{byte(i + 1)})
			span.SetName(s.op)
			span.SetStartTimestamp(pcommon.NewTimestampFromTime(base.Add(s.at)))
		}
		require.NoError(t, store.WriteTraces(context.Background(), traces))
	}
	type s = struct {
		op string
		at time.Duration
	}
	write(1, s{"a", time.Minute}, s{"b", 10 * time.Minute})
	write(2, s{"a", 5 * time.Minute})

	everything := tracestore.TraceQueryParams{ServiceName: "svc", Attributes: pcommon.NewMap(), Pagination: &tracestore.Pagination{PageSize: 10}}
	chunk, err := findTraceIDsPage(t, store, everything)
	require.NoError(t, err)
	assert.Equal(t, []byte{1, 2}, traceIDBytes(chunk.Results), "trace 1 is positioned by its later span")

	onlyA := tracestore.TraceQueryParams{ServiceName: "svc", Attributes: pcommon.NewMap(), OperationName: "a", Pagination: &tracestore.Pagination{PageSize: 10}}
	chunk, err = findTraceIDsPage(t, store, onlyA)
	require.NoError(t, err)
	assert.Equal(t, []byte{2, 1}, traceIDBytes(chunk.Results), "trace 1 is positioned by its only matching span")
}

func TestFindTraceIDs_PaginationRefusals(t *testing.T) {
	store, err := NewStore(Configuration{MaxTraces: 10})
	require.NoError(t, err)
	writeTracesStartingAt(t, store, 3, time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC))

	paged := tracestore.TraceQueryParams{ServiceName: "svc", Attributes: pcommon.NewMap(), Pagination: &tracestore.Pagination{PageSize: 1}}
	first, err := findTraceIDsPage(t, store, paged)
	require.NoError(t, err)
	require.NotEmpty(t, first.NextPageToken)
	fingerprint, err := paged.Fingerprint()
	require.NoError(t, err)
	notATracePosition, err := tracestore.NewPageToken(fingerprint, []byte{1, 2, 3})
	require.NoError(t, err)

	tests := []struct {
		name  string
		query tracestore.TraceQueryParams
		want  string
	}{
		{
			name:  "no page size",
			query: tracestore.TraceQueryParams{ServiceName: "svc", Attributes: pcommon.NewMap(), Pagination: &tracestore.Pagination{}},
			want:  "page size",
		},
		{
			name:  "another query's token",
			query: tracestore.TraceQueryParams{ServiceName: "other", Attributes: pcommon.NewMap(), Pagination: &tracestore.Pagination{PageSize: 1, PageToken: first.NextPageToken}},
			want:  "different query",
		},
		{
			name:  "a cursor that is not a trace position",
			query: tracestore.TraceQueryParams{ServiceName: "svc", Attributes: pcommon.NewMap(), Pagination: &tracestore.Pagination{PageSize: 1, PageToken: notATracePosition}},
			want:  "trace position",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			chunk, err := findTraceIDsPage(t, store, tc.query)
			require.ErrorIs(t, err, tracestore.ErrPaginationInvalid)
			require.ErrorContains(t, err, tc.want)
			assert.Zero(t, chunk)
		})
	}
}

func TestFindTraceIDs_FilterThatCannotBeFingerprintedIsRefused(t *testing.T) {
	store, err := NewStore(Configuration{MaxTraces: 10})
	require.NoError(t, err)
	query := tracestore.TraceQueryParams{
		Filter:     &expression.Call{Op: expression.OpAnd, Args: []expression.Expression{(*expression.Call)(nil)}},
		Pagination: &tracestore.Pagination{PageSize: 1},
	}
	chunk, err := findTraceIDsPage(t, store, query)
	require.Error(t, err)
	require.NotErrorIs(t, err, tracestore.ErrPaginationInvalid, "the filter is refused, not the token")
	assert.Zero(t, chunk)
}

// TestFindTraceIDs_CursorSurvivesWritesBetweenPages is TestFindSpans_CursorSurvivesWritesBetweenPages
// for a trace search.
func TestFindTraceIDs_CursorSurvivesWritesBetweenPages(t *testing.T) {
	store, err := NewStore(Configuration{MaxTraces: 10})
	require.NoError(t, err)
	base := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	writeTracesStartingAt(t, store, 3, base)
	query := tracestore.TraceQueryParams{ServiceName: "svc", Attributes: pcommon.NewMap(), Pagination: &tracestore.Pagination{PageSize: 1}}
	first, err := findTraceIDsPage(t, store, query)
	require.NoError(t, err)
	assert.Equal(t, []byte{3}, traceIDBytes(first.Results))

	writeTracesStartingAt(t, store, 9, base) // rewrites 1..3 unchanged and adds 4..9, all newer
	query.Pagination.PageToken = first.NextPageToken
	second, err := findTraceIDsPage(t, store, query)
	require.NoError(t, err)
	assert.Equal(t, []byte{2}, traceIDBytes(second.Results), "continues after trace 3, unaffected by the newer traces")
}

func TestSearchCapabilities_DeclaresPaginated(t *testing.T) {
	store, err := NewStore(Configuration{MaxTraces: 10})
	require.NoError(t, err)
	caps, err := store.SearchCapabilities(context.Background())
	require.NoError(t, err)
	assert.True(t, caps.Paginated)
}
