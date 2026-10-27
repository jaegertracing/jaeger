// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	builder "github.com/jaegertracing/jaeger/internal/expression"
)

// constantFirst writes the constant on the left of an ordered comparison, which the builder's
// chained form cannot.
func constantFirst(op expression.Operator, value string, ref builder.Ref) *expression.Call {
	return p.Compare(op, p.RefOf(&expression.AnyValue{Value: value}), ref)
}

func TestFinalize(t *testing.T) {
	filter := p.And(
		constantFirst(expression.OpGt, "2s", p.Span().Duration),
		p.Attr("http.method").Eq("GET"),
	)

	finalized, err := FinalizeFilter(filter)
	require.NoError(t, err)
	assert.Equal(t, p.And(
		p.Span().Duration.Lt(p.Duration(2*time.Second)),
		p.Attr("http.method").Eq("GET"),
	), finalized, "the constant is read, and the reference comes first")
}

// TestFinalize_IsIdempotent is what lets every boundary finalize a filter it did not build: the
// query service after an interceptor has edited one, and the remote-storage server on a tree a
// client may already have finalized.
func TestFinalize_IsIdempotent(t *testing.T) {
	filters := map[string]*expression.Call{
		"an ordered comparison against a duration field": constantFirst(expression.OpGt, "2s", p.Span().Duration),
		"an equality against a word-valued field":        p.Span().Kind.Eq("server"),
		"a timestamp bound":                              p.Span().StartTime.Lte("2026-08-18T00:00:00Z"),
		"membership":                                     p.Span().Status.In(p.List(expression.ValueTypeString, "error")),
		"a quantified predicate":                         p.Some(p.Event(), p.Event().Name.Eq("exception")),
	}
	for name, filter := range filters {
		t.Run(name, func(t *testing.T) {
			once, err := FinalizeFilter(filter)
			require.NoError(t, err)
			twice, err := FinalizeFilter(once)
			require.NoError(t, err)
			assert.Equal(t, once, twice)
		})
	}
}

// TestFinalize_RefusesADepthNoConsumerCouldWalk covers the bound at the entry point every boundary
// calls, since that is where a tree arriving off a wire is stopped.
func TestFinalize_RefusesADepthNoConsumerCouldWalk(t *testing.T) {
	deep := p.Attr("a").Eq("1")
	for range expression.MaxNestingDepth {
		deep = p.Not(deep)
	}
	_, err := FinalizeFilter(deep)
	require.ErrorIs(t, err, expression.ErrTooDeeplyNested)
}

func TestFinalize_RefusesWhatValidationRefuses(t *testing.T) {
	// The builder has no method for an operator the vocabulary lacks, so that tree is built by hand.
	_, err := FinalizeFilter(&expression.Call{Op: "matches", Args: []expression.Expression{
		&expression.AttributeRef{Key: "a"}, &expression.AnyValue{Value: "b"},
	}})
	require.ErrorContains(t, err, `unknown filter operator "matches"`)

	_, err = FinalizeFilter(p.Span().Duration.Gt("banana"))
	require.ErrorContains(t, err, `cannot compare span.duration against "banana"`)
}
