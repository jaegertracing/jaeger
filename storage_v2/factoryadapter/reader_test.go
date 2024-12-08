// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package factoryadapter

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/plugin/storage/memory"
	dependencyStoreMocks "github.com/jaegertracing/jaeger/storage/dependencystore/mocks"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	spanStoreMocks "github.com/jaegertracing/jaeger/storage/spanstore/mocks"
	"github.com/jaegertracing/jaeger/storage_v2/depstore"
	"github.com/jaegertracing/jaeger/storage_v2/tracestore"
)

func TestGetV1Reader_NoError(t *testing.T) {
	memstore := memory.NewStore()
	traceReader := &TraceReader{
		spanReader: memstore,
	}
	v1Reader, err := GetV1Reader(traceReader)
	require.NoError(t, err)
	require.Equal(t, memstore, v1Reader)
}

type fakeReader struct{}

func (*fakeReader) GetTrace(_ context.Context, _ pcommon.TraceID) (ptrace.Traces, error) {
	panic("not implemented")
}

func (*fakeReader) GetServices(_ context.Context) ([]string, error) {
	panic("not implemented")
}

func (*fakeReader) GetOperations(_ context.Context, _ tracestore.OperationQueryParameters) ([]tracestore.Operation, error) {
	panic("not implemented")
}

func (*fakeReader) FindTraces(_ context.Context, _ tracestore.TraceQueryParameters) ([]ptrace.Traces, error) {
	panic("not implemented")
}

func (*fakeReader) FindTraceIDs(_ context.Context, _ tracestore.TraceQueryParameters) ([]pcommon.TraceID, error) {
	panic("not implemented")
}

func TestGetV1Reader_Error(t *testing.T) {
	fr := &fakeReader{}
	_, err := GetV1Reader(fr)
	require.ErrorIs(t, err, errV1ReaderNotAvailable)
}

func TestTraceReader_GetTracePanics(t *testing.T) {
	memstore := memory.NewStore()
	traceReader := &TraceReader{
		spanReader: memstore,
	}
	require.Panics(t, func() { traceReader.GetTrace(context.Background(), pcommon.NewTraceIDEmpty()) })
}

func TestTraceReader_GetServicesDelegatesToSpanReader(t *testing.T) {
	sr := new(spanStoreMocks.Reader)
	expectedServices := []string{"service-a", "service-b"}
	sr.On("GetServices", mock.Anything).Return(expectedServices, nil)
	traceReader := &TraceReader{
		spanReader: sr,
	}
	services, err := traceReader.GetServices(context.Background())
	require.NoError(t, err)
	require.Equal(t, expectedServices, services)
}

func TestTraceReader_GetOperationsDelegatesResponse(t *testing.T) {
	tests := []struct {
		name               string
		operations         []spanstore.Operation
		expectedOperations []tracestore.Operation
		err                error
	}{
		{
			name: "successful response",
			operations: []spanstore.Operation{
				{
					Name:     "operation-a",
					SpanKind: "server",
				},
				{
					Name:     "operation-b",
					SpanKind: "server",
				},
			},
			expectedOperations: []tracestore.Operation{
				{
					Name:     "operation-a",
					SpanKind: "server",
				},
				{
					Name:     "operation-b",
					SpanKind: "server",
				},
			},
		},
		{
			name:               "nil response",
			operations:         nil,
			expectedOperations: []tracestore.Operation{},
		},
		{
			name:               "error response",
			operations:         nil,
			expectedOperations: []tracestore.Operation{},
			err:                errors.New("test error"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sr := new(spanStoreMocks.Reader)
			sr.On("GetOperations",
				mock.Anything,
				spanstore.OperationQueryParameters{
					ServiceName: "service-a",
					SpanKind:    "server",
				}).Return(test.operations, test.err)
			traceReader := &TraceReader{
				spanReader: sr,
			}
			operations, err := traceReader.GetOperations(
				context.Background(),
				tracestore.OperationQueryParameters{
					ServiceName: "service-a",
					SpanKind:    "server",
				})
			require.ErrorIs(t, err, test.err)
			require.Equal(t, test.expectedOperations, operations)
		})
	}
}

func TestTraceReader_FindTracesPanics(t *testing.T) {
	memstore := memory.NewStore()
	traceReader := &TraceReader{
		spanReader: memstore,
	}
	require.Panics(
		t,
		func() { traceReader.FindTraces(context.Background(), tracestore.TraceQueryParameters{}) },
	)
}

func TestTraceReader_FindTraceIDsDelegatesResponse(t *testing.T) {
	tests := []struct {
		name             string
		modelTraceIDs    []model.TraceID
		expectedTraceIDs []pcommon.TraceID
		err              error
	}{
		{
			name: "successful response",
			modelTraceIDs: []model.TraceID{
				{Low: 3, High: 2},
				{Low: 4, High: 3},
			},
			expectedTraceIDs: []pcommon.TraceID{
				pcommon.TraceID([]byte{0, 0, 0, 0, 0, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0, 3}),
				pcommon.TraceID([]byte{0, 0, 0, 0, 0, 0, 0, 3, 0, 0, 0, 0, 0, 0, 0, 4}),
			},
		},
		{
			name:             "empty response",
			modelTraceIDs:    []model.TraceID{},
			expectedTraceIDs: []pcommon.TraceID{},
		},
		{
			name:             "nil response",
			modelTraceIDs:    nil,
			expectedTraceIDs: []pcommon.TraceID{},
		},
		{
			name:             "error response",
			modelTraceIDs:    nil,
			expectedTraceIDs: []pcommon.TraceID{},
			err:              errors.New("test error"),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sr := new(spanStoreMocks.Reader)
			now := time.Now()
			sr.On(
				"FindTraceIDs",
				mock.Anything,
				&spanstore.TraceQueryParameters{
					ServiceName:   "service",
					OperationName: "operation",
					Tags:          map[string]string{"tag-a": "val-a"},
					StartTimeMin:  now,
					StartTimeMax:  now.Add(time.Minute),
					DurationMin:   time.Minute,
					DurationMax:   time.Hour,
					NumTraces:     10,
				},
			).Return(test.modelTraceIDs, test.err)
			traceReader := &TraceReader{
				spanReader: sr,
			}
			traceIDs, err := traceReader.FindTraceIDs(
				context.Background(),
				tracestore.TraceQueryParameters{
					ServiceName:   "service",
					OperationName: "operation",
					Tags:          map[string]string{"tag-a": "val-a"},
					StartTimeMin:  now,
					StartTimeMax:  now.Add(time.Minute),
					DurationMin:   time.Minute,
					DurationMax:   time.Hour,
					NumTraces:     10,
				},
			)
			require.ErrorIs(t, err, test.err)
			require.Equal(t, test.expectedTraceIDs, traceIDs)
		})
	}
}

func TestDependencyReader_GetDependencies(t *testing.T) {
	end := time.Now()
	start := end.Add(-1 * time.Minute)
	query := depstore.QueryParameters{
		StartTime: start,
		EndTime:   end,
	}
	expectedDeps := []model.DependencyLink{{Parent: "parent", Child: "child", CallCount: 12}}
	mr := new(dependencyStoreMocks.Reader)
	mr.On("GetDependencies", mock.Anything, end, time.Minute).Return(expectedDeps, nil)
	dr := NewDependencyReader(mr)
	deps, err := dr.GetDependencies(context.Background(), query)
	require.NoError(t, err)
	require.Equal(t, expectedDeps, deps)
}
