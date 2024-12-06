// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package factoryadapter

import (
	"context"
	"errors"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	"github.com/jaegertracing/jaeger/storage_v2/depstore"
	"github.com/jaegertracing/jaeger/storage_v2/tracestore"
)

var errV1ReaderNotAvailable = errors.New("spanstore.Reader is not a wrapper around v1 reader")

type TraceReader struct {
	spanReader spanstore.Reader
}

func GetV1Reader(reader tracestore.Reader) (spanstore.Reader, error) {
	if tr, ok := reader.(*TraceReader); ok {
		return tr.spanReader, nil
	}
	return nil, errV1ReaderNotAvailable
}

func NewTraceReader(spanReader spanstore.Reader) *TraceReader {
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

func (*TraceReader) GetOperations(_ context.Context, _ tracestore.OperationQueryParameters) ([]tracestore.Operation, error) {
	panic("not implemented")
}

func (*TraceReader) FindTraces(_ context.Context, _ tracestore.TraceQueryParameters) ([]ptrace.Traces, error) {
	panic("not implemented")
}

func (*TraceReader) FindTraceIDs(_ context.Context, _ tracestore.TraceQueryParameters) ([]pcommon.TraceID, error) {
	panic("not implemented")
}

type DependencyReader struct {
	reader dependencystore.Reader
}

func NewDependencyReader(reader dependencystore.Reader) *DependencyReader {
	return &DependencyReader{
		reader: reader,
	}
}

func (dr *DependencyReader) GetDependencies(
	ctx context.Context,
	query depstore.QueryParameters) ([]model.DependencyLink, error) {
	return dr.reader.GetDependencies(ctx, query.EndTime, query.EndTime.Sub(query.StartTime))
}
