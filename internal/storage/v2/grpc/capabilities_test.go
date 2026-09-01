// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
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
	decoded, err := filterCapableHandler(t).toTraceQueryParams(t.Context(), sent)
	require.NoError(t, err)
	assert.Equal(t, filter, decoded.Filter)

	noFilter, err := toProtoQueryParameters(tracestore.TraceQueryParams{Attributes: pcommon.NewMap()})
	require.NoError(t, err)
	assert.Nil(t, noFilter.GetFilter())
	decodedNoFilter, err := filterCapableHandler(t).toTraceQueryParams(t.Context(), noFilter)
	require.NoError(t, err)
	assert.Nil(t, decodedNoFilter.Filter)
}

// TestRemoteIngress_RefusesAMixedQuery pins the mutual exclusion at the remote boundary. A client
// reaching this server directly bypasses the query service, so the invariant is checked here too:
// a query carrying both filtering models would otherwise reach the reader, which would answer one of
// them without saying which.
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
			_, err := filterCapableHandler(t).toTraceQueryParams(t.Context(), query)
			require.Error(t, err)
			assert.Equal(t, codes.InvalidArgument, status.Code(err))
			assert.Contains(t, err.Error(), "it cannot be combined with")
		})
	}
}

// TestRemoteIngress_GivesAReaderTheModelItDeclared covers the other half: a reader that evaluates no
// filter is handed the legacy fields instead, because the alternative is a reader that ignores the
// field it does not know and answers with every trace in the range.
func TestRemoteIngress_GivesAReaderTheModelItDeclared(t *testing.T) {
	serviceIs, err := expressionproto.ToProto(&expression.Call{
		Op: expression.OpEq,
		Args: []expression.Expression{
			&expression.FieldRef{Name: expression.ResourceFieldService, Level: expression.LevelResource},
			&expression.AnyValue{Value: "cart"},
		},
	})
	require.NoError(t, err)

	t.Run("a reader with no filter support is given the legacy fields", func(t *testing.T) {
		reader := new(tracestoremocks.Reader)
		reader.On("SearchCapabilities", mock.Anything).
			Return(tracestore.SearchCapabilities{WithoutServiceName: true}, nil)

		query, err := NewHandler(reader, nil, nil).
			toTraceQueryParams(t.Context(), &storage.TraceQueryParameters{Filter: serviceIs})
		require.NoError(t, err)
		assert.Nil(t, query.Filter, "the filter was rewritten rather than passed on")
		assert.Equal(t, "cart", query.ServiceName)
	})

	t.Run("a filter the legacy fields cannot carry is refused", func(t *testing.T) {
		orFilter, err := expressionproto.ToProto(&expression.Call{Op: expression.OpOr, Args: []expression.Expression{
			&expression.Call{Op: expression.OpEq, Args: []expression.Expression{
				&expression.FieldRef{Name: expression.ResourceFieldService, Level: expression.LevelResource},
				&expression.AnyValue{Value: "cart"},
			}},
			&expression.Call{Op: expression.OpEq, Args: []expression.Expression{
				&expression.FieldRef{Name: expression.ResourceFieldService, Level: expression.LevelResource},
				&expression.AnyValue{Value: "checkout"},
			}},
		}})
		require.NoError(t, err)

		reader := new(tracestoremocks.Reader)
		reader.On("SearchCapabilities", mock.Anything).
			Return(tracestore.SearchCapabilities{WithoutServiceName: true}, nil)

		_, err = NewHandler(reader, nil, nil).
			toTraceQueryParams(t.Context(), &storage.TraceQueryParameters{Filter: orFilter})
		require.Error(t, err)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("a reader that cannot report its capabilities reads as the least capable", func(t *testing.T) {
		reader := new(tracestoremocks.Reader)
		reader.On("SearchCapabilities", mock.Anything).
			Return(tracestore.SearchCapabilities{}, assert.AnError)

		query, err := NewHandler(reader, nil, nil).
			toTraceQueryParams(t.Context(), &storage.TraceQueryParameters{Filter: serviceIs})
		require.NoError(t, err)
		assert.Nil(t, query.Filter)
		assert.Equal(t, "cart", query.ServiceName)
	})

	t.Run("a predicate the reader did not declare is refused", func(t *testing.T) {
		spanLevel, err := expressionproto.ToProto(&expression.Call{
			Op: expression.OpEq,
			Args: []expression.Expression{
				&expression.AttributeRef{Key: "http.route", Level: expression.LevelSpan},
				&expression.AnyValue{Value: "/cart"},
			},
		})
		require.NoError(t, err)

		reader := new(tracestoremocks.Reader)
		reader.On("SearchCapabilities", mock.Anything).Return(tracestore.SearchCapabilities{
			Filter: &tracestore.FilterCapabilities{
				Levels:    []expression.Level{expression.LevelResource},
				Operators: []expression.Operator{expression.OpEq},
			},
		}, nil)

		_, err = NewHandler(reader, nil, nil).
			toTraceQueryParams(t.Context(), &storage.TraceQueryParameters{Filter: spanLevel})
		require.Error(t, err)
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
		assert.Contains(t, err.Error(), `it does not index the "span" level`)
	})
}

// filterCapableHandler fronts a reader that evaluates the filters these tests send, so a test about
// the query crossing the wire is not also a test of what a reader declared.
func filterCapableHandler(*testing.T) *Handler {
	reader := new(tracestoremocks.Reader)
	reader.On("SearchCapabilities", mock.Anything).Return(tracestore.SearchCapabilities{
		WithoutServiceName: true,
		Filter: &tracestore.FilterCapabilities{
			Levels: []expression.Level{
				expression.LevelSpan, expression.LevelResource, expression.LevelEvent,
			},
			Operators: []expression.Operator{
				expression.OpAnd, expression.OpOr, expression.OpNot, expression.OpEq,
				expression.OpNe, expression.OpGt, expression.OpLt, expression.OpGte,
				expression.OpLte, expression.OpRegex, expression.OpExists,
				expression.OpIn, expression.OpNotIn,
			},
		},
	}, nil).Maybe() // A query with no filter never asks, which is itself worth not failing on.
	return NewHandler(reader, nil, nil)
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

// TestQueryParametersCarryPagination pins Pagination onto the query parameters the same way
// TestQueryParametersCarryTheFilter pins Filter: encoded by toProtoQueryParameters, decoded by
// toTraceQueryParams, on a reader that declares Paginated so the token is not refused.
func TestQueryParametersCarryPagination(t *testing.T) {
	reader := new(tracestoremocks.Reader)
	reader.On("SearchCapabilities", mock.Anything).Return(tracestore.SearchCapabilities{Paginated: true}, nil)

	sent, err := toProtoQueryParameters(tracestore.TraceQueryParams{
		Attributes: pcommon.NewMap(),
		Pagination: tracestore.Pagination{PageSize: 25, PageToken: "opaque-cursor"},
	})
	require.NoError(t, err)
	require.NotNil(t, sent.GetPagination())
	assert.Equal(t, uint32(25), sent.GetPagination().GetPageSize())
	assert.Equal(t, "opaque-cursor", sent.GetPagination().GetPageToken())

	decoded, err := NewHandler(reader, nil, nil).toTraceQueryParams(t.Context(), sent)
	require.NoError(t, err)
	assert.Equal(t, tracestore.Pagination{PageSize: 25, PageToken: "opaque-cursor"}, decoded.Pagination)
	assert.Equal(t, 25, decoded.SearchDepth, "PageSize overrides SearchDepth")
}

// TestToProtoQueryParameters_RejectsOutOfRangePageSize pins that a PageSize which does not fit
// in the wire's uint32 is refused before it can wrap into an unintended value, rather than
// silently sent as something the caller never asked for.
func TestToProtoQueryParameters_RejectsOutOfRangePageSize(t *testing.T) {
	tests := []struct {
		name     string
		pageSize int
	}{
		{name: "negative", pageSize: -1},
		{name: "larger than uint32", pageSize: math.MaxUint32 + 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := toProtoQueryParameters(tracestore.TraceQueryParams{
				Attributes: pcommon.NewMap(),
				Pagination: tracestore.Pagination{PageSize: test.pageSize},
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid pagination page size")
		})
	}
}

// TestToTraceQueryParams_RejectsPageTokenWhenUnsupported pins RFC 0014 §6.2 at the storage/v2
// boundary: a reader that cannot paginate cannot have minted the token, so the query is
// refused with InvalidArgument rather than silently treated as a new search.
func TestToTraceQueryParams_RejectsPageTokenWhenUnsupported(t *testing.T) {
	reader := new(tracestoremocks.Reader)
	reader.On("SearchCapabilities", mock.Anything).Return(tracestore.SearchCapabilities{}, nil)

	_, err := NewHandler(reader, nil, nil).toTraceQueryParams(t.Context(), &storage.TraceQueryParameters{
		Pagination: &storage.Pagination{PageToken: "opaque-cursor"},
	})
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
	assert.Contains(t, err.Error(), tracestore.ErrPaginationUnsupported.Error())
}

// TestToTraceQueryParams_PageSizeOnlySkipsCapabilityCheck pins that a Pagination with no
// PageToken never needs the reader's capabilities: it is not asking to resume anything, so
// nothing about pagination support needs answering — the reader has no expectations set, so a
// call to it would fail this test.
func TestToTraceQueryParams_PageSizeOnlySkipsCapabilityCheck(t *testing.T) {
	reader := new(tracestoremocks.Reader)

	decoded, err := NewHandler(reader, nil, nil).toTraceQueryParams(t.Context(), &storage.TraceQueryParameters{
		Pagination: &storage.Pagination{PageSize: 10},
	})
	require.NoError(t, err)
	assert.Equal(t, tracestore.Pagination{PageSize: 10}, decoded.Pagination)
	assert.Equal(t, 10, decoded.SearchDepth)
}
