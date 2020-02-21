// Copyright (c) 2018 The Jaeger Authors.
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

package grpc

import (
	"context"
	"io"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
	"github.com/jaegertracing/jaeger/thrift-gen/sampling"
)

func close(t *testing.T, c io.Closer) {
	require.NoError(t, c.Close())
}

func TestSamplingManager_GetSamplingStrategy(t *testing.T) {
	s, addr := initializeGRPCTestServer(t, func(s *grpc.Server) {
		api_v2.RegisterSamplingManagerServer(s, &mockSamplingHandler{})
	})
	conn, err := grpc.Dial(addr.String(), grpc.WithInsecure())
	defer close(t, conn)
	require.NoError(t, err)
	defer s.GracefulStop()
	manager := NewConfigManager(conn)
	resp, err := manager.GetSamplingStrategy("any")
	require.NoError(t, err)
	assert.Equal(t, &sampling.SamplingStrategyResponse{StrategyType: sampling.SamplingStrategyType_PROBABILISTIC}, resp)
}

func TestSamplingManager_GetSamplingStrategy_error(t *testing.T) {
	conn, err := grpc.Dial("foo", grpc.WithInsecure())
	defer close(t, conn)
	require.NoError(t, err)
	manager := NewConfigManager(conn)
	resp, err := manager.GetSamplingStrategy("any")
	require.Nil(t, resp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Error while dialing dial tcp: address foo: missing port in address")
}

func TestSamplingManager_GetBaggageRestrictions(t *testing.T) {
	manager := NewConfigManager(nil)
	rest, err := manager.GetBaggageRestrictions("foo")
	require.Nil(t, rest)
	assert.EqualError(t, err, "baggage not implemented")
}

type mockSamplingHandler struct {
}

func (*mockSamplingHandler) GetSamplingStrategy(context.Context, *api_v2.SamplingStrategyParameters) (*api_v2.SamplingStrategyResponse, error) {
	return &api_v2.SamplingStrategyResponse{StrategyType: api_v2.SamplingStrategyType_PROBABILISTIC}, nil
}

func initializeGRPCTestServer(t *testing.T, beforeServe func(server *grpc.Server)) (*grpc.Server, net.Addr) {
	server := grpc.NewServer()
	lis, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)
	beforeServe(server)
	go func() {
		err := server.Serve(lis)
		require.NoError(t, err)
	}()
	return server, lis.Addr()
}
