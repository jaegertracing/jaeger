// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package factoryadapter

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/plugin/storage/memory"
	"github.com/jaegertracing/jaeger/storage_v2/spanstore"
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

func (*fakeReader) GetOperations(_ context.Context, _ spanstore.OperationQueryParameters) ([]spanstore.Operation, error) {
	panic("not implemented")
}

func (*fakeReader) FindTraces(_ context.Context, _ spanstore.TraceQueryParameters) ([]ptrace.Traces, error) {
	panic("not implemented")
}

func (*fakeReader) FindTraceIDs(_ context.Context, _ spanstore.TraceQueryParameters) ([]pcommon.TraceID, error) {
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

func TestTraceReader_GetServicesPanics(t *testing.T) {
	memstore := memory.NewStore()
	traceReader := &TraceReader{
		spanReader: memstore,
	}
	require.Panics(t, func() { traceReader.GetServices(context.Background()) })
}

func TestTraceReader_GetOperationsPanics(t *testing.T) {
	memstore := memory.NewStore()
	traceReader := &TraceReader{
		spanReader: memstore,
	}
	require.Panics(
		t,
		func() { traceReader.GetOperations(context.Background(), spanstore.OperationQueryParameters{}) },
	)
}

func TestTraceReader_FindTracesPanics(t *testing.T) {
	memstore := memory.NewStore()
	traceReader := &TraceReader{
		spanReader: memstore,
	}
	require.Panics(
		t,
		func() { traceReader.FindTraces(context.Background(), spanstore.TraceQueryParameters{}) },
	)
}

func TestTraceReader_FindTraceIDsPanics(t *testing.T) {
	memstore := memory.NewStore()
	traceReader := &TraceReader{
		spanReader: memstore,
	}
	require.Panics(
		t,
		func() { traceReader.FindTraceIDs(context.Background(), spanstore.TraceQueryParameters{}) },
	)
}
