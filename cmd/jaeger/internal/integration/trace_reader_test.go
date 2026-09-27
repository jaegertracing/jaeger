// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"io"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	"github.com/jaegertracing/jaeger/internal/proto/api_v3"
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

func TestTraceReaderFindSpansPreservesNextPageToken(t *testing.T) {
	t.Skip("The integration trace reader does not implement FindSpans yet.")
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

func TestToProtoQueryPaginationBounds(t *testing.T) {
	for _, size := range []int{-1, math.MaxUint32 + 1} {
		_, err := toProtoQuery(tracestore.TraceQueryParams{
			Attributes: pcommon.NewMap(), Pagination: &tracestore.Pagination{PageSize: size},
		})
		require.ErrorContains(t, err, "PageSize must be in")
	}
	query, err := toProtoQuery(tracestore.TraceQueryParams{Attributes: pcommon.NewMap()})
	require.NoError(t, err)
	assert.Nil(t, query.Pagination)
}
