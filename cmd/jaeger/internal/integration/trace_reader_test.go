// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"io"
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
}

func (c *traceSummariesClient) FindTraceSummaries(
	context.Context,
	*api_v3.FindTraceSummariesRequest,
	...grpc.CallOption,
) (api_v3.QueryService_FindTraceSummariesClient, error) {
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

func TestTraceReaderFindTraceSummariesPreservesNextPageToken(t *testing.T) {
	reader := &traceReader{
		logger: zap.NewNop(),
		client: &traceSummariesClient{response: &api_v3.FindTraceSummariesResponse{
			NextPageToken: "next-page",
		}},
	}

	var chunks []tracestore.PageChunk[[]tracestore.TraceSummary]
	for chunk, err := range reader.FindTraceSummaries(context.Background(), tracestore.TraceQueryParams{
		Attributes: pcommon.NewMap(),
	}) {
		require.NoError(t, err)
		chunks = append(chunks, chunk)
	}

	require.Len(t, chunks, 1)
	assert.Equal(t, "next-page", chunks[0].NextPageToken)
}
