// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"context"
	"iter"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"google.golang.org/grpc"

	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/proto-gen/storage/v2"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	tracestoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore/mocks"
)

type testStream struct {
	grpc.ServerStream
	sent    []*jptrace.TracesData
	sendErr error
}

func (*testStream) Context() context.Context {
	return context.Background()
}

func (f *testStream) Send(td *jptrace.TracesData) error {
	if f.sendErr != nil {
		return f.sendErr
	}
	f.sent = append(f.sent, td)
	return nil
}

func TestHandler_GetTraces(t *testing.T) {
	reader := new(tracestoremocks.Reader)
	start := time.Now()
	end := start.Add(time.Minute)
	query := tracestore.GetTraceParams{
		TraceID: pcommon.TraceID([16]byte{1}),
		Start:   start,
		End:     end,
	}
	trace := makeTestTrace()
	td := jptrace.TracesData(trace)

	tests := []struct {
		name         string
		traces       [][]ptrace.Traces
		expectedSent []*jptrace.TracesData
		sendErr      error
		getTraceErr  error
		expectedErr  error
	}{
		{
			name:   "single trace",
			traces: [][]ptrace.Traces{{trace}},
			expectedSent: []*jptrace.TracesData{
				&td,
			},
		},
		{
			name:         "multiple traces",
			traces:       [][]ptrace.Traces{{trace, trace}},
			expectedSent: []*jptrace.TracesData{&td, &td},
		},
		{
			name:         "multiple chunks",
			traces:       [][]ptrace.Traces{{trace, trace}, {trace, trace}},
			expectedSent: []*jptrace.TracesData{&td, &td, &td, &td},
		},
		{
			name:        "storage error",
			getTraceErr: assert.AnError,
			expectedErr: assert.AnError,
		},
		{
			name:        "send error",
			traces:      [][]ptrace.Traces{{trace, trace}},
			sendErr:     assert.AnError,
			expectedErr: assert.AnError,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader.On("GetTraces", mock.Anything, query).
				Return(iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
					if test.getTraceErr != nil {
						yield(nil, test.getTraceErr)
						return
					}
					for _, traces := range test.traces {
						if !yield(traces, nil) {
							return
						}
					}
				})).Once()

			server := NewHandler(reader)
			stream := &testStream{
				sendErr: test.sendErr,
			}
			err := server.GetTraces(&storage.GetTracesRequest{
				Query: []*storage.GetTraceParams{
					{
						TraceId:   []byte{1},
						StartTime: start,
						EndTime:   end,
					},
				},
			}, stream)
			if test.expectedErr != nil {
				require.ErrorIs(t, err, test.expectedErr)
			} else {
				require.NoError(t, err)
				require.Equal(t, test.expectedSent, stream.sent)
			}
		})
	}
}

func TestHandler_GetServices(t *testing.T) {
	tests := []struct {
		name             string
		services         []string
		err              error
		expectedServices []string
		expectedErr      error
	}{
		{
			name:             "success",
			services:         []string{"service1", "service2"},
			expectedServices: []string{"service1", "service2"},
		},
		{
			name:             "empty",
			services:         []string{},
			expectedServices: []string{},
		},
		{
			name:        "error",
			err:         assert.AnError,
			expectedErr: assert.AnError,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader := new(tracestoremocks.Reader)
			reader.On("GetServices", mock.Anything).
				Return(test.services, test.err).Once()

			server := NewHandler(reader)
			resp, err := server.GetServices(context.Background(), &storage.GetServicesRequest{})
			if test.expectedErr == nil {
				require.NoError(t, err)
				require.Equal(t, test.expectedServices, resp.Services)
			} else {
				require.ErrorIs(t, err, test.expectedErr)
			}
		})
	}
}

func TestHandler_GetOperations(t *testing.T) {
	params := tracestore.OperationQueryParams{
		ServiceName: "service",
		SpanKind:    "kind",
	}
	req := &storage.GetOperationsRequest{
		Service:  "service",
		SpanKind: "kind",
	}
	tests := []struct {
		name               string
		operations         []tracestore.Operation
		err                error
		expectedOperations []*storage.Operation
		expectedErr        error
	}{
		{
			name: "success",
			operations: []tracestore.Operation{
				{Name: "operation1", SpanKind: "kind"},
				{Name: "operation2", SpanKind: "kind"},
			},
			expectedOperations: []*storage.Operation{
				{Name: "operation1", SpanKind: "kind"},
				{Name: "operation2", SpanKind: "kind"},
			},
		},
		{
			name:               "empty",
			operations:         []tracestore.Operation{},
			expectedOperations: []*storage.Operation{},
		},
		{
			name:        "error",
			err:         assert.AnError,
			expectedErr: assert.AnError,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader := new(tracestoremocks.Reader)
			reader.On("GetOperations", mock.Anything, params).
				Return(test.operations, test.err).Once()

			server := NewHandler(reader)
			resp, err := server.GetOperations(context.Background(), req)
			if test.expectedErr == nil {
				require.NoError(t, err)
				require.Equal(t, test.expectedOperations, resp.Operations)
			} else {
				require.ErrorIs(t, err, test.expectedErr)
			}
		})
	}
}

func TestHandler_FindTraces(t *testing.T) {
	reader := new(tracestoremocks.Reader)
	query := tracestore.TraceQueryParams{
		ServiceName:   "service",
		OperationName: "operation",
		Attributes:    pcommon.NewMap(),
	}
	trace := makeTestTrace()
	td := jptrace.TracesData(trace)

	tests := []struct {
		name         string
		traces       [][]ptrace.Traces
		expectedSent []*jptrace.TracesData
		sendErr      error
		getTraceErr  error
		expectedErr  error
	}{
		{
			name:   "single trace",
			traces: [][]ptrace.Traces{{trace}},
			expectedSent: []*jptrace.TracesData{
				&td,
			},
		},
		{
			name:         "multiple traces",
			traces:       [][]ptrace.Traces{{trace, trace}},
			expectedSent: []*jptrace.TracesData{&td, &td},
		},
		{
			name:         "multiple chunks",
			traces:       [][]ptrace.Traces{{trace, trace}, {trace, trace}},
			expectedSent: []*jptrace.TracesData{&td, &td, &td, &td},
		},
		{
			name:        "storage error",
			getTraceErr: assert.AnError,
			expectedErr: assert.AnError,
		},
		{
			name:        "send error",
			traces:      [][]ptrace.Traces{{trace, trace}},
			sendErr:     assert.AnError,
			expectedErr: assert.AnError,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader.On("FindTraces", mock.Anything, query).
				Return(iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
					if test.getTraceErr != nil {
						yield(nil, test.getTraceErr)
						return
					}
					for _, traces := range test.traces {
						if !yield(traces, nil) {
							return
						}
					}
				})).Once()

			server := NewHandler(reader)
			stream := &testStream{
				sendErr: test.sendErr,
			}
			err := server.FindTraces(&storage.FindTracesRequest{
				Query: &storage.TraceQueryParameters{
					ServiceName:   "service",
					OperationName: "operation",
				},
			}, stream)
			if test.expectedErr != nil {
				require.ErrorIs(t, err, test.expectedErr)
			} else {
				require.NoError(t, err)
				require.Equal(t, test.expectedSent, stream.sent)
			}
		})
	}
}

func TestHandler_FindTraceIDs(t *testing.T) {
	reader := new(tracestoremocks.Reader)
	query := tracestore.TraceQueryParams{
		ServiceName:   "service",
		OperationName: "operation",
		Attributes:    pcommon.NewMap(),
	}
	now := time.Now()
	traceIDA := [16]byte{1}
	traceIDB := [16]byte{2}
	tests := []struct {
		name             string
		traceIDs         []tracestore.FoundTraceID
		expectedTraceIDs []*storage.FoundTraceID
		findTraceIDsErr  error
		expectedErr      error
	}{
		{
			name: "success",
			traceIDs: []tracestore.FoundTraceID{
				{
					TraceID: traceIDA,
					Start:   now,
					End:     now.Add(time.Minute),
				},
				{
					TraceID: traceIDB,
					Start:   now,
					End:     now.Add(time.Hour),
				},
			},
			expectedTraceIDs: []*storage.FoundTraceID{
				{
					TraceId: traceIDA[:],
					Start:   now,
					End:     now.Add(time.Minute),
				},
				{
					TraceId: traceIDB[:],
					Start:   now,
					End:     now.Add(time.Hour),
				},
			},
		},
		{
			name:             "empty",
			traceIDs:         []tracestore.FoundTraceID{},
			expectedTraceIDs: []*storage.FoundTraceID{},
		},
		{
			name:            "error",
			findTraceIDsErr: assert.AnError,
			expectedErr:     assert.AnError,
		},
	}

	for _, test := range tests {
		reader.On("FindTraceIDs", mock.Anything, query).
			Return(iter.Seq2[[]tracestore.FoundTraceID, error](func(yield func([]tracestore.FoundTraceID, error) bool) {
				yield(test.traceIDs, test.findTraceIDsErr)
			})).Once()
		server := NewHandler(reader)

		response, err := server.FindTraceIDs(context.Background(), &storage.FindTracesRequest{
			Query: &storage.TraceQueryParameters{
				ServiceName:   "service",
				OperationName: "operation",
			},
		})
		if test.expectedErr != nil {
			require.ErrorIs(t, err, test.expectedErr)
		} else {
			require.NoError(t, err)
			require.Equal(t, test.expectedTraceIDs, response.TraceIds)
		}
	}
}

func TestConvertKeyValueListToMap(t *testing.T) {
	tests := []struct {
		name     string
		input    []*storage.KeyValue
		expected pcommon.Map
	}{
		{
			name:     "empty list",
			input:    []*storage.KeyValue{},
			expected: pcommon.NewMap(),
		},
		{
			name:     "nil entry",
			input:    []*storage.KeyValue{nil},
			expected: pcommon.NewMap(),
		},
		{
			name: "nil value",
			input: []*storage.KeyValue{
				{
					Key:   "key1",
					Value: nil,
				},
			},
			expected: pcommon.NewMap(),
		},
		{
			name: "primitive types",
			input: []*storage.KeyValue{
				{
					Key: "key1",
					Value: &storage.AnyValue{
						Value: &storage.AnyValue_StringValue{StringValue: "value1"},
					},
				},
				{
					Key: "key2",
					Value: &storage.AnyValue{
						Value: &storage.AnyValue_IntValue{IntValue: 42},
					},
				},
				{
					Key: "key3",
					Value: &storage.AnyValue{
						Value: &storage.AnyValue_DoubleValue{DoubleValue: 3.14},
					},
				},
				{
					Key: "key4",
					Value: &storage.AnyValue{
						Value: &storage.AnyValue_BoolValue{BoolValue: true},
					},
				},
				{
					Key: "key5",
					Value: &storage.AnyValue{
						Value: &storage.AnyValue_BytesValue{BytesValue: []byte{1, 2}},
					},
				},
			},
			expected: func() pcommon.Map {
				m := pcommon.NewMap()
				m.PutStr("key1", "value1")
				m.PutInt("key2", 42)
				m.PutDouble("key3", 3.14)
				m.PutBool("key4", true)
				m.PutEmptyBytes("key5").FromRaw([]byte{1, 2})
				return m
			}(),
		},
		{
			name: "nested map",
			input: []*storage.KeyValue{
				{
					Key: "key1",
					Value: &storage.AnyValue{
						Value: &storage.AnyValue_KvlistValue{
							KvlistValue: &storage.KeyValueList{
								Values: []*storage.KeyValue{
									{
										Key: "nestedKey",
										Value: &storage.AnyValue{
											Value: &storage.AnyValue_StringValue{StringValue: "nestedValue"},
										},
									},
									{
										Key:   "nilValueKey",
										Value: nil, // should be skipped
									},
								},
							},
						},
					},
				},
				nil, // should be skipped
			},
			expected: func() pcommon.Map {
				m := pcommon.NewMap()
				nested := m.PutEmptyMap("key1")
				nested.PutStr("nestedKey", "nestedValue")
				return m
			}(),
		},
		{
			name: "array",
			input: []*storage.KeyValue{
				{
					Key: "key1",
					Value: &storage.AnyValue{
						Value: &storage.AnyValue_ArrayValue{
							ArrayValue: &storage.ArrayValue{
								Values: []*storage.AnyValue{
									{
										Value: &storage.AnyValue_StringValue{StringValue: "value1"},
									},
									{
										Value: &storage.AnyValue_IntValue{IntValue: 42},
									},
									{
										Value: &storage.AnyValue_DoubleValue{DoubleValue: 3.14},
									},
									{
										Value: &storage.AnyValue_BoolValue{BoolValue: true},
									},
									{
										Value: &storage.AnyValue_BytesValue{BytesValue: []byte{1, 2}},
									},
									{
										Value: &storage.AnyValue_KvlistValue{
											KvlistValue: &storage.KeyValueList{
												Values: []*storage.KeyValue{
													{
														Key: "nestedKey",
														Value: &storage.AnyValue{
															Value: &storage.AnyValue_StringValue{StringValue: "nestedValue"},
														},
													},
													{
														Key:   "nilValueKey",
														Value: nil, // should be skipped
													},
													nil, // should be skipped
												},
											},
										},
									},
									nil,
								},
							},
						},
					},
				},
			},
			expected: func() pcommon.Map {
				m := pcommon.NewMap()
				slice := m.PutEmptySlice("key1")
				slice.AppendEmpty().SetStr("value1")
				slice.AppendEmpty().SetInt(42)
				slice.AppendEmpty().SetDouble(3.14)
				slice.AppendEmpty().SetBool(true)
				slice.AppendEmpty().SetEmptyBytes().FromRaw([]byte{1, 2})
				nested := slice.AppendEmpty().SetEmptyMap()
				nested.PutStr("nestedKey", "nestedValue")
				slice.AppendEmpty() // for the nil entry
				return m
			}(),
		},
		{
			name: "nested array",
			input: []*storage.KeyValue{
				{
					Key: "key1",
					Value: &storage.AnyValue{
						Value: &storage.AnyValue_ArrayValue{
							ArrayValue: &storage.ArrayValue{
								Values: []*storage.AnyValue{
									{
										Value: &storage.AnyValue_ArrayValue{
											ArrayValue: &storage.ArrayValue{
												Values: []*storage.AnyValue{
													{
														Value: &storage.AnyValue_StringValue{StringValue: "inner1"},
													},
													nil,
												},
											},
										},
									},
									nil,
								},
							},
						},
					},
				},
			},
			expected: func() pcommon.Map {
				m := pcommon.NewMap()
				slice := m.PutEmptySlice("key1")
				nestedSlice := slice.AppendEmpty().SetEmptySlice()
				nestedSlice.AppendEmpty().SetStr("inner1")
				nestedSlice.AppendEmpty() // for the nil entry
				slice.AppendEmpty()       // for the nil entry
				return m
			}(),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := convertKeyValueListToMap(test.input)
			assert.Equal(t, test.expected, result)
		})
	}
}
