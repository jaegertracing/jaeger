// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package pagetoken

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	exprproto "github.com/jaegertracing/jaeger/internal/proto/expression/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

var (
	windowStart = time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)
	windowEnd   = windowStart.Add(time.Hour)
)

func serviceIs(name string) *expression.Call {
	return &expression.Call{Op: expression.OpEq, Args: []expression.Expression{
		&expression.AttributeRef{Key: "service.name", Level: expression.LevelResource},
		&expression.AnyValue{Value: name},
	}}
}

func attrs(kv ...string) pcommon.Map {
	m := pcommon.NewMap()
	for i := 0; i < len(kv); i += 2 {
		m.PutStr(kv[i], kv[i+1])
	}
	return m
}

func sampleTraceQuery() tracestore.TraceQueryParams {
	return tracestore.TraceQueryParams{
		ServiceName:   "cart",
		OperationName: "checkout",
		Attributes:    attrs("a", "1", "b", "2"),
		StartTimeMin:  windowStart,
		StartTimeMax:  windowEnd,
		DurationMin:   time.Millisecond,
		DurationMax:   time.Second,
		SearchDepth:   20,
		Pagination:    &tracestore.Pagination{PageSize: 10, PageToken: "cursor"},
	}
}

func TestTraceQuery_IgnoresTheBoundAndTheCursor(t *testing.T) {
	base, err := TraceQuery(sampleTraceQuery())
	require.NoError(t, err)
	assert.Len(t, base, fingerprintSize)

	q := sampleTraceQuery()
	q.SearchDepth = 0
	q.Pagination = nil
	same, err := TraceQuery(q)
	require.NoError(t, err)
	assert.Equal(t, base, same, "the page bound and the cursor are not part of what selects the results")
}

func TestTraceQuery_AttributeOrderDoesNotMatter(t *testing.T) {
	q := sampleTraceQuery()
	q.Attributes = attrs("b", "2", "a", "1")
	reordered, err := TraceQuery(q)
	require.NoError(t, err)
	base, err := TraceQuery(sampleTraceQuery())
	require.NoError(t, err)
	assert.Equal(t, base, reordered)
}

// TestTraceQuery_EverySelectingFieldCounts changes each selecting field in turn and expects a
// different fingerprint, since each one changes which results the cursor is a position among.
func TestTraceQuery_EverySelectingFieldCounts(t *testing.T) {
	base, err := TraceQuery(sampleTraceQuery())
	require.NoError(t, err)
	changes := map[string]func(*tracestore.TraceQueryParams){
		"service":       func(q *tracestore.TraceQueryParams) { q.ServiceName = "checkout" },
		"operation":     func(q *tracestore.TraceQueryParams) { q.OperationName = "pay" },
		"attribute":     func(q *tracestore.TraceQueryParams) { q.Attributes = attrs("a", "1", "b", "3") },
		"no attributes": func(q *tracestore.TraceQueryParams) { q.Attributes = pcommon.Map{} },
		"start":         func(q *tracestore.TraceQueryParams) { q.StartTimeMin = windowStart.Add(-time.Minute) },
		"end":           func(q *tracestore.TraceQueryParams) { q.StartTimeMax = windowEnd.Add(time.Minute) },
		"zero end":      func(q *tracestore.TraceQueryParams) { q.StartTimeMax = time.Time{} },
		"min duration":  func(q *tracestore.TraceQueryParams) { q.DurationMin = 2 * time.Millisecond },
		"max duration":  func(q *tracestore.TraceQueryParams) { q.DurationMax = 2 * time.Second },
		"filter":        func(q *tracestore.TraceQueryParams) { q.Filter = serviceIs("cart") },
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			q := sampleTraceQuery()
			change(&q)
			changed, err := TraceQuery(q)
			require.NoError(t, err)
			assert.NotEqual(t, base, changed)
		})
	}
}

// TestTraceQuery_FieldBoundariesCount pins that the fields are hashed with their lengths: moving
// the boundary between two adjacent strings is a different query.
func TestTraceQuery_FieldBoundariesCount(t *testing.T) {
	a := tracestore.TraceQueryParams{ServiceName: "ab", OperationName: "c"}
	b := tracestore.TraceQueryParams{ServiceName: "a", OperationName: "bc"}
	fa, err := TraceQuery(a)
	require.NoError(t, err)
	fb, err := TraceQuery(b)
	require.NoError(t, err)
	assert.NotEqual(t, fa, fb)
}

func TestSpanQuery(t *testing.T) {
	q := tracestore.SpanQueryParams{
		StartTimeMin: windowStart,
		StartTimeMax: windowEnd,
		Filter:       serviceIs("cart"),
		Pagination:   tracestore.Pagination{PageSize: 10, PageToken: "cursor"},
	}
	base, err := SpanQuery(q)
	require.NoError(t, err)
	assert.Len(t, base, fingerprintSize)

	q.Pagination = tracestore.Pagination{}
	same, err := SpanQuery(q)
	require.NoError(t, err)
	assert.Equal(t, base, same, "pagination is not part of what selects the results")

	q.Filter = serviceIs("checkout")
	other, err := SpanQuery(q)
	require.NoError(t, err)
	assert.NotEqual(t, base, other)

	q.Filter = nil
	unfiltered, err := SpanQuery(q)
	require.NoError(t, err)
	assert.NotEqual(t, other, unfiltered)

	trace, err := TraceQuery(tracestore.TraceQueryParams{StartTimeMin: windowStart, StartTimeMax: windowEnd})
	require.NoError(t, err)
	assert.NotEqual(t, unfiltered, trace, "a span search and a trace search over the same window are different queries")
}

// TestFilterNotEncodable pins that a filter the wire cannot carry is reported rather than hashed
// as something else. Every filter reaching the query service has been finalized, so this is the
// one way the fingerprint can fail.
func TestFilterNotEncodable(t *testing.T) {
	broken := &expression.Call{Op: expression.OpEq, Args: []expression.Expression{(*expression.AttributeRef)(nil)}}

	_, err := TraceQuery(tracestore.TraceQueryParams{Filter: broken})
	require.ErrorIs(t, err, exprproto.ErrTermNotEncodable)

	_, err = SpanQuery(tracestore.SpanQueryParams{Filter: broken})
	require.ErrorIs(t, err, exprproto.ErrTermNotEncodable)
}
