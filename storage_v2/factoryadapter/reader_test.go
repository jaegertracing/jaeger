// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package factoryadapter

import (
	"context"
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
	sr := new(spanStoreMocks.Reader)
	o := []spanstore.Operation{
		{
			Name:     "operation-a",
			SpanKind: "server",
		},
		{
			Name:     "operation-b",
			SpanKind: "server",
		},
	}
	sr.On("GetOperations",
		mock.Anything,
		spanstore.OperationQueryParameters{
			ServiceName: "service-a",
			SpanKind:    "server",
		}).Return(o, nil)
	traceReader := &TraceReader{
		spanReader: sr,
	}
	operations, err := traceReader.GetOperations(context.Background(), tracestore.OperationQueryParameters{
		ServiceName: "service-a",
		SpanKind:    "server",
	})
	require.NoError(t, err)
	require.Equal(t, []tracestore.Operation{
		{
			Name:     "operation-a",
			SpanKind: "server",
		},
		{
			Name:     "operation-b",
			SpanKind: "server",
		},
	}, operations)
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

func TestTraceReader_FindTraceIDsPanics(t *testing.T) {
	memstore := memory.NewStore()
	traceReader := &TraceReader{
		spanReader: memstore,
	}
	require.Panics(
		t,
		func() { traceReader.FindTraceIDs(context.Background(), tracestore.TraceQueryParameters{}) },
	)
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
