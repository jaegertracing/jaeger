// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/jaegertracing/jaeger/internal/jiter"
	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/proto-gen/storage/v2"
)

// testServer implements the storage.TraceReaderServer interface
// to simulate responses for testing.
type testServer struct {
	storage.UnimplementedTraceReaderServer

	traces     []*jptrace.TracesData
	services   []string
	operations []*storage.Operation
	err        error
}

func (ts *testServer) GetTraces(_ *storage.GetTracesRequest, s storage.TraceReader_GetTracesServer) error {
	s.SendMsg(&storage.TracesChunk{
		Traces: ts.traces,
	})
	// s.Send(&storage.TracesChunk{
	// 	Traces: ts.traces,
	// })
	return ts.err
}

func (ts *testServer) GetServices(
	context.Context,
	*storage.GetServicesRequest,
) (*storage.GetServicesResponse, error) {
	return &storage.GetServicesResponse{
		Services: ts.services,
	}, ts.err
}

func (ts *testServer) GetOperations(
	context.Context,
	*storage.GetOperationsRequest,
) (*storage.GetOperationsResponse, error) {
	return &storage.GetOperationsResponse{
		Operations: ts.operations,
	}, ts.err
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
	tests := []struct {
		name           string
		traces         func() []*jptrace.TracesData
		expectedTraces func() []ptrace.Traces
		expectedError  string
	}{
		{
			name: "success",
			traces: func() []*jptrace.TracesData {
				traceA := ptrace.NewTraces()
				spanA := traceA.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()
				spanA.SetName("span-a")

				traceB := ptrace.NewTraces()
				spanB := traceA.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()
				spanB.SetName("span-b")

				return []*jptrace.TracesData{
					(*jptrace.TracesData)(&traceA),
					(*jptrace.TracesData)(&traceB),
				}
			},
			expectedTraces: func() []ptrace.Traces {
				traceA := ptrace.NewTraces()
				spanA := traceA.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()
				spanA.SetName("span-a")

				traceB := ptrace.NewTraces()
				spanB := traceA.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()
				spanB.SetName("span-b")

				return []ptrace.Traces{traceA, traceB}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			conn := startTestServer(t, &testServer{
				traces: test.traces(),
			})

			reader := NewTraceReader(conn)
			getTracesIter := reader.GetTraces(context.Background(), tracestore.GetTraceParams{})
			traces, err := jiter.FlattenWithErrors(getTracesIter)

			if test.expectedError != "" {
				require.ErrorContains(t, err, test.expectedError)
			} else {
				require.NoError(t, err)
				require.Equal(t, test.expectedTraces(), traces)
			}
		})
	}
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
	tr := &TraceReader{}

	require.Panics(t, func() {
		tr.FindTraceIDs(context.Background(), tracestore.TraceQueryParams{})
	})
}
