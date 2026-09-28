// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	builder "github.com/jaegertracing/jaeger/internal/expression"
	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

func buildSpanOrderingTraces() []ptrace.Traces {
	base := time.Now().Add(-time.Hour).Truncate(time.Second)
	var traces []ptrace.Traces
	for _, f := range []struct {
		name, service   string
		trace, span     byte
		start, duration int
	}{
		{"a", "ordering-a", 2, 3, 1, 4},
		{"b", "ordering-b", 1, 2, 3, 2},
		{"c", "ordering-a", 2, 1, 2, 3},
		{"d", "ordering-a", 1, 1, 3, 1},
		{"e", "ordering-b", 3, 1, 4, 0},
		{"a", "ordering-a", 2, 3, 1, 4},
	} {
		trace := ptrace.NewTraces()
		resource := trace.ResourceSpans().AppendEmpty()
		resource.Resource().Attributes().PutStr("service.name", f.service)
		span := resource.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
		span.SetName(f.name)
		span.SetTraceID(pcommon.TraceID{0xE2, f.trace})
		span.SetSpanID(pcommon.SpanID{f.span})
		span.SetStartTimestamp(pcommon.NewTimestampFromTime(base.Add(time.Duration(f.start) * time.Second)))
		span.SetEndTimestamp(pcommon.NewTimestampFromTime(base.Add(time.Duration(f.start+f.duration) * time.Second)))
		span.Attributes().PutStr("ordering-corpus", "yes")
		traces = append(traces, trace)
	}
	return traces
}

func orderingTerm(field string, direction tracestore.SortDirection) tracestore.SpanSortOrder {
	return tracestore.SpanSortOrder{Expression: &expression.FieldRef{Level: expression.LevelSpan, Name: field}, Direction: direction}
}

func (s *StorageIntegration) testSpanOrdering(t *testing.T) {
	s.skipIfNeeded(t)
	first := s.Corpus.SpanOrdering[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
	query := tracestore.SpanQueryParams{
		StartTimeMin: first.StartTimestamp().AsTime().Add(-time.Minute),
		StartTimeMax: first.StartTimestamp().AsTime().Add(time.Minute),
		Filter:       (builder.Predicate{}).Span().Attr("ordering-corpus").Eq("yes"),
		Pagination:   tracestore.Pagination{PageSize: 10},
	}
	search := func(q tracestore.SpanQueryParams) ([]string, tracestore.PageToken, error) {
		var names []string
		var token tracestore.PageToken
		for chunk, err := range s.TraceReader.FindSpans(t.Context(), q) {
			if err != nil {
				return nil, "", err
			}
			for _, span := range jptrace.SpanIter(chunk.Results) {
				names = append(names, span.Name())
			}
			token = chunk.NextPageToken
		}
		return names, token, nil
	}
	require.True(t, s.waitForCondition(t, func(t *testing.T) bool {
		names, _, err := search(query)
		if err != nil {
			t.Log(err)
			return false
		}
		return len(names) == 6
	}), "the ordering corpus must be searchable before paging")
	cases := []struct {
		name  string
		order []tracestore.SpanSortOrder
		want  []string
	}{
		{"Default", nil, []string{"e", "d", "b", "c", "a", "a"}},
		{"StartAsc", []tracestore.SpanSortOrder{orderingTerm("startTime", "asc")}, []string{"a", "a", "c", "d", "b", "e"}},
		{"StartDesc", []tracestore.SpanSortOrder{orderingTerm("startTime", "desc")}, []string{"e", "d", "b", "c", "a", "a"}},
		{"DurationAsc", []tracestore.SpanSortOrder{orderingTerm("duration", "")}, []string{"e", "d", "b", "c", "a", "a"}},
		{"DurationDesc", []tracestore.SpanSortOrder{orderingTerm("duration", "desc")}, []string{"a", "a", "c", "b", "d", "e"}},
		{"TraceAsc", []tracestore.SpanSortOrder{orderingTerm("traceID", "asc")}, []string{"d", "b", "c", "a", "a", "e"}},
		{"TraceDesc", []tracestore.SpanSortOrder{orderingTerm("traceID", "desc")}, []string{"e", "c", "a", "a", "d", "b"}},
		{"SpanAsc", []tracestore.SpanSortOrder{orderingTerm("spanID", "asc")}, []string{"e", "d", "c", "b", "a", "a"}},
		{"SpanDesc", []tracestore.SpanSortOrder{orderingTerm("spanID", "desc")}, []string{"a", "a", "b", "e", "d", "c"}},
		{"MixedDirections", []tracestore.SpanSortOrder{orderingTerm("startTime", "asc"), orderingTerm("spanID", "desc")}, []string{"a", "a", "c", "b", "d", "e"}},
		{"TraceThenTime", []tracestore.SpanSortOrder{orderingTerm("traceID", "asc"), orderingTerm("startTime", "asc")}, []string{"d", "b", "a", "a", "c", "e"}},
		{"AllFields", []tracestore.SpanSortOrder{orderingTerm("spanID", "asc"), orderingTerm("traceID", "desc"), orderingTerm("duration", "desc"), orderingTerm("startTime", "asc")}, []string{"e", "c", "d", "b", "a", "a"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q := query
			q.OrderBy = tc.order
			names, token, err := search(q)
			require.NoError(t, err)
			assert.Equal(t, tc.want, names)
			assert.Empty(t, token)
			q.Pagination.PageSize = 1
			for i, want := range tc.want {
				names, token, err = search(q)
				require.NoError(t, err)
				require.Equal(t, []string{want}, names)
				if i == len(tc.want)-1 {
					require.Empty(t, token)
				} else {
					require.NotEmpty(t, token)
				}
				q.Pagination.PageToken = token
			}
		})
	}
	for _, tc := range []struct {
		name, field string
		value       expression.Expression
		want        []string
	}{
		{"DurationFilter", expression.SpanFieldDuration, &expression.DurationValue{Value: 2 * time.Second}, []string{"a", "a", "c"}},
		{"TimestampFilter", expression.SpanFieldStartTime, &expression.TimestampValue{Value: first.StartTimestamp().AsTime()}, []string{"c", "b", "d", "e"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := query
			q.Filter = &expression.Call{Op: expression.OpAnd, Args: []expression.Expression{
				query.Filter,
				&expression.Call{Op: expression.OpGt, Args: []expression.Expression{
					&expression.FieldRef{Level: expression.LevelSpan, Name: tc.field}, tc.value,
				}},
			}}
			q.OrderBy = []tracestore.SpanSortOrder{orderingTerm("duration", "desc")}
			names, token, err := search(q)
			require.NoError(t, err)
			assert.Equal(t, tc.want, names)
			assert.Empty(t, token)
		})
	}
	t.Run("EquivalentAndChangedOrderTokens", func(t *testing.T) {
		q := query
		q.Pagination.PageSize = 1
		_, token, err := search(q)
		require.NoError(t, err)
		require.NotEmpty(t, token)
		q.Pagination.PageToken = token
		q.Pagination.PageSize = 2
		q.OrderBy = []tracestore.SpanSortOrder{orderingTerm("startTime", "desc")}
		names, _, err := search(q)
		require.NoError(t, err)
		assert.Equal(t, []string{"d", "b"}, names)
		q.OrderBy = []tracestore.SpanSortOrder{orderingTerm("startTime", "asc")}
		_, _, err = search(q)
		require.ErrorContains(t, err, "different query")
	})
	t.Run("Empty", func(t *testing.T) {
		q := query
		q.OrderBy = []tracestore.SpanSortOrder{orderingTerm("duration", "desc")}
		q.Filter = (builder.Predicate{}).Span().Name.Eq("absent-ordering-span")
		names, token, err := search(q)
		require.NoError(t, err)
		assert.Empty(t, names)
		assert.Empty(t, token)
	})
}
