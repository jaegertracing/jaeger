// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	expressionproto "github.com/jaegertracing/jaeger/internal/proto/expression/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// TestQueryParametersCarryTheFilter pins the filter onto the query parameters themselves,
// which is what makes a remote backend that declares filter support actually receive one.
func TestQueryParametersCarryTheFilter(t *testing.T) {
	filter := &expression.Call{Op: expression.OpEq, Args: []expression.Expression{
		&expression.AttributeRef{Key: "http.route", Level: expression.LevelSpan},
		&expression.AnyValue{Value: "/cart"},
	}}

	sent := toProtoQueryParameters(tracestore.TraceQueryParams{Attributes: pcommon.NewMap(), Filter: filter})
	assert.Equal(t, expressionproto.ToProto(filter), sent.GetFilter())
	decoded, err := toTraceQueryParams(sent)
	require.NoError(t, err)
	assert.Equal(t, filter, decoded.Filter)

	noFilter := toProtoQueryParameters(tracestore.TraceQueryParams{Attributes: pcommon.NewMap()})
	assert.Nil(t, noFilter.GetFilter())
	decodedNoFilter, err := toTraceQueryParams(noFilter)
	require.NoError(t, err)
	assert.Nil(t, decodedNoFilter.Filter)
}

// TestFilterCapabilitiesRoundTrip covers the declaration crossing the same boundary: a
// backend's levels and operators must arrive as declared, since the query service refuses
// filters based on them.
func TestFilterCapabilitiesRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		caps *tracestore.FilterCapabilities
	}{
		{
			name: "no filter support declared",
		},
		{
			name: "declared but at its least capable",
			caps: &tracestore.FilterCapabilities{Levels: []expression.Level{}, Operators: []expression.Operator{}},
		},
		{
			name: "a flat backend",
			caps: &tracestore.FilterCapabilities{
				Levels:    []expression.Level{expression.LevelSpan, expression.LevelResource, expression.LevelEvent},
				Operators: []expression.Operator{expression.OpEq},
			},
		},
		{
			name: "a fully capable backend",
			caps: &tracestore.FilterCapabilities{
				Levels: []expression.Level{
					expression.LevelSpan, expression.LevelResource, expression.LevelScope,
					expression.LevelEvent, expression.LevelLink,
				},
				Operators: []expression.Operator{
					expression.OpEq, expression.OpNe, expression.OpGt, expression.OpLt,
					expression.OpGte, expression.OpLte, expression.OpRegex, expression.OpExists,
					expression.OpAnd, expression.OpOr, expression.OpNot,
					expression.OpIn, expression.OpNotIn, expression.OpSome,
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.caps, fromProtoFilterCapabilities(toProtoFilterCapabilities(test.caps)))
		})
	}
}
