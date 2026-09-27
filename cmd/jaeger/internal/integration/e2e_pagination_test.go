// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	"github.com/jaegertracing/jaeger/internal/jiter"
	"github.com/jaegertracing/jaeger/internal/proto/api_v3"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	storagegrpc "github.com/jaegertracing/jaeger/internal/storage/v2/grpc"
	"github.com/jaegertracing/jaeger/ports"
)

func testRemotePagination(t *testing.T, collector *E2EStorageIntegration) {
	t.Cleanup(func() { collector.CleanUp(t) })
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	conn, err := grpc.NewClient(ports.PortToHostPort(ports.RemoteStorageGRPC), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close()) })
	reader := storagegrpc.NewTraceReader(conn)
	base := time.Now().Add(-time.Hour).Truncate(time.Second)
	query := tracestore.TraceQueryParams{
		ServiceName: "pagination-test", Attributes: pcommon.NewMap(),
		StartTimeMin: base, StartTimeMax: base.Add(time.Minute),
		Pagination: &tracestore.Pagination{PageSize: 10},
	}
	var want []pcommon.TraceID
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", query.ServiceName)
	spans := rs.ScopeSpans().AppendEmpty().Spans()
	for i := byte(1); i <= 3; i++ {
		span := spans.AppendEmpty()
		span.SetTraceID(pcommon.TraceID{i})
		span.SetSpanID(pcommon.SpanID{i})
		span.SetName("operation")
		span.SetStartTimestamp(pcommon.NewTimestampFromTime(base.Add(time.Duration(i) * time.Second)))
		span.SetEndTimestamp(pcommon.NewTimestampFromTime(base.Add(time.Duration(i)*time.Second + time.Millisecond)))
		want = append([]pcommon.TraceID{span.TraceID()}, want...)
	}
	require.NoError(t, collector.TraceWriter.WriteTraces(ctx, traces))
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		chunks, err := jiter.CollectWithErrors(reader.FindTraceIDs(ctx, query))
		require.NoError(c, err)
		require.Len(c, chunks, 1)
		require.Len(c, chunks[0].Results, len(want))
	}, 15*time.Second, 100*time.Millisecond)

	t.Run("StorageTraceIDs", func(t *testing.T) {
		q := query
		q.Pagination = &tracestore.Pagination{PageSize: 1}
		var firstToken tracestore.PageToken
		for i, id := range want {
			chunks, err := jiter.CollectWithErrors(reader.FindTraceIDs(ctx, q))
			require.NoError(t, err)
			require.Len(t, chunks, 1)
			require.Len(t, chunks[0].Results, 1)
			assert.Equal(t, id, chunks[0].Results[0].TraceID)
			q.Pagination.PageToken = chunks[0].NextPageToken
			if i == 0 {
				firstToken = q.Pagination.PageToken
			}
			if i < len(want)-1 {
				require.NotEmpty(t, q.Pagination.PageToken)
			} else {
				require.Empty(t, q.Pagination.PageToken)
			}
		}
		for name, token := range map[string]tracestore.PageToken{"malformed": "not-a-token", "different-query": firstToken} {
			t.Run(name, func(t *testing.T) {
				q := query
				q.OperationName = "different-operation"
				q.Pagination = &tracestore.Pagination{PageSize: 1, PageToken: token}
				chunks, err := jiter.CollectWithErrors(reader.FindTraceIDs(ctx, q))
				require.ErrorIs(t, err, tracestore.ErrPaginationInvalid)
				assert.Empty(t, chunks)
			})
		}
		t.Run("MalformedFilter", func(t *testing.T) {
			q := query
			q.ServiceName = ""
			q.Filter = &expression.Call{Op: "no-such-operator"}
			_, err := jiter.CollectWithErrors(reader.FindTraceIDs(ctx, q))
			require.Error(t, err)
			assert.Equal(t, codes.InvalidArgument, status.Code(err))
			assert.NotErrorIs(t, err, tracestore.ErrPaginationInvalid)
		})
	})

	t.Run("QuerySummaries", func(t *testing.T) {
		client := collector.TraceReader.(*traceReader).client
		q, err := toProtoQuery(query)
		require.NoError(t, err)
		q.Pagination = &api_v3.Pagination{PageSize: 2}
		var firstToken string
		for i := 0; i < len(want); i += 2 {
			summaries, token, err := collectSummaryPage(ctx, client, q)
			require.NoError(t, err)
			page := want[i:min(i+2, len(want))]
			require.Len(t, summaries, len(page))
			for j, id := range page {
				assert.Equal(t, id.String(), summaries[j].TraceId)
				assert.EqualValues(t, 1, summaries[j].SpanCount)
				assert.Equal(t, query.ServiceName, summaries[j].RootServiceName)
			}
			q.Pagination.PageToken = token
			if i == 0 {
				firstToken = token
			}
			if i+len(page) < len(want) {
				require.NotEmpty(t, token)
			} else {
				require.Empty(t, token)
			}
		}
		for name, token := range map[string]string{"malformed": "not-a-token", "different-query": firstToken} {
			t.Run(name, func(t *testing.T) {
				q.OperationName = "different-operation"
				q.Pagination.PageToken = token
				summaries, _, err := collectSummaryPage(ctx, client, q)
				require.Error(t, err)
				assert.Equal(t, codes.InvalidArgument, status.Code(err))
				assert.Contains(t, err.Error(), tracestore.ErrPaginationInvalid.Error())
				assert.Empty(t, summaries)

				params := url.Values{
					"query.serviceName":          {query.ServiceName},
					"query.operationName":        {q.OperationName},
					"query.startTimeMin":         {query.StartTimeMin.Format(time.RFC3339Nano)},
					"query.startTimeMax":         {query.StartTimeMax.Format(time.RFC3339Nano)},
					"query.pagination.pageSize":  {"1"},
					"query.pagination.pageToken": {token},
				}
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+ports.PortToHostPort(ports.QueryHTTP)+"/api/v3/trace-summaries?"+params.Encode(), http.NoBody)
				require.NoError(t, err)
				resp, err := testingHttpClient(t).Do(req)
				require.NoError(t, err)
				defer resp.Body.Close()
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "%s", body)
				assert.Contains(t, string(body), tracestore.ErrPaginationInvalid.Error())
			})
		}
	})
}

func collectSummaryPage(ctx context.Context, client api_v3.QueryServiceClient, query *api_v3.TraceQueryParameters) ([]*api_v3.TraceSummary, string, error) {
	stream, err := client.FindTraceSummaries(ctx, &api_v3.FindTraceSummariesRequest{Query: query})
	if err != nil {
		return nil, "", err
	}
	var summaries []*api_v3.TraceSummary
	var token string
	for {
		chunk, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return summaries, token, nil
		}
		if err != nil {
			return summaries, token, err
		}
		summaries = append(summaries, chunk.Summaries...)
		token = chunk.NextPageToken
	}
}
