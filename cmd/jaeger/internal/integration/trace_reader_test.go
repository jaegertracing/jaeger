// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
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
		Pagination: tracestore.Pagination{PageSize: 2, PageToken: "current-page"},
		OrderBy:    order,
	}) {
		require.NoError(t, err)
		chunks = append(chunks, chunk)
	}
	require.NotNil(t, client.request)
	query := client.request.Query
	decodedOrder, err := tracestore.SpanOrderFromProto(query.OrderBy)
	require.NoError(t, err)
	assert.Equal(t, order, decodedOrder)
	assert.Equal(t, start, query.StartTimeMin)
	assert.Equal(t, end, query.StartTimeMax)
	decoded, err := expressionproto.CallFromProto(query.Filter)
	require.NoError(t, err)
	assert.Equal(t, filter, decoded)
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
