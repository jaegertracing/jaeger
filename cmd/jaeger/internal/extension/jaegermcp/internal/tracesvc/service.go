// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracesvc

import (
	"context"
	"iter"

	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// TraceService provides a wrapper around the Jaeger storage readers for MCP tools.
// It abstracts the underlying storage implementations and provides a unified
// interface for trace operations needed by MCP tools.
type TraceService struct {
	v1SpanReader  spanstore.Reader
	v2TraceReader tracestore.Reader
	depReader     depstore.Reader
}

// NewTraceService creates a new TraceService instance.
func NewTraceService(v1SpanReader spanstore.Reader, v2TraceReader tracestore.Reader, depReader depstore.Reader) *TraceService {
	return &TraceService{
		v1SpanReader:  v1SpanReader,
		v2TraceReader: v2TraceReader,
		depReader:     depReader,
	}
}

// GetServices returns the list of services from the trace storage.
func (ts *TraceService) GetServices(ctx context.Context) ([]string, error) {
	return ts.v1SpanReader.GetServices(ctx)
}

// GetOperations returns the list of operations for a given service.
func (ts *TraceService) GetOperations(ctx context.Context, params spanstore.OperationQueryParameters) ([]spanstore.Operation, error) {
	return ts.v1SpanReader.GetOperations(ctx, params)
}

// FindTraces searches for traces matching the given query parameters.
func (ts *TraceService) FindTraces(ctx context.Context, params *spanstore.TraceQueryParameters) ([]*model.Trace, error) {
	return ts.v1SpanReader.FindTraces(ctx, params)
}

// GetTrace retrieves a single trace by its ID.
func (ts *TraceService) GetTrace(ctx context.Context, params spanstore.GetTraceParameters) (*model.Trace, error) {
	return ts.v1SpanReader.GetTrace(ctx, params)
}

// GetTracesV2 retrieves traces using the v2 API.
func (ts *TraceService) GetTracesV2(ctx context.Context, traceIDs ...tracestore.GetTraceParams) iter.Seq2[[]ptrace.Traces, error] {
	return ts.v2TraceReader.GetTraces(ctx, traceIDs...)
}

// FindTracesV2 searches for traces using the v2 API.
func (ts *TraceService) FindTracesV2(ctx context.Context, params tracestore.TraceQueryParams) iter.Seq2[[]ptrace.Traces, error] {
	return ts.v2TraceReader.FindTraces(ctx, params)
}

// GetDependencies retrieves service dependencies.
func (ts *TraceService) GetDependencies(ctx context.Context, params depstore.QueryParameters) ([]model.DependencyLink, error) {
	return ts.depReader.GetDependencies(ctx, params)
}
