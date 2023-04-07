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

package testutils

import (
	"context"
	"net"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
)

// GrpcCollector is a mock collector for tests
type GrpcCollector struct {
	listener net.Listener
	*mockSpanHandler
	server *grpc.Server
}

var _ api_v2.CollectorServiceServer = (*GrpcCollector)(nil)

// StartGRPCCollector starts GRPC collector on a random port
func StartGRPCCollector(t *testing.T) *GrpcCollector {
	server := grpc.NewServer()
	lis, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)
	handler := &mockSpanHandler{t: t}
	api_v2.RegisterCollectorServiceServer(server, handler)
	go func() {
		require.NoError(t, server.Serve(lis))
	}()
	return &GrpcCollector{
		mockSpanHandler: handler,
		listener:        lis,
		server:          server,
	}
}

// Listener returns server's listener
func (c *GrpcCollector) Listener() net.Listener {
	return c.listener
}

// Close closes the server
func (c *GrpcCollector) Close() error {
	c.server.GracefulStop()
	return c.listener.Close()
}

type mockSpanHandler struct {
	t        *testing.T
	mux      sync.Mutex
	requests []*api_v2.PostSpansRequest
}

// GetJaegerBatches returns accumulated Jaeger batches
func (h *mockSpanHandler) GetJaegerBatches() []model.Batch {
	h.mux.Lock()
	defer h.mux.Unlock()
	var batches []model.Batch
	for _, r := range h.requests {
		batches = append(batches, r.Batch)
	}
	return batches
}

func (h *mockSpanHandler) PostSpans(_ context.Context, r *api_v2.PostSpansRequest) (*api_v2.PostSpansResponse, error) {
	h.t.Logf("mockSpanHandler received %d spans", len(r.Batch.Spans))
	h.mux.Lock()
	defer h.mux.Unlock()
	h.requests = append(h.requests, r)
	return &api_v2.PostSpansResponse{}, nil
}
