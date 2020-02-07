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

package server

import (
	"context"
	"net"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"

	"github.com/jaegertracing/jaeger/cmd/collector/app/handler"
	"github.com/jaegertracing/jaeger/cmd/collector/app/processor"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
	"github.com/jaegertracing/jaeger/thrift-gen/sampling"
)

// test wrong port number
func TestFailToListen(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	server, err := StartGRPCServer(&GRPCServerParams{
		Port:          -1,
		Handler:       handler.NewGRPCHandler(logger, &mockSpanProcessor{}),
		SamplingStore: &mockSamplingStore{},
		Logger:        logger,
	})
	assert.Nil(t, server)
	assert.EqualError(t, err, "failed to listen on gRPC port: listen tcp: address -1: invalid port")
}

func TestFailServe(t *testing.T) {
	lis := bufconn.Listen(0)
	lis.Close()
	core, logs := observer.New(zap.NewAtomicLevelAt(zapcore.ErrorLevel))
	var wg sync.WaitGroup
	wg.Add(1)

	logger := zap.New(core)
	serveGRPC(grpc.NewServer(), lis, &GRPCServerParams{
		Handler:       handler.NewGRPCHandler(logger, &mockSpanProcessor{}),
		SamplingStore: &mockSamplingStore{},
		Logger:        logger,
		OnError: func(e error) {
			assert.Equal(t, 1, len(logs.All()))
			assert.Equal(t, "Could not launch gRPC service", logs.All()[0].Message)
			wg.Done()
		},
	})
	wg.Wait()
}

func TestSpanCollector(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	params := &GRPCServerParams{
		Handler:       handler.NewGRPCHandler(logger, &mockSpanProcessor{}),
		SamplingStore: &mockSamplingStore{},
		Logger:        logger,
	}

	server := grpc.NewServer()
	defer server.Stop()

	listener, err := net.Listen("tcp", ":0")
	defer listener.Close()

	serveGRPC(server, listener, params)

	conn, err := grpc.Dial(listener.Addr().String(), grpc.WithInsecure())
	require.NoError(t, err)
	defer conn.Close()

	c := api_v2.NewCollectorServiceClient(conn)
	response, err := c.PostSpans(context.Background(), &api_v2.PostSpansRequest{})
	require.NoError(t, err)
	require.NotNil(t, response)
}

type mockSamplingStore struct{}

func (s mockSamplingStore) GetSamplingStrategy(serviceName string) (*sampling.SamplingStrategyResponse, error) {
	return nil, nil
}

type mockSpanProcessor struct {
}

func (p *mockSpanProcessor) ProcessSpans(spans []*model.Span, _ processor.SpansOptions) ([]bool, error) {
	return []bool{}, nil
}
