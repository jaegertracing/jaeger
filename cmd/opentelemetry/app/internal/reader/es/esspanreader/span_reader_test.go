// Copyright (c) 2020 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package esspanreader

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/internal/esclient"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/plugin/storage/es/spanstore/dbmodel"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

func TestGetServices(t *testing.T) {
	client := &mockClient{
		searchResponse: &esclient.SearchResponse{
			Aggs: map[string]esclient.AggregationResponse{
				serviceNameField: {
					Buckets: []struct {
						Key string `json:"key"`
					}{{Key: "foo"}, {Key: "bar"}},
				},
			},
		},
	}
	reader := NewEsSpanReader(client, zap.NewNop(), Config{})
	services, err := reader.GetServices(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{"foo", "bar"}, services)
}

func TestGetOperations(t *testing.T) {
	client := &mockClient{
		searchResponse: &esclient.SearchResponse{
			Aggs: map[string]esclient.AggregationResponse{
				operationNameField: {
					Buckets: []struct {
						Key string `json:"key"`
					}{{Key: "foo"}, {Key: "bar"}},
				},
			},
		},
	}
	reader := NewEsSpanReader(client, zap.NewNop(), Config{})
	operations, err := reader.GetOperations(context.Background(), spanstore.OperationQueryParameters{ServiceName: "baz"})
	require.NoError(t, err)
	assert.Equal(t, []spanstore.Operation{{Name: "foo"}, {Name: "bar"}}, operations)
}

func TestGetTrace(t *testing.T) {
	s := dbmodel.Span{
		TraceID: dbmodel.TraceID("aaaa"),
		SpanID:  dbmodel.SpanID("aaaa"),
	}
	jsonSpan, err := json.Marshal(s)
	require.NoError(t, err)
	jsonMessage := json.RawMessage(jsonSpan)

	client := &mockClient{
		multiSearchResponse: &esclient.MultiSearchResponse{
			Responses: []esclient.SearchResponse{
				{
					Hits: esclient.Hits{
						Total: 1,
						Hits: []esclient.Hit{
							{
								Source: &jsonMessage,
							},
						},
					},
				},
			},
		},
	}
	reader := NewEsSpanReader(client, zap.NewNop(), Config{TagDotReplacement: "@"})

	trace, err := reader.GetTrace(context.Background(), model.TraceID{})
	require.NoError(t, err)
	domain := dbmodel.NewToDomain("@")
	modelSpan, err := domain.SpanToDomain(&s)
	require.NoError(t, err)
	assert.Equal(t, &model.Trace{Spans: []*model.Span{modelSpan}}, trace)
}

func TestFindTraces(t *testing.T) {
	dbSpan := dbmodel.Span{
		TraceID: dbmodel.TraceID("aaaa"),
		SpanID:  dbmodel.SpanID("aaaa"),
	}
	jsonSpan, err := json.Marshal(dbSpan)
	require.NoError(t, err)
	jsonMessage := json.RawMessage(jsonSpan)

	client := &mockClient{
		searchResponse: &esclient.SearchResponse{
			Aggs: map[string]esclient.AggregationResponse{
				traceIDField: {
					Buckets: []struct {
						Key string `json:"key"`
					}{{Key: "aaaa"}},
				},
			},
		},
		multiSearchResponse: &esclient.MultiSearchResponse{
			Responses: []esclient.SearchResponse{
				{
					Hits: esclient.Hits{
						Total: 1,
						Hits: []esclient.Hit{
							{
								Source: &jsonMessage,
							},
						},
					},
				},
			},
		},
	}
	reader := NewEsSpanReader(client, zap.NewNop(), Config{TagDotReplacement: "@"})
	traces, err := reader.FindTraces(context.Background(), &spanstore.TraceQueryParameters{
		StartTimeMin: time.Now().Add(-time.Hour),
		StartTimeMax: time.Now(),
	})
	require.NoError(t, err)

	domain := dbmodel.NewToDomain("@")
	modelSpan, err := domain.SpanToDomain(&dbSpan)
	require.NoError(t, err)
	assert.Equal(t, []*model.Trace{{Spans: []*model.Span{modelSpan}}}, traces)
}

type mockClient struct {
	receivedBody        io.Reader
	searchResponse      *esclient.SearchResponse
	multiSearchResponse *esclient.MultiSearchResponse
	searchErr           error
}

var _ esclient.ElasticsearchClient = (*mockClient)(nil)

func (m *mockClient) PutTemplate(ctx context.Context, name string, template io.Reader) error {
	m.receivedBody = template
	return nil
}

func (m mockClient) Bulk(ctx context.Context, bulkBody io.Reader) (*esclient.BulkResponse, error) {
	panic("implement me")
}

func (m mockClient) AddDataToBulkBuffer(bulkBody *bytes.Buffer, data []byte, index, typ string) {
	panic("implement me")
}

func (m *mockClient) Index(ctx context.Context, body io.Reader, index, typ string) error {
	m.receivedBody = body
	return nil
}

func (m *mockClient) Search(ctx context.Context, query esclient.SearchBody, size int, indices ...string) (*esclient.SearchResponse, error) {
	return m.searchResponse, m.searchErr
}

func (m *mockClient) MultiSearch(ctx context.Context, queries []esclient.SearchBody) (*esclient.MultiSearchResponse, error) {
	return m.multiSearchResponse, nil
}

func (m *mockClient) MajorVersion() int {
	panic("implement me")
}
