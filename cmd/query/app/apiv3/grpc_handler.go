// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package apiv3

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc"
	"github.com/jaegertracing/jaeger/internal/proto/api_v3"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

// Handler implements api_v3.QueryServiceServer
type Handler struct {
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

	query := querysvc.GetTraceParameters{
		GetTraceParameters: spanstore.GetTraceParameters{
			TraceID:   traceID,
			StartTime: request.GetStartTime(),
			EndTime:   request.GetEndTime(),
		},
		RawTraces: request.GetRawTraces(),
	}
	trace, err := h.QueryService.GetTrace(stream.Context(), query)
	if err != nil {
		return fmt.Errorf("cannot retrieve trace: %w", err)
	}
	td := modelToOTLP(trace.GetSpans())
	tracesData := api_v3.TracesData(td)
	return stream.Send(&tracesData)
}

// FindTraces implements api_v3.QueryServiceServer's FindTraces
func (h *Handler) FindTraces(request *api_v3.FindTracesRequest, stream api_v3.QueryService_FindTracesServer) error {
	return h.internalFindTraces(stream.Context(), request, stream.Send)
}

// separated for testing
func (h *Handler) internalFindTraces(
	ctx context.Context,
	request *api_v3.FindTracesRequest,
	streamSend func(*api_v3.TracesData) error,
) error {
	query := request.GetQuery()
	if query == nil {
		return status.Error(codes.InvalidArgument, "missing query")
	}
	if query.GetStartTimeMin().IsZero() ||
		query.GetStartTimeMax().IsZero() {
		return errors.New("start time min and max are required parameters")
	}

	queryParams := &querysvc.TraceQueryParameters{
		TraceQueryParameters: spanstore.TraceQueryParameters{
			ServiceName:   query.GetServiceName(),
			OperationName: query.GetOperationName(),
			Tags:          query.GetAttributes(),
			NumTraces:     int(query.GetSearchDepth()),
		},
		RawTraces: query.GetRawTraces(),
	}
	if ts := query.GetStartTimeMin(); !ts.IsZero() {
		queryParams.StartTimeMin = ts
	}
	if ts := query.GetStartTimeMax(); !ts.IsZero() {
		queryParams.StartTimeMax = ts
	}
	if d := query.GetDurationMin(); d != 0 {
		queryParams.DurationMin = d
	}
	if d := query.GetDurationMax(); d != 0 {
		queryParams.DurationMax = d
	}

	traces, err := h.QueryService.FindTraces(ctx, queryParams)
	if err != nil {
		return err
	}
	for _, t := range traces {
		td := modelToOTLP(t.GetSpans())
		tracesData := api_v3.TracesData(td)
		if err := streamSend(&tracesData); err != nil {
			return status.Error(codes.Internal,
				fmt.Sprintf("failed to send response stream chunk to client: %v", err))
		}
	}
	return nil
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

// GetOperations implements api_v3.QueryService's GetOperations
func (h *Handler) GetOperations(ctx context.Context, request *api_v3.GetOperationsRequest) (*api_v3.GetOperationsResponse, error) {
	operations, err := h.QueryService.GetOperations(ctx, spanstore.OperationQueryParameters{
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
