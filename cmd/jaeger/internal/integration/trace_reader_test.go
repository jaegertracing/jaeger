// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"errors"
	"io"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	builder "github.com/jaegertracing/jaeger/internal/expression"
	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/proto/api_v3"
	expressionproto "github.com/jaegertracing/jaeger/internal/proto/expression/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

type traceSummariesClient struct {
	api_v3.QueryServiceClient
	response *api_v3.FindTraceSummariesResponse
	request  *api_v3.FindTraceSummariesRequest
}

func (c *traceSummariesClient) FindTraceSummaries(
	_ context.Context,
	request *api_v3.FindTraceSummariesRequest,
	_ ...grpc.CallOption,
) (api_v3.QueryService_FindTraceSummariesClient, error) {
	c.request = request
	return &traceSummariesStream{response: c.response}, nil
}

type traceSummariesStream struct {
	grpc.ClientStream
	response *api_v3.FindTraceSummariesResponse
}

func (s *traceSummariesStream) Recv() (*api_v3.FindTraceSummariesResponse, error) {
	if s.response == nil {
		return nil, io.EOF
	}
	response := s.response
	s.response = nil
	return response, nil
}

type spansClient struct {
	api_v3.QueryServiceClient
	request   *api_v3.FindSpansRequest
	responses []*api_v3.FindSpansResponse
}

func (c *spansClient) FindSpans(_ context.Context, request *api_v3.FindSpansRequest, _ ...grpc.CallOption) (api_v3.QueryService_FindSpansClient, error) {
	c.request = request
	return &spansStream{responses: c.responses}, nil
}

type spansStream struct {
	grpc.ClientStream
	responses []*api_v3.FindSpansResponse
}

func (s *spansStream) Recv() (*api_v3.FindSpansResponse, error) {
	if len(s.responses) == 0 {
		return nil, io.EOF
	}
	response := s.responses[0]
	s.responses = s.responses[1:]
	return response, nil
}

func TestTraceReaderFindSpansPreservesPagination(t *testing.T) {
	spans := ptrace.NewTraces()
	span := spans.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span.SetTraceID(pcommon.TraceID{1})
	span.SetSpanID(pcommon.SpanID{2})
	data := jptrace.TracesData(spans)
	client := &spansClient{responses: []*api_v3.FindSpansResponse{
		{Spans: &data},
		{NextPageToken: "next-page"},
	}}
	reader := &traceReader{client: client}
	filter := (builder.Predicate{}).Resource().Service.Eq("service-a")
	start := time.Now().Add(-time.Hour)
	end := start.Add(time.Minute)
	order := []tracestore.SpanSortOrder{{Expression: &expression.FieldRef{Level: expression.LevelSpan, Name: "duration"}, Direction: tracestore.SortDescending}}
	var chunks []tracestore.PageChunk[ptrace.Traces]
	for chunk, err := range reader.FindSpans(context.Background(), tracestore.SpanQueryParams{
		StartTimeMin: start, StartTimeMax: end, Filter: filter,
		OrderBy:    order,
		Pagination: tracestore.Pagination{PageSize: 2, PageToken: "current-page"},
	}) {
		require.NoError(t, err)
		chunks = append(chunks, chunk)
	}
	require.NotNil(t, client.request)
	query := client.request.Query
	assert.Equal(t, start, query.StartTimeMin)
	assert.Equal(t, end, query.StartTimeMax)
	decoded, err := expressionproto.CallFromProto(query.Filter)
	require.NoError(t, err)
	assert.Equal(t, filter, decoded)
	decodedOrder, err := tracestore.SpanOrderFromProto(query.OrderBy)
	require.NoError(t, err)
	assert.Equal(t, order, decodedOrder)

	require.NotNil(t, query.Pagination)
	assert.EqualValues(t, 2, query.Pagination.PageSize)
	assert.Equal(t, "current-page", query.Pagination.PageToken)
	require.Len(t, chunks, 2)
	assert.Equal(t, spans, chunks[0].Results)
	assert.Empty(t, chunks[0].NextPageToken)
	assert.Zero(t, chunks[1].Results.SpanCount())
	assert.Equal(t, tracestore.PageToken("next-page"), chunks[1].NextPageToken)
}

func TestTraceReaderFindTraceIDsPreservesNextPageToken(t *testing.T) {
	t.Skip("The API v3 query service does not expose FindTraceIDs.")
}

func TestTraceReaderFindTraceSummariesPreservesPagination(t *testing.T) {
	reader := &traceReader{
		logger: zap.NewNop(),
		client: &traceSummariesClient{response: &api_v3.FindTraceSummariesResponse{
			NextPageToken: "next-page",
		}},
	}

	var chunks []tracestore.PageChunk[[]tracestore.TraceSummary]
	for chunk, err := range reader.FindTraceSummaries(context.Background(), tracestore.TraceQueryParams{
		Attributes: pcommon.NewMap(),
		Pagination: &tracestore.Pagination{PageSize: 2, PageToken: "current-page"},
	}) {
		require.NoError(t, err)
		chunks = append(chunks, chunk)
	}

	request := reader.client.(*traceSummariesClient).request
	require.NotNil(t, request.Query.Pagination)
	assert.EqualValues(t, 2, request.Query.Pagination.PageSize)
	assert.Equal(t, "current-page", request.Query.Pagination.PageToken)
	require.Len(t, chunks, 1)
	assert.Equal(t, tracestore.PageToken("next-page"), chunks[0].NextPageToken)
}

func TestToProtoQueryPreservesPageSize(t *testing.T) {
	for _, size := range []uint32{0, math.MaxUint32} {
		query, err := toProtoQuery(tracestore.TraceQueryParams{
			Attributes: pcommon.NewMap(), Pagination: &tracestore.Pagination{PageSize: size},
		})
		require.NoError(t, err)
		assert.Equal(t, size, query.Pagination.PageSize)
	}
	query, err := toProtoQuery(tracestore.TraceQueryParams{Attributes: pcommon.NewMap()})
	require.NoError(t, err)
	assert.Nil(t, query.Pagination)
}

type capabilitiesClient struct {
	response *api_v3.GetCapabilitiesResponse
	err      error
}

func (c *capabilitiesClient) GetCapabilities(
	context.Context,
	*api_v3.GetCapabilitiesRequest,
	...grpc.CallOption,
) (*api_v3.GetCapabilitiesResponse, error) {
	return c.response, c.err
}

func TestTraceReaderSearchCapabilities(t *testing.T) {
	reader := &traceReader{
		logger: zap.NewNop(),
		capabilities: &capabilitiesClient{response: &api_v3.GetCapabilitiesResponse{
			Search: &api_v3.SearchCapabilities{
				WithoutServiceName:  true,
				SameSpanConjunction: true,
				Filter: &api_v3.FilterCapabilities{
					Levels:    []string{"span", "event"},
					Operators: []string{"and", "or", "eq"},
				},
				Paginated:   true,
				SpanSearch:  true,
				SpanSorting: true,
			},
		}},
	}

	caps, err := reader.SearchCapabilities(context.Background())
	require.NoError(t, err)
	assert.Equal(t, tracestore.SearchCapabilities{
		WithoutServiceName:  true,
		SameSpanConjunction: true,
		Filter: &tracestore.FilterCapabilities{
			Levels:    []expression.Level{expression.LevelSpan, expression.LevelEvent},
			Operators: []expression.Operator{expression.OpAnd, expression.OpOr, expression.OpEq},
		},
		Paginated:   true,
		SpanSearch:  true,
		SpanSorting: true,
	}, caps)
}

func TestTraceReaderSearchCapabilitiesNoFilter(t *testing.T) {
	reader := &traceReader{
		logger:       zap.NewNop(),
		capabilities: &capabilitiesClient{response: &api_v3.GetCapabilitiesResponse{Search: &api_v3.SearchCapabilities{}}},
	}

	caps, err := reader.SearchCapabilities(context.Background())
	require.NoError(t, err)
	assert.Equal(t, tracestore.SearchCapabilities{}, caps)
}

func TestTraceReaderSearchCapabilitiesErrors(t *testing.T) {
	t.Run("Unimplemented reads as ErrUnsupported", func(t *testing.T) {
		reader := &traceReader{
			logger:       zap.NewNop(),
			capabilities: &capabilitiesClient{err: status.Error(codes.Unimplemented, "no capabilities")},
		}
		_, err := reader.SearchCapabilities(context.Background())
		require.ErrorIs(t, err, errors.ErrUnsupported)
	})
	t.Run("other statuses pass through", func(t *testing.T) {
		reader := &traceReader{
			logger:       zap.NewNop(),
			capabilities: &capabilitiesClient{err: status.Error(codes.Unavailable, "down")},
		}
		_, err := reader.SearchCapabilities(context.Background())
		assert.Equal(t, codes.Unavailable, status.Code(err))
	})
}

func TestConsumeTracesRestoresRefusals(t *testing.T) {
	collect := func(startErr error) error {
		reader := &traceReader{logger: zap.NewNop()}
		var got error
		reader.consumeTraces(func(_ []ptrace.Traces, err error) bool {
			got = err
			return true
		}, nil, startErr)
		return got
	}
	t.Run("Unimplemented reads as ErrUnsupported", func(t *testing.T) {
		err := collect(status.Error(codes.Unimplemented, "it does not index the \"scope\" level"))
		require.ErrorIs(t, err, errors.ErrUnsupported)
		assert.ErrorContains(t, err, "scope")
	})
	t.Run("InvalidArgument stays a status", func(t *testing.T) {
		err := collect(status.Error(codes.InvalidArgument, "bad filter"))
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
		assert.NotErrorIs(t, err, errors.ErrUnsupported)
	})
}
