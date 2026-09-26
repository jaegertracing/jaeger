// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"encoding/hex"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	exprproto "github.com/jaegertracing/jaeger/internal/proto/expression/v1"
)

var (
	windowStart = time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)
	windowEnd   = windowStart.Add(time.Hour)
)

func serviceAttributeIs(name string) *expression.Call {
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

func sampleTraceQuery() TraceQueryParams {
	return TraceQueryParams{
		ServiceName:   "cart",
		OperationName: "checkout",
		Attributes:    attrs("a", "1", "b", "2"),
		StartTimeMin:  windowStart,
		StartTimeMax:  windowEnd,
		DurationMin:   time.Millisecond,
		DurationMax:   time.Second,
		SearchDepth:   20,
		Pagination:    &Pagination{PageSize: 10, PageToken: "cursor"},
	}
}

func TestTraceQueryFingerprint_IgnoresTheBoundAndTheCursor(t *testing.T) {
	base, err := sampleTraceQuery().Fingerprint()
	require.NoError(t, err)
	assert.Len(t, base, fingerprintSize)

	q := sampleTraceQuery()
	q.SearchDepth = 0
	q.Pagination = nil
	same, err := q.Fingerprint()
	require.NoError(t, err)
	assert.Equal(t, base, same, "the page bound and the cursor are not part of what selects the results")
}

// TestFingerprint_Golden pins the fingerprints of two fixed queries. A token outlives the
// process that returned it, so a change to the hashing, its framing, the canonical form, or a
// field number of the expression proto the filter is hashed through would refuse every token
// still held by a client. A deliberate change of this kind bumps Version and updates these values.
func TestFingerprint_Golden(t *testing.T) {
	trace, err := sampleTraceQuery().Fingerprint()
	require.NoError(t, err)
	assert.Equal(t, "fe2deb1b5cda9ebbd876297368b28e7d", hex.EncodeToString(trace))

	span, err := (SpanQueryParams{
		StartTimeMin: windowStart, StartTimeMax: windowEnd, Filter: serviceAttributeIs("cart"),
	}).Fingerprint()
	require.NoError(t, err)
	assert.Equal(t, "da96d67bbd0ae4347baaec8e3b29f00a", hex.EncodeToString(span))
}

// TestTraceQueryFingerprint_ZeroAndEmptyAttributesAreTheSame pins that a request with no attributes
// fingerprints the same whether the API layer left the map at its zero value or built an empty
// one, since either form can arrive on either page.
func TestTraceQueryFingerprint_ZeroAndEmptyAttributesAreTheSame(t *testing.T) {
	zero := sampleTraceQuery()
	zero.Attributes = pcommon.Map{}
	empty := sampleTraceQuery()
	empty.Attributes = attrs()
	fz, err := zero.Fingerprint()
	require.NoError(t, err)
	fe, err := empty.Fingerprint()
	require.NoError(t, err)
	assert.Equal(t, fz, fe)
}

func TestTraceQueryFingerprint_AttributeOrderDoesNotMatter(t *testing.T) {
	q := sampleTraceQuery()
	q.Attributes = attrs("b", "2", "a", "1")
	reordered, err := q.Fingerprint()
	require.NoError(t, err)
	base, err := sampleTraceQuery().Fingerprint()
	require.NoError(t, err)
	assert.Equal(t, base, reordered)
}

// TestTraceQueryFingerprint_EverySelectingFieldCounts changes each selecting field in turn and expects a
// different fingerprint, since each one changes which results the cursor is a position among.
func TestTraceQueryFingerprint_EverySelectingFieldCounts(t *testing.T) {
	base, err := sampleTraceQuery().Fingerprint()
	require.NoError(t, err)
	changes := map[string]func(*TraceQueryParams){
		"service":       func(q *TraceQueryParams) { q.ServiceName = "checkout" },
		"operation":     func(q *TraceQueryParams) { q.OperationName = "pay" },
		"attribute":     func(q *TraceQueryParams) { q.Attributes = attrs("a", "1", "b", "3") },
		"no attributes": func(q *TraceQueryParams) { q.Attributes = pcommon.Map{} },
		"attribute type": func(q *TraceQueryParams) {
			q.Attributes = attrs("a", "1")
			q.Attributes.PutInt("b", 2)
		},
		"start":        func(q *TraceQueryParams) { q.StartTimeMin = windowStart.Add(-time.Minute) },
		"end":          func(q *TraceQueryParams) { q.StartTimeMax = windowEnd.Add(time.Minute) },
		"zero end":     func(q *TraceQueryParams) { q.StartTimeMax = time.Time{} },
		"min duration": func(q *TraceQueryParams) { q.DurationMin = 2 * time.Millisecond },
		"max duration": func(q *TraceQueryParams) { q.DurationMax = 2 * time.Second },
		"filter":       func(q *TraceQueryParams) { q.Filter = serviceAttributeIs("cart") },
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			q := sampleTraceQuery()
			change(&q)
			changed, err := q.Fingerprint()
			require.NoError(t, err)
			assert.NotEqual(t, base, changed)
		})
	}
}

// TestTraceQueryFingerprint_FieldBoundariesCount pins that the fields are hashed with their lengths: moving
// the boundary between two adjacent strings is a different query.
func TestTraceQueryFingerprint_FieldBoundariesCount(t *testing.T) {
	a := TraceQueryParams{ServiceName: "ab", OperationName: "c"}
	b := TraceQueryParams{ServiceName: "a", OperationName: "bc"}
	fa, err := a.Fingerprint()
	require.NoError(t, err)
	fb, err := b.Fingerprint()
	require.NoError(t, err)
	assert.NotEqual(t, fa, fb)
}

func TestSpanQueryFingerprint(t *testing.T) {
	q := SpanQueryParams{
		StartTimeMin: windowStart,
		StartTimeMax: windowEnd,
		Filter:       serviceAttributeIs("cart"),
		Pagination:   Pagination{PageSize: 10, PageToken: "cursor"},
	}
	base, err := q.Fingerprint()
	require.NoError(t, err)
	assert.Len(t, base, fingerprintSize)

	q.Pagination = Pagination{}
	same, err := q.Fingerprint()
	require.NoError(t, err)
	assert.Equal(t, base, same, "pagination is not part of what selects the results")

	q.Filter = serviceAttributeIs("checkout")
	other, err := q.Fingerprint()
	require.NoError(t, err)
	assert.NotEqual(t, base, other)

	q.Filter = nil
	unfiltered, err := q.Fingerprint()
	require.NoError(t, err)
	assert.NotEqual(t, other, unfiltered)

	trace, err := (TraceQueryParams{StartTimeMin: windowStart, StartTimeMax: windowEnd}).Fingerprint()
	require.NoError(t, err)
	assert.NotEqual(t, unfiltered, trace, "a span search and a trace search over the same window are different queries")
}

func spanQueryWith(filter *expression.Call) SpanQueryParams {
	return SpanQueryParams{StartTimeMin: windowStart, StartTimeMax: windowEnd, Filter: filter}
}

func tagIs(key, value string) *expression.Call {
	return &expression.Call{Op: expression.OpEq, Args: []expression.Expression{
		&expression.AttributeRef{Key: key, Level: expression.LevelSpan},
		&expression.AnyValue{Value: value},
	}}
}

func call(op expression.Operator, args ...expression.Expression) *expression.Call {
	return &expression.Call{Op: op, Args: args}
}

func fingerprintOf(t *testing.T, filter *expression.Call) []byte {
	fp, err := spanQueryWith(filter).Fingerprint()
	require.NoError(t, err)
	return fp
}

// TestFilter_InvariantUnderEquivalentPermutations pins that two filters selecting the same
// spans fingerprint alike however their commutative operands and list values are ordered,
// nested or not, while a permutation that changes the meaning does not.
func TestFilter_InvariantUnderEquivalentPermutations(t *testing.T) {
	a, b, c := tagIs("a", "1"), tagIs("b", "2"), tagIs("c", "3")
	in := func(values ...string) *expression.Call {
		return call(expression.OpIn,
			&expression.AttributeRef{Key: "k", Level: expression.LevelSpan},
			&expression.List{Values: values, Type: expression.ValueTypeString})
	}

	same := []struct {
		name string
		x, y *expression.Call
	}{
		{"and operands", call(expression.OpAnd, a, b, c), call(expression.OpAnd, c, a, b)},
		{"or operands", call(expression.OpOr, a, b), call(expression.OpOr, b, a)},
		{"nested", call(expression.OpAnd, call(expression.OpOr, a, b), c), call(expression.OpAnd, c, call(expression.OpOr, b, a))},
		{"list values", in("x", "y", "z"), in("z", "x", "y")},
	}
	for _, tc := range same {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, fingerprintOf(t, tc.x), fingerprintOf(t, tc.y))
		})
	}

	duration := &expression.FieldRef{Level: expression.LevelSpan, Name: "duration"}
	second := &expression.DurationValue{Value: time.Second}
	different := []struct {
		name string
		x, y *expression.Call
	}{
		{"and versus or", call(expression.OpAnd, a, b), call(expression.OpOr, a, b)},
		{"different operand", call(expression.OpAnd, a, b), call(expression.OpAnd, a, c)},
		{"operand count", call(expression.OpAnd, a, b), call(expression.OpAnd, a, b, c)},
		{"comparison operands", call(expression.OpGt, duration, second), call(expression.OpGt, second, duration)},
		{"list versus other list", in("x", "y"), in("x", "z")},
	}
	for _, tc := range different {
		t.Run(tc.name, func(t *testing.T) {
			assert.NotEqual(t, fingerprintOf(t, tc.x), fingerprintOf(t, tc.y))
		})
	}
}

// TestCanonicalize_LeavesTheInputAlone pins that canonicalization works on a copy, since the
// filter it is given is the one the reader is about to be dispatched.
func TestCanonicalize_LeavesTheInputAlone(t *testing.T) {
	a, b := tagIs("b", "2"), tagIs("a", "1")
	list := &expression.List{Values: []string{"z", "x"}, Type: expression.ValueTypeString}
	filter := call(expression.OpAnd, a, b, call(expression.OpIn, &expression.AttributeRef{Key: "k", Level: expression.LevelSpan}, list))

	_, err := canonicalize(filter)
	require.NoError(t, err)

	assert.Same(t, a, filter.Args[0])
	assert.Same(t, b, filter.Args[1])
	assert.Equal(t, []string{"z", "x"}, list.Values)
}

// TestCanonicalize_NilTerms pins that a nil call or list operand is carried through rather
// than dereferenced; the encoder is what refuses it.
func TestCanonicalize_NilTerms(t *testing.T) {
	filter := call(expression.OpAnd, (*expression.Call)(nil), (*expression.List)(nil))
	_, err := encodeCanonical(filter)
	require.ErrorIs(t, err, exprproto.ErrTermNotEncodable)

	nested := call(expression.OpAnd, call(expression.OpAnd, (*expression.Call)(nil)), tagIs("a", "1"))
	_, err = encodeCanonical(nested)
	require.ErrorIs(t, err, exprproto.ErrTermNotEncodable)
}

// TestFilterNotEncodable pins that a filter the wire cannot carry is reported rather than hashed
// as something else. Every filter reaching the query service has been finalized, so this is the
// one way the fingerprint can fail.
func TestFilterNotEncodable(t *testing.T) {
	broken := &expression.Call{Op: expression.OpEq, Args: []expression.Expression{(*expression.AttributeRef)(nil)}}

	_, err := (TraceQueryParams{Filter: broken}).Fingerprint()
	require.ErrorIs(t, err, exprproto.ErrTermNotEncodable)

	_, err = (SpanQueryParams{Filter: broken}).Fingerprint()
	require.ErrorIs(t, err, exprproto.ErrTermNotEncodable)
}
