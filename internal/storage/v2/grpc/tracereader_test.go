// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/jaegertracing/jaeger/internal/jiter"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/proto-gen/storage/v2"
)

// testServer implements the storage.TraceReaderServer interface
// to simulate responses for testing.
type testServer struct {
	storage.UnimplementedTraceReaderServer

	services   []string
	operations []*storage.Operation
	traceIDs   []*storage.FoundTraceID
	err        error
}

func (s *testServer) GetServices(
	context.Context,
	*storage.GetServicesRequest,
) (*storage.GetServicesResponse, error) {
	return &storage.GetServicesResponse{
		Services: s.services,
	}, s.err
}

func (s *testServer) GetOperations(
	context.Context,
	*storage.GetOperationsRequest,
) (*storage.GetOperationsResponse, error) {
	return &storage.GetOperationsResponse{
		Operations: s.operations,
	}, s.err
}

func (s *testServer) FindTraceIDs(
	context.Context,
	*storage.FindTracesRequest,
) (*storage.FindTraceIDsResponse, error) {
	return &storage.FindTraceIDsResponse{
		TraceIds: s.traceIDs,
	}, s.err
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

func TestTraceReader_GetTraces(t *testing.T) {
	tr := &TraceReader{}

	require.Panics(t, func() {
		tr.GetTraces(context.Background(), tracestore.GetTraceParams{})
	})
}

func TestTraceReader_GetServices(t *testing.T) {
	tests := []struct {
		name             string
		testServer       *testServer
		expectedServices []string
		expectedError    string
	}{
		{
			name: "success",
			testServer: &testServer{
				services: []string{"service-a", "service-b"},
			},
			expectedServices: []string{"service-a", "service-b"},
		},
		{
			name: "error",
			testServer: &testServer{
				err: assert.AnError,
			},
			expectedError: "failed to get services",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			conn := startTestServer(t, test.testServer)

			reader := NewTraceReader(conn)
			services, err := reader.GetServices(context.Background())

			if test.expectedError != "" {
				require.ErrorContains(t, err, test.expectedError)
			} else {
				require.Equal(t, test.expectedServices, services)
			}
		})
	}
}

func TestTraceReader_GetOperations(t *testing.T) {
	tests := []struct {
		name          string
		testServer    *testServer
		expectedOps   []tracestore.Operation
		expectedError string
	}{
		{
			name: "success",
			testServer: &testServer{
				operations: []*storage.Operation{
					{Name: "operation-a", SpanKind: "kind"},
					{Name: "operation-b", SpanKind: "kind"},
				},
			},
			expectedOps: []tracestore.Operation{
				{Name: "operation-a", SpanKind: "kind"},
				{Name: "operation-b", SpanKind: "kind"},
			},
		},
		{
			name: "error",
			testServer: &testServer{
				err: assert.AnError,
			},
			expectedError: "failed to get operations",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			conn := startTestServer(t, test.testServer)

			reader := NewTraceReader(conn)
			ops, err := reader.GetOperations(context.Background(), tracestore.OperationQueryParams{
				ServiceName: "service-a",
				SpanKind:    "kind",
			})

			if test.expectedError != "" {
				require.ErrorContains(t, err, test.expectedError)
			} else {
				require.Equal(t, test.expectedOps, ops)
			}
		})
	}
}

func TestTraceReader_FindTraces(t *testing.T) {
	tr := &TraceReader{}

	require.Panics(t, func() {
		tr.FindTraces(context.Background(), tracestore.TraceQueryParams{})
	})
}

func TestTraceReader_FindTraceIDs(t *testing.T) {
	now := time.Now().UTC()
	tests := []struct {
		name          string
		testServer    *testServer
		queryParams   tracestore.TraceQueryParams
		expectedIDs   []tracestore.FoundTraceID
		expectedError string
	}{
		{
			name: "success",
			testServer: &testServer{
				traceIDs: []*storage.FoundTraceID{
					{
						TraceId: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
						Start:   now,
						End:     now.Add(1 * time.Second),
					},
					{
						TraceId: []byte{2, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
						Start:   now,
						End:     now.Add(1 * time.Minute),
					},
				},
			},
			queryParams: tracestore.TraceQueryParams{
				ServiceName:   "service-a",
				OperationName: "operation-a",
			},
			expectedIDs: []tracestore.FoundTraceID{
				{
					TraceID: pcommon.TraceID([16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}),
					Start:   now,
					End:     now.Add(1 * time.Second),
				},
				{
					TraceID: pcommon.TraceID([16]byte{2, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}),
					Start:   now,
					End:     now.Add(1 * time.Minute),
				},
			},
		},
		{
			name: "trace ID with less than 16 bytes",
			testServer: &testServer{
				traceIDs: []*storage.FoundTraceID{
					{
						TraceId: []byte{1, 2, 3, 4, 5, 6, 7, 8},
						Start:   now,
						End:     now.Add(1 * time.Second),
					},
				},
			},
			queryParams: tracestore.TraceQueryParams{
				ServiceName:   "service-a",
				OperationName: "operation-a",
			},
			expectedIDs: []tracestore.FoundTraceID{
				{
					TraceID: pcommon.TraceID([16]byte{1, 2, 3, 4, 5, 6, 7, 8}),
					Start:   now,
					End:     now.Add(1 * time.Second),
				},
			},
		},
		{
			name: "trace ID with more than 16 bytes",
			testServer: &testServer{
				traceIDs: []*storage.FoundTraceID{
					{
						TraceId: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17},
						Start:   now,
						End:     now.Add(1 * time.Second),
					},
				},
			},
			queryParams: tracestore.TraceQueryParams{
				ServiceName:   "service-a",
				OperationName: "operation-a",
			},
			expectedIDs: []tracestore.FoundTraceID{
				{
					TraceID: pcommon.TraceID([16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}),
					Start:   now,
					End:     now.Add(1 * time.Second),
				},
			},
		},
		{
			name: "error",
			testServer: &testServer{
				err: assert.AnError,
			},
			queryParams: tracestore.TraceQueryParams{
				ServiceName: "service-a",
			},
			expectedError: "failed to execute FindTraceIDs",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			conn := startTestServer(t, test.testServer)

			reader := NewTraceReader(conn)

			foundIDsIter := reader.FindTraceIDs(context.Background(), test.queryParams)
			foundIDs, err := jiter.FlattenWithErrors(foundIDsIter)

			if test.expectedError != "" {
				require.ErrorContains(t, err, test.expectedError)
			} else {
				require.NoError(t, err)
				require.Equal(t, test.expectedIDs, foundIDs)
			}
		})
	}
}
