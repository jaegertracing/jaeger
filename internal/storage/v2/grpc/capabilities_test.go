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
	decoded, err := toTraceQueryParams(sent)
	require.NoError(t, err)
	assert.Equal(t, filter, decoded.Filter)

	noFilter, err := toProtoQueryParameters(tracestore.TraceQueryParams{Attributes: pcommon.NewMap()})
	require.NoError(t, err)
	assert.Nil(t, noFilter.GetFilter())
	decodedNoFilter, err := toTraceQueryParams(noFilter)
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
			_, err := toTraceQueryParams(query)
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

// TestQueryParametersCarryPagination pins Pagination onto the query parameters the same way
// TestQueryParametersCarryTheFilter pins Filter: encoded by toProtoQueryParameters, decoded by
// toTraceQueryParams.
func TestQueryParametersCarryPagination(t *testing.T) {
	sent, err := toProtoQueryParameters(tracestore.TraceQueryParams{
		Attributes: pcommon.NewMap(),
		Pagination: tracestore.Pagination{PageSize: 25, PageToken: "opaque-cursor"},
	})
	require.NoError(t, err)
	require.NotNil(t, sent.GetPagination())
	assert.Equal(t, uint32(25), sent.GetPagination().GetPageSize())
	assert.Equal(t, "opaque-cursor", sent.GetPagination().GetPageToken())

	decoded, err := toTraceQueryParams(sent)
	require.NoError(t, err)
	assert.Equal(t, tracestore.Pagination{PageSize: 25, PageToken: "opaque-cursor"}, decoded.Pagination)
	assert.Zero(t, decoded.SearchDepth, "Pagination replaces search_depth rather than setting it")
}

// TestToTraceQueryParams_PageSizeClampedToMax pins that a page_size above tracestore.MaxPageSize
// is clamped down rather than refused, the treatment RFC 0014 §4 (and AIP-158) prescribes. This
// happens in DecodePagination itself, independent of the reader's capabilities.
func TestToTraceQueryParams_PageSizeClampedToMax(t *testing.T) {
	decoded, err := toTraceQueryParams(&storage.TraceQueryParameters{
		Pagination: &storage.Pagination{PageSize: tracestore.MaxPageSize + 1000},
	})
	require.NoError(t, err)
	assert.Equal(t, tracestore.MaxPageSize, decoded.Pagination.PageSize)
}

// TestToTraceQueryParams_RejectsPaginationWithSearchDepth pins RFC 0014 §4's mutual exclusivity
// at the storage/v2 boundary: a query that sets both has not said how many results it wants.
func TestToTraceQueryParams_RejectsPaginationWithSearchDepth(t *testing.T) {
	_, err := toTraceQueryParams(&storage.TraceQueryParameters{
		SearchDepth: 20,
		Pagination:  &storage.Pagination{PageSize: 10},
	})
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
	assert.Contains(t, err.Error(), "search_depth")
}

// TestToTraceQueryParams_RejectsEmptyPagination pins the proto-presence gap Copilot found: an
// explicitly-present-but-empty Pagination message (`{"pagination":{}}`) reads identically to an
// absent one once page_size (a plain proto3 scalar with no presence of its own) is copied into
// the Go zero-valued Pagination struct. The rejection has to happen at decode time, while
// GetPagination() != nil still distinguishes "sent, empty" from "not sent" — a check on the Go
// value afterward cannot.
func TestToTraceQueryParams_RejectsEmptyPagination(t *testing.T) {
	_, err := toTraceQueryParams(&storage.TraceQueryParameters{
		Pagination: &storage.Pagination{},
	})
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
	assert.Contains(t, err.Error(), "page_size is required")
}
