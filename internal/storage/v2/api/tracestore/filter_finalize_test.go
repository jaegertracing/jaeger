// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
)

func TestFinalize(t *testing.T) {
	filter := &expression.Call{Op: expression.OpAnd, Args: []expression.Expression{
		&expression.Call{Op: expression.OpGt, Args: []expression.Expression{
			&expression.AnyValue{Value: "2s"}, &expression.FieldRef{Name: expression.SpanFieldDuration, Level: expression.LevelSpan},
		}},
		&expression.Call{Op: expression.OpEq, Args: []expression.Expression{
			&expression.AttributeRef{Key: "http.method"}, &expression.AnyValue{Value: "GET"},
		}},
	}}

	finalized, err := FinalizeFilter(filter)
	require.NoError(t, err)
	assert.Equal(t, &expression.Call{Op: expression.OpAnd, Args: []expression.Expression{
		&expression.Call{Op: expression.OpLt, Args: []expression.Expression{
			&expression.FieldRef{Name: expression.SpanFieldDuration, Level: expression.LevelSpan},
			&expression.DurationValue{Value: 2 * time.Second},
		}},
		&expression.Call{Op: expression.OpEq, Args: []expression.Expression{
			&expression.AttributeRef{Key: "http.method"}, &expression.AnyValue{Value: "GET"},
		}},
	}}, finalized, "the constant is read, and the reference comes first")
}

// TestFinalize_IsIdempotent is what lets every boundary finalize a filter it did not build: the
// query service after an interceptor has edited one, and the remote-storage server on a tree a
// client may already have finalized.
func TestFinalize_IsIdempotent(t *testing.T) {
	filters := map[string]*expression.Call{
		"an ordered comparison against a duration field": {Op: expression.OpGt, Args: []expression.Expression{
			&expression.AnyValue{Value: "2s"}, &expression.FieldRef{Name: expression.SpanFieldDuration, Level: expression.LevelSpan},
		}},
		"an equality against a word-valued field": {Op: expression.OpEq, Args: []expression.Expression{
			&expression.FieldRef{Name: expression.SpanFieldKind, Level: expression.LevelSpan}, &expression.AnyValue{Value: "server"},
		}},
		"a timestamp bound": {Op: expression.OpLte, Args: []expression.Expression{
			&expression.FieldRef{Name: expression.SpanFieldStartTime, Level: expression.LevelSpan},
			&expression.AnyValue{Value: "2026-08-18T00:00:00Z"},
		}},
		"membership": {Op: expression.OpIn, Args: []expression.Expression{
			&expression.FieldRef{Name: expression.SpanFieldStatus, Level: expression.LevelSpan},
			&expression.List{Values: []string{"error"}, Type: expression.ValueTypeString},
		}},
		"a quantified predicate": {Op: expression.OpSome, Args: []expression.Expression{
			&expression.NestedRef{Level: expression.LevelEvent},
			&expression.Call{Op: expression.OpEq, Args: []expression.Expression{
				&expression.FieldRef{Name: expression.EventFieldName, Level: expression.LevelEvent}, &expression.AnyValue{Value: "exception"},
			}},
		}},
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
	deep := eq(&expression.AttributeRef{Key: "a"}, &expression.AnyValue{Value: "1"})
	for range expression.MaxNestingDepth {
		deep = &expression.Call{Op: expression.OpNot, Args: []expression.Expression{deep}}
	}
	_, err := FinalizeFilter(deep)
	require.ErrorIs(t, err, expression.ErrTooDeeplyNested)
}

func TestFinalize_RefusesWhatValidationRefuses(t *testing.T) {
	_, err := FinalizeFilter(&expression.Call{Op: "matches", Args: []expression.Expression{
		&expression.AttributeRef{Key: "a"}, &expression.AnyValue{Value: "b"},
	}})
	require.ErrorContains(t, err, `unknown filter operator "matches"`)

	_, err = FinalizeFilter(&expression.Call{Op: expression.OpGt, Args: []expression.Expression{
		&expression.FieldRef{Name: expression.SpanFieldDuration, Level: expression.LevelSpan}, &expression.AnyValue{Value: "banana"},
	}})
	require.ErrorContains(t, err, `cannot compare span.duration against "banana"`)
}
