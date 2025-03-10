// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/proto-gen/storage/v2"
)

// testServer implements the storage.TraceReaderServer interface
// to simulate responses for testing.
type testServer struct {
	storage.UnimplementedTraceReaderServer

	getServicesError error
}

func (s *testServer) GetServices(
	context.Context,
	*storage.GetServicesRequest,
) (*storage.GetServicesResponse, error) {
	if s.getServicesError != nil {
		return nil, s.getServicesError
	}
	return &storage.GetServicesResponse{
		Services: []string{"service-a", "service-b"},
	}, nil
}

func startTestServer(t *testing.T, testServer *testServer) *grpc.ClientConn {
	listener, err := net.Listen("tcp", ":0")
	require.NoError(t, err)

	server := grpc.NewServer()
	storage.RegisterTraceReaderServer(server, testServer)

	go func() {
		server.Serve(listener)
	}()

	conn, err := grpc.NewClient(
		listener.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)

	t.Cleanup(
		func() {
			conn.Close()
			server.Stop()
			listener.Close()
		},
	)

	return conn
}

func TestTraceReader_GetOperations(t *testing.T) {
	tr := &TraceReader{}

	require.Panics(t, func() {
		_, _ = tr.GetOperations(context.Background(), tracestore.OperationQueryParams{})
	})
}

func TestTraceReader_GetServices(t *testing.T) {
	tests := []struct {
		name             string
		testServer       *testServer
		expectedServices []string
		expectedError    error
	}{
		{
			name:             "success",
			testServer:       &testServer{},
			expectedServices: []string{"service-a", "service-b"},
		},
		{
			name: "error",
			testServer: &testServer{
				getServicesError: assert.AnError,
			},
			expectedError: errFailedToGetServices,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			conn := startTestServer(t, test.testServer)

			reader := NewTraceReader(conn)
			services, err := reader.GetServices(context.Background())

			require.ErrorIs(t, err, test.expectedError)
			require.Equal(t, test.expectedServices, services)
		})
	}
}

func TestTraceReader_FindTraces(t *testing.T) {
	tr := &TraceReader{}

	require.Panics(t, func() {
		_ = tr.FindTraces(context.Background(), tracestore.TraceQueryParams{})
	})
}

func TestTraceReader_FindTraceIDs(t *testing.T) {
	tr := &TraceReader{}

	require.Panics(t, func() {
		_ = tr.FindTraceIDs(context.Background(), tracestore.TraceQueryParams{})
	})
}
