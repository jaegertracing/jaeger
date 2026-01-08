// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package querysvc

import (
	"context"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	v2querysvc "github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/querysvc/v2/querysvc"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/v1adapter"
)

// queryServiceV2Adapter wraps v2 QueryService to provide v1 QueryService interface.
type queryServiceV2Adapter struct {
	v2qs *v2querysvc.QueryService
}

// NewQueryServiceV2Adapter creates a v1 QueryService wrapper around v2 QueryService.
func NewQueryServiceV2Adapter(v2qs *v2querysvc.QueryService) *QueryService {
	return &QueryService{
		v2adapter: &queryServiceV2Adapter{v2qs: v2qs},
	}
}

func (a *queryServiceV2Adapter) GetTrace(ctx context.Context, query GetTraceParameters) (*model.Trace, error) {
	params := v2querysvc.GetTraceParams{
		TraceIDs: []tracestore.GetTraceParams{
			{
				TraceID: v1adapter.FromV1TraceID(query.TraceID),
				Start:   query.StartTime,
				End:     query.EndTime,
			},
		},
		RawTraces: query.RawTraces,
	}
	iter := a.v2qs.GetTraces(ctx, params)
	traces, err := v1adapter.V1TracesFromSeq2(iter)
	if err != nil {
		return nil, err
	}
	if len(traces) == 0 {
		return nil, spanstore.ErrTraceNotFound
	}
	return traces[0], nil
}

func (a *queryServiceV2Adapter) GetServices(ctx context.Context) ([]string, error) {
	return a.v2qs.GetServices(ctx)
}

func (a *queryServiceV2Adapter) GetOperations(
	ctx context.Context,
	query spanstore.OperationQueryParameters,
) ([]spanstore.Operation, error) {
	ops, err := a.v2qs.GetOperations(ctx, tracestore.OperationQueryParams{
		ServiceName: query.ServiceName,
		SpanKind:    query.SpanKind,
	})
	if err != nil {
		return nil, err
	}
	result := make([]spanstore.Operation, len(ops))
	for i, op := range ops {
		result[i] = spanstore.Operation{
			Name:     op.Name,
			SpanKind: op.SpanKind,
		}
	}
	return result, nil
}

func (a *queryServiceV2Adapter) FindTraces(ctx context.Context, query *TraceQueryParameters) ([]*model.Trace, error) {
	// Convert tags map to pcommon.Map
	attrs := pcommon.NewMap()
	for k, v := range query.Tags {
		attrs.PutStr(k, v)
	}
	params := v2querysvc.TraceQueryParams{
		TraceQueryParams: tracestore.TraceQueryParams{
			ServiceName:   query.ServiceName,
			OperationName: query.OperationName,
			Attributes:    attrs,
			StartTimeMin:  query.StartTimeMin,
			StartTimeMax:  query.StartTimeMax,
			DurationMin:   query.DurationMin,
			DurationMax:   query.DurationMax,
			SearchDepth:   query.NumTraces,
		},
		RawTraces: query.RawTraces,
	}
	iter := a.v2qs.FindTraces(ctx, params)
	return v1adapter.V1TracesFromSeq2(iter)
}

func (a *queryServiceV2Adapter) ArchiveTrace(ctx context.Context, query spanstore.GetTraceParameters) error {
	params := tracestore.GetTraceParams{
		TraceID: v1adapter.FromV1TraceID(query.TraceID),
		Start:   query.StartTime,
		End:     query.EndTime,
	}
	return a.v2qs.ArchiveTrace(ctx, params)
}

func (a *queryServiceV2Adapter) GetDependencies(ctx context.Context, endTs time.Time, lookback time.Duration) ([]model.DependencyLink, error) {
	return a.v2qs.GetDependencies(ctx, endTs, lookback)
}

func (a *queryServiceV2Adapter) GetCapabilities() StorageCapabilities {
	caps := a.v2qs.GetCapabilities()
	return StorageCapabilities{
		ArchiveStorage: caps.ArchiveStorage,
	}
}
