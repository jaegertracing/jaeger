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
	"go.opentelemetry.io/collector/pdata/ptrace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/jaegertracing/jaeger/internal/jiter"
	"github.com/jaegertracing/jaeger/internal/jptrace"
	_ "github.com/jaegertracing/jaeger/pkg/gogocodec" // force gogo codec registration
	"github.com/jaegertracing/jaeger/internal/proto-gen/storage/v2"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// testServer implements the storage.TraceReaderServer interface
// to simulate responses for testing.
type testServer struct {
	storage.UnimplementedTraceReaderServer

	traces     []*jptrace.TracesData
	services   []string
	operations []*storage.Operation
	traceIDs   []*storage.FoundTraceID
	err        error
}

func (ts *testServer) GetTraces(_ *storage.GetTracesRequest, s storage.TraceReader_GetTracesServer) error {
	s.Send(&storage.TracesChunk{
		Traces: ts.traces,
	})
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

func (ts *testServer) FindTraceIDs(
	context.Context,
	*storage.FindTracesRequest,
) (*storage.FindTraceIDsResponse, error) {
	return &storage.FindTraceIDsResponse{
		TraceIds: ts.traceIDs,
	}, ts.err
}

func startTestServer(t *testing.T, testServer *testServer) *grpc.ClientConn {
	listener, err := net.Listen("tcp", ":0")
	require.NoError(t, err)

	server := grpc.NewServer()
	storage.RegisterTraceReaderServer(server, testServer)

	return startServer(t, server, listener)
}

func startServer(t *testing.T, server *grpc.Server, listener net.Listener) *grpc.ClientConn {
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
	queryParams := tracestore.TraceQueryParams{
		ServiceName:   "service-a",
		OperationName: "operation-a",
		Attributes:    pcommon.NewMap(),
	}
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
			queryParams: queryParams,
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
			queryParams: queryParams,
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
			queryParams: queryParams,
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
			queryParams:   queryParams,
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

func TestConvertMapToKeyValueList(t *testing.T) {
	tests := []struct {
		name       string
		attributes pcommon.Map
		expected   []*storage.KeyValue
	}{
		{
			name:       "empty map",
			attributes: pcommon.NewMap(),
			expected:   []*storage.KeyValue{},
		},
		{
			name: "empty value",
			attributes: func() pcommon.Map {
				m := pcommon.NewMap()
				m.PutEmpty("key1")
				return m
			}(),
			expected: []*storage.KeyValue{
				{
					Key:   "key1",
					Value: nil,
				},
			},
		},
		{
			name: "primitive types",
			attributes: func() pcommon.Map {
				m := pcommon.NewMap()
				m.PutStr("key1", "value1")
				m.PutInt("key2", 42)
				m.PutDouble("key3", 3.14)
				m.PutBool("key4", true)
				m.PutEmptyBytes("key5").Append(1, 2)
				return m
			}(),
			expected: []*storage.KeyValue{
				{
					Key: "key1",
					Value: &storage.AnyValue{
						Value: &storage.AnyValue_StringValue{
							StringValue: "value1",
						},
					},
				},
				{
					Key: "key2",
					Value: &storage.AnyValue{
						Value: &storage.AnyValue_IntValue{
							IntValue: 42,
						},
					},
				},
				{
					Key: "key3",
					Value: &storage.AnyValue{
						Value: &storage.AnyValue_DoubleValue{
							DoubleValue: 3.14,
						},
					},
				},
				{
					Key: "key4",
					Value: &storage.AnyValue{
						Value: &storage.AnyValue_BoolValue{
							BoolValue: true,
						},
					},
				},
				{
					Key: "key5",
					Value: &storage.AnyValue{
						Value: &storage.AnyValue_BytesValue{
							BytesValue: []byte{1, 2},
						},
					},
				},
			},
		},
		{
			name: "nested map",
			attributes: func() pcommon.Map {
				m := pcommon.NewMap()
				nested := pcommon.NewMap()
				nested.PutStr("nestedKey", "nestedValue")
				nested.CopyTo(m.PutEmptyMap("key1"))
				return m
			}(),
			expected: []*storage.KeyValue{
				{
					Key: "key1",
					Value: &storage.AnyValue{
						Value: &storage.AnyValue_KvlistValue{
							KvlistValue: &storage.KeyValueList{
								Values: []*storage.KeyValue{
									{
										Key: "nestedKey",
										Value: &storage.AnyValue{
											Value: &storage.AnyValue_StringValue{
												StringValue: "nestedValue",
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "array attribute",
			attributes: func() pcommon.Map {
				m := pcommon.NewMap()
				arr := pcommon.NewValueSlice()
				arr.Slice().AppendEmpty().SetStr("value1")
				arr.Slice().AppendEmpty().SetInt(42)
				arr.Slice().CopyTo(m.PutEmptySlice("key1"))
				return m
			}(),
			expected: []*storage.KeyValue{
				{
					Key: "key1",
					Value: &storage.AnyValue{
						Value: &storage.AnyValue_ArrayValue{
							ArrayValue: &storage.ArrayValue{
								Values: []*storage.AnyValue{
									{
										Value: &storage.AnyValue_StringValue{
											StringValue: "value1",
										},
									},
									{
										Value: &storage.AnyValue_IntValue{
											IntValue: 42,
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := convertMapToKeyValueList(test.attributes)
			require.Equal(t, test.expected, result)
		})
	}
}
