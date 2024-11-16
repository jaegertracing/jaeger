// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package factoryadapter

import (
	"context"
	"errors"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	spanstore_v1 "github.com/jaegertracing/jaeger/storage/spanstore"
	"github.com/jaegertracing/jaeger/storage_v2/spanstore"
)

var ErrV1ReaderNotAvailable = errors.New("v1 reader is not available")

type TraceReader struct {
	spanReader spanstore_v1.Reader
}

func GetV1Reader(reader spanstore.Reader) (spanstore_v1.Reader, error) {
	if tr, ok := reader.(*TraceReader); ok {
		return tr.spanReader, nil
	}
	return nil, ErrV1ReaderNotAvailable
}

func NewTraceReader(spanReader spanstore_v1.Reader) *TraceReader {
	return &TraceReader{
		spanReader: spanReader,
	}
}

func (*TraceReader) GetTrace(_ context.Context, _ pcommon.TraceID) (ptrace.Traces, error) {
	panic("not implemented")
}

func (*TraceReader) GetServices(_ context.Context) ([]string, error) {
	panic("not implemented")
}

func (*TraceReader) GetOperations(_ context.Context, _ spanstore.OperationQueryParameters) ([]spanstore.Operation, error) {
	panic("not implemented")
}

func (*TraceReader) FindTraces(_ context.Context, _ spanstore.TraceQueryParameters) ([]ptrace.Traces, error) {
	panic("not implemented")
}

func (*TraceReader) FindTraceIDs(_ context.Context, _ spanstore.TraceQueryParameters) ([]pcommon.TraceID, error) {
	panic("not implemented")
}
