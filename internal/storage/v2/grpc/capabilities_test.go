// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	"github.com/jaegertracing/jaeger/internal/proto-gen/storage/v2"
	expressionproto "github.com/jaegertracing/jaeger/internal/proto/expression/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	tracestoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore/mocks"
)

// TestQueryParametersCarryTheFilter pins the filter onto the query parameters themselves,
// which is what makes a remote backend that declares filter support actually receive one.
func TestQueryParametersCarryTheFilter(t *testing.T) {
	filter := &expression.Call{Op: expression.OpEq, Args: []expression.Expression{
		&expression.AttributeRef{Key: "http.route", Level: expression.LevelSpan},
		&expression.AnyValue{Value: "/cart"},
	}}

	sent, err := toProtoQueryParameters(tracestore.TraceQueryParams{Attributes: pcommon.NewMap(), Filter: filter})
	require.NoError(t, err)
	encoded, err := expressionproto.ToProto(filter)
	require.NoError(t, err)
	assert.Equal(t, encoded, sent.GetFilter())
	decoded, err := NewHandler(new(tracestoremocks.Reader), nil, nil).toTraceQueryParams(sent)
	require.NoError(t, err)
	assert.Equal(t, filter, decoded.Filter)

	noFilter, err := toProtoQueryParameters(tracestore.TraceQueryParams{Attributes: pcommon.NewMap()})
	require.NoError(t, err)
	assert.Nil(t, noFilter.GetFilter())
	decodedNoFilter, err := NewHandler(new(tracestoremocks.Reader), nil, nil).toTraceQueryParams(noFilter)
	require.NoError(t, err)
	assert.Nil(t, decodedNoFilter.Filter)
}

// TestRemoteIngress_RefusesAMixedQuery pins the mutual exclusion at the remote boundary. A client
// reaching this server directly bypasses the query service, so the invariant is checked here too:
// a query carrying both filtering models would otherwise reach the reader, which would answer one of
// them without saying which. The filter itself is passed on as sent, whatever the reader declared,
// because converting it toward the reader's capabilities is the query service's job (ADR-013).
func TestRemoteIngress_RefusesAMixedQuery(t *testing.T) {
	filter, err := expressionproto.ToProto(&expression.Call{
		Op: expression.OpEq,
		Args: []expression.Expression{
			&expression.AttributeRef{Key: "http.route", Level: expression.LevelSpan},
			&expression.AnyValue{Value: "/cart"},
		},
	})
	require.NoError(t, err)

	tests := map[string]*storage.TraceQueryParameters{
		"a service name beside it":    {Filter: filter, ServiceName: "cart"},
		"an operation name beside it": {Filter: filter, OperationName: "checkout"},
		"a duration bound beside it":  {Filter: filter, DurationMin: time.Second},
		"attributes beside it": {Filter: filter, Attributes: []*storage.KeyValue{
			{Key: "k", Value: &storage.AnyValue{Value: &storage.AnyValue_StringValue{StringValue: "v"}}},
		}},
	}
	for name, query := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := NewHandler(new(tracestoremocks.Reader), nil, nil).toTraceQueryParams(query)
			require.Error(t, err)
			assert.Equal(t, codes.InvalidArgument, status.Code(err))
			assert.Contains(t, err.Error(), "it cannot be combined with")
		})
	}
}

// TestQueryParametersRefuseAnUnsendableFilter pins that a filter the wire has no form for is
// answered here rather than sent as a query with no predicates at all.
func TestQueryParametersRefuseAnUnsendableFilter(t *testing.T) {
	filter := &expression.Call{Op: expression.OpEq, Args: []expression.Expression{
		&expression.AttributeRef{Key: "http.route"}, nil,
	}}
	_, err := toProtoQueryParameters(tracestore.TraceQueryParams{Attributes: pcommon.NewMap(), Filter: filter})
	require.ErrorIs(t, err, expressionproto.ErrTermNotEncodable)
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
