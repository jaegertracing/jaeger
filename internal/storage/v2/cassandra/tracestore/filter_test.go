// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

func TestFilterCapabilities(t *testing.T) {
	caps := FilterCapabilities()
	assert.ElementsMatch(t, []expression.Level{
		expression.LevelSpan,
		expression.LevelResource,
		expression.LevelEvent,
	}, caps.Levels)
	assert.ElementsMatch(t, []expression.Operator{
		expression.OpAnd,
		expression.OpEq,
	}, caps.Operators)
}

func TestLowerFilter_NoFilter(t *testing.T) {
	query := newTraceQueryParams(t)
	lowered, err := lowerFilter(query)
	require.NoError(t, err)
	assert.Equal(t, query, lowered)
}

func TestLowerFilter_SupportedConjunction(t *testing.T) {
	start := time.Now().Add(-time.Hour)
	end := time.Now()
	query := tracestore.TraceQueryParams{
		StartTimeMin: start,
		StartTimeMax: end,
		Filter: &expression.Call{
			Op: expression.OpAnd,
			Args: []expression.Expression{
				&expression.Call{
					Op: expression.OpEq,
					Args: []expression.Expression{
						&expression.FieldRef{Level: expression.LevelResource, Name: expression.ResourceFieldService},
						&expression.StringValue{Value: "service-1"},
					},
				},
				&expression.Call{
					Op: expression.OpEq,
					Args: []expression.Expression{
						&expression.AttributeRef{Key: "http.status_code"},
						&expression.AnyValue{Value: "500"},
					},
				},
			},
		},
	}
	lowered, err := lowerFilter(query)
	require.NoError(t, err)
	assert.Nil(t, lowered.Filter)
	assert.Equal(t, "service-1", lowered.ServiceName)
	value, ok := lowered.Attributes.Get("http.status_code")
	require.True(t, ok)
	assert.Equal(t, "500", value.AsString())
}

func TestLowerFilter_RefusesOr(t *testing.T) {
	query := tracestore.TraceQueryParams{
		Filter: &expression.Call{
			Op: expression.OpOr,
			Args: []expression.Expression{
				&expression.Call{
					Op: expression.OpEq,
					Args: []expression.Expression{
						&expression.AttributeRef{Key: "a"},
						&expression.AnyValue{Value: "1"},
					},
				},
				&expression.Call{
					Op: expression.OpEq,
					Args: []expression.Expression{
						&expression.AttributeRef{Key: "b"},
						&expression.AnyValue{Value: "2"},
					},
				},
			},
		},
	}
	_, err := lowerFilter(query)
	require.ErrorIs(t, err, tracestore.ErrFilterUnsupported)
}

func TestLowerFilter_RefusesUnindexedLevel(t *testing.T) {
	query := tracestore.TraceQueryParams{
		Filter: &expression.Call{
			Op: expression.OpEq,
			Args: []expression.Expression{
				&expression.AttributeRef{Key: "zone", Level: expression.LevelScope},
				&expression.AnyValue{Value: "us-east"},
			},
		},
	}
	_, err := lowerFilter(query)
	require.ErrorIs(t, err, tracestore.ErrFilterUnsupported)
}
