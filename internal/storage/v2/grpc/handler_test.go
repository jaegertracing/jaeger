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

	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/proto-gen/storage/v2"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	tracestoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore/mocks"
)

type testStream struct {
	storage.TraceReader_GetTracesServer
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
