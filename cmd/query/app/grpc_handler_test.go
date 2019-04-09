// Copyright (c) 2019 The Jaeger Authors.
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

package app

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/opentracing/opentracing-go"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
)

type mockQueryService struct {
}

func (q *mockQueryService) GetTrace(ctx context.Context, traceID model.TraceID) (*model.Trace, error) {
	return &model.Trace{
		Spans : []*model.Span{
			&model.Span{
				TraceID: traceID,
			},
		},
	}, nil
}

func (q *mockQueryService) ArchiveTrace(ctx context.Context, traceID model.TraceID) error {
	return errors.New("Trace with ID: "+traceID.String()+" not found")
}

func initializeGRPCTestServer(t *testing.T) (*grpc.Server, net.Addr) {
	server := grpc.NewServer()
	q := &mockQueryService{}
	handler := NewGRPCHandler(q, zap.NewNop(), opentracing.NoopTracer{})
	api_v2.RegisterQueryServiceServer(server, handler)

	lis, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)
	go func() {
		err := server.Serve(lis)
		require.NoError(t, err)
	}()
	return server, lis.Addr()
}

func newClient(t *testing.T, addr net.Addr) (api_v2.QueryServiceClient, *grpc.ClientConn) {
	conn, err := grpc.Dial(addr.String(), grpc.WithInsecure())
	require.NoError(t, err)
	return api_v2.NewQueryServiceClient(conn), conn
}

func TestGetTrace(t *testing.T) {
	server, addr := initializeGRPCTestServer(t)
	defer server.Stop()
	client, conn := newClient(t, addr)
	defer conn.Close()
	sampleTraceID, _ := model.TraceIDFromString("AAAAAAAAAABSlpqJVVcaPw==")

	getTraceClient, err := client.GetTrace(context.Background(), &api_v2.GetTraceRequest{
		TraceID: sampleTraceID,
	})
	spanResponseChunk, err := getTraceClient.Recv()
	require.NoError(t, err)
	require.Equal(t, spanResponseChunk.Spans[0].TraceID, sampleTraceID)
}