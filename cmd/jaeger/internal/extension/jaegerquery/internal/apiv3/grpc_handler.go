// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package apiv3

import (
	"context"
	"fmt"
	"iter"

	"go.opentelemetry.io/collector/pdata/ptrace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/proto/api_v3"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/v1adapter"
)

// Handler implements api_v3.QueryServiceServer
type Handler struct {
	api_v3.UnimplementedQueryServiceServer
	QueryService *querysvc.QueryService
}

// remove me
var _ api_v3.QueryServiceServer = (*Handler)(nil)

// GetTrace implements api_v3.QueryServiceServer's GetTrace
func (h *Handler) GetTrace(request *api_v3.GetTraceRequest, stream api_v3.QueryService_GetTraceServer) error {
	traceID, err := model.TraceIDFromString(request.GetTraceId())
	if err != nil {
		return fmt.Errorf("malform trace ID: %w", err)
	}

	query := querysvc.GetTraceParams{
		TraceIDs: []tracestore.GetTraceParams{
			{
				TraceID: v1adapter.FromV1TraceID(traceID),
				Start:   request.GetStartTime(),
				End:     request.GetEndTime(),
			},
		},
		RawTraces: request.GetRawTraces(),
	}
	getTracesIter := h.QueryService.GetTraces(stream.Context(), query)
	return receiveTraces(getTracesIter, stream.Send)
}

// FindTraces implements api_v3.QueryServiceServer's FindTraces
func (h *Handler) FindTraces(request *api_v3.FindTracesRequest, stream api_v3.QueryService_FindTracesServer) error {
	return h.internalFindTraces(stream.Context(), request, stream.Send)
}

// separated for testing
func (h *Handler) internalFindTraces(
	ctx context.Context,
	request *api_v3.FindTracesRequest,
	streamSend func(*jptrace.TracesData) error,
) error {
	queryParams, err := traceQueryParams(request.GetQuery())
	if err != nil {
		return err
	}
	queryParams.RawTraces = request.GetQuery().GetRawTraces()
	findTracesIter := h.QueryService.FindTraces(ctx, queryParams)
	return receiveTraces(findTracesIter, streamSend)
}

// traceQueryParams converts a proto TraceQueryParameters to querysvc.TraceQueryParams,
// validating that the required time range fields are present.
func traceQueryParams(query *api_v3.TraceQueryParameters) (querysvc.TraceQueryParams, error) {
	if query == nil {
		return querysvc.TraceQueryParams{}, status.Error(codes.InvalidArgument, "missing query")
	}
	if query.GetStartTimeMin().IsZero() || query.GetStartTimeMax().IsZero() {
		return querysvc.TraceQueryParams{}, status.Error(codes.InvalidArgument, "start time min and max are required parameters")
	}
	queryParams := querysvc.TraceQueryParams{
		TraceQueryParams: tracestore.TraceQueryParams{
			ServiceName:   query.GetServiceName(),
			OperationName: query.GetOperationName(),
			Attributes:    jptrace.PlainMapToPcommonMap(query.GetAttributes()),
			SearchDepth:   int(query.GetSearchDepth()),
			StartTimeMin:  query.GetStartTimeMin(),
			StartTimeMax:  query.GetStartTimeMax(),
			DurationMin:   query.GetDurationMin(),
			DurationMax:   query.GetDurationMax(),
		},
	}
	return queryParams, nil
}

// FindTraceSummaries implements api_v3.QueryServiceServer's FindTraceSummaries
func (h *Handler) FindTraceSummaries(request *api_v3.FindTraceSummariesRequest, stream api_v3.QueryService_FindTraceSummariesServer) error {
	queryParams, err := traceQueryParams(request.GetQuery())
	if err != nil {
		return err
	}

	for summaries, err := range h.QueryService.FindTraceSummaries(stream.Context(), queryParams) {
		if err != nil {
			return err
		}
		if err := stream.Send(&api_v3.FindTraceSummariesResponse{Summaries: toProtoTraceSummaries(summaries)}); err != nil {
			return status.Errorf(codes.Internal, "failed to send response stream chunk to client: %v", err)
		}
	}
	return nil
}

func toProtoTraceSummaries(summaries []tracestore.TraceSummary) []*api_v3.TraceSummary {
	out := make([]*api_v3.TraceSummary, len(summaries))
	for i := range summaries {
		s := &summaries[i]
		svcs := make([]*api_v3.ServiceSummary, len(s.Services))
		for j := range s.Services {
			ss := &s.Services[j]
			svcs[j] = &api_v3.ServiceSummary{
				Name:           ss.Name,
				SpanCount:      int32(ss.SpanCount),      //nolint:gosec // G115
				ErrorSpanCount: int32(ss.ErrorSpanCount), //nolint:gosec // G115
			}
		}
		out[i] = &api_v3.TraceSummary{
			TraceId:              s.TraceID.String(),
			RootServiceName:      s.RootServiceName,
			RootOperationName:    s.RootOperationName,
			MinStartTimeUnixNano: jptrace.TimeToUnixNano(s.MinStartTime),
			MaxEndTimeUnixNano:   jptrace.TimeToUnixNano(s.MaxEndTime),
			SpanCount:            int32(s.SpanCount),       //nolint:gosec // G115
			ErrorSpanCount:       int32(s.ErrorSpanCount),  //nolint:gosec // G115
			OrphanSpanCount:      int32(s.OrphanSpanCount), //nolint:gosec // G115
			Services:             svcs,
		}
	}
	return out
}

// GetServices implements api_v3.QueryServiceServer's GetServices
func (h *Handler) GetServices(ctx context.Context, _ *api_v3.GetServicesRequest) (*api_v3.GetServicesResponse, error) {
	services, err := h.QueryService.GetServices(ctx)
	if err != nil {
		return nil, err
	}
	return &api_v3.GetServicesResponse{
		Services: services,
	}, nil
}

// GetDependencies implements api_v3.QueryServiceServer's GetDependencies
func (h *Handler) GetDependencies(ctx context.Context, request *api_v3.GetDependenciesRequest) (*api_v3.DependenciesResponse, error) {
	startTime := request.GetStartTime()
	endTime := request.GetEndTime()
	if startTime.IsZero() || endTime.IsZero() {
		return nil, status.Error(codes.InvalidArgument, "start_time and end_time are required")
	}
	if !endTime.After(startTime) {
		return nil, status.Error(codes.InvalidArgument, "end_time must be after start_time")
	}
	deps, err := h.QueryService.GetDependencies(ctx, endTime, endTime.Sub(startTime))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to fetch dependencies: %v", err)
	}
	return &api_v3.DependenciesResponse{Dependencies: toAPIDependencies(deps)}, nil
}

func toAPIDependencies(deps []model.DependencyLink) []*api_v3.Dependency {
	apiDeps := make([]*api_v3.Dependency, len(deps))
	for i, d := range deps {
		apiDeps[i] = &api_v3.Dependency{
			Parent:    d.Parent,
			Child:     d.Child,
			CallCount: uint64(d.CallCount),
		}
	}
	return apiDeps
}

// GetOperations implements api_v3.QueryService's GetOperations
func (h *Handler) GetOperations(ctx context.Context, request *api_v3.GetOperationsRequest) (*api_v3.GetOperationsResponse, error) {
	operations, err := h.QueryService.GetOperations(ctx, tracestore.OperationQueryParams{
		ServiceName: request.GetService(),
		SpanKind:    request.GetSpanKind(),
	})
	if err != nil {
		return nil, err
	}
	apiOperations := make([]*api_v3.Operation, len(operations))
	for i := range operations {
		apiOperations[i] = &api_v3.Operation{
			Name:     operations[i].Name,
			SpanKind: operations[i].SpanKind,
		}
	}
	return &api_v3.GetOperationsResponse{
		Operations: apiOperations,
	}, nil
}

func receiveTraces(
	seq iter.Seq2[[]ptrace.Traces, error],
	sendFn func(*jptrace.TracesData) error,
) error {
	for traces, err := range seq {
		if err != nil {
			return err
		}
		for _, trace := range traces {
			tracesData := jptrace.TracesData(trace)
			if err := sendFn(&tracesData); err != nil {
				return status.Error(codes.Internal,
					fmt.Sprintf("failed to send response stream chunk to client: %v", err))
			}
		}
	}
	return nil
}
