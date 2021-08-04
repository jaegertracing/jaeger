// Copyright (c) 2021 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package apiv3

import (
	"context"
	"fmt"

	"github.com/gogo/protobuf/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/proto-gen/api_v3"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

// Handler implements api_v3.QueryServiceServer
type Handler struct {
	QueryService *querysvc.QueryService
}

var _ api_v3.QueryServiceServer = (*Handler)(nil)

// GetTrace implements api_v3.QueryServiceServer's GetTrace
func (h *Handler) GetTrace(request *api_v3.GetTraceRequest, stream api_v3.QueryService_GetTraceServer) error {
	traceID, err := model.TraceIDFromString(request.GetTraceId())
	if err != nil {
		return err
	}

	trace, err := h.QueryService.GetTrace(stream.Context(), traceID)
	if err != nil {
		return err
	}
	resourceSpans := jaegerSpansToOTLP(trace.GetSpans())
	return stream.Send(&api_v3.SpansResponseChunk{
		ResourceSpans: resourceSpans,
	})
}

// FindTraces implements api_v3.QueryServiceServer's FindTraces
func (h *Handler) FindTraces(request *api_v3.FindTracesRequest, stream api_v3.QueryService_FindTracesServer) error {
	query := request.GetQuery()
	if query == nil {
		return status.Errorf(codes.InvalidArgument, "missing query")
	}
	if query.GetStartTimeMin() == nil ||
		query.GetStartTimeMax() == nil {
		return fmt.Errorf("start time min and max are required parameters")
	}

	queryParams := &spanstore.TraceQueryParameters{
		ServiceName:   query.GetServiceName(),
		OperationName: query.GetOperationName(),
		Tags:          query.GetAttributes(),
		NumTraces:     int(query.GetNumTraces()),
	}
	if query.GetStartTimeMin() != nil {
		startTimeMin, err := types.TimestampFromProto(query.GetStartTimeMin())
		if err != nil {
			return err
		}
		queryParams.StartTimeMin = startTimeMin
	}
	if query.GetStartTimeMax() != nil {
		startTimeMax, err := types.TimestampFromProto(query.GetStartTimeMax())
		if err != nil {
			return err
		}
		queryParams.StartTimeMax = startTimeMax
	}
	if query.GetDurationMin() != nil {
		durationMin, err := types.DurationFromProto(query.GetDurationMin())
		if err != nil {
			return err
		}
		queryParams.DurationMin = durationMin
	}
	if query.GetDurationMax() != nil {
		durationMax, err := types.DurationFromProto(query.GetDurationMax())
		if err != nil {
			return err
		}
		queryParams.DurationMax = durationMax
	}

	traces, err := h.QueryService.FindTraces(stream.Context(), queryParams)
	if err != nil {
		return err
	}
	for _, t := range traces {
		resourceSpans := jaegerSpansToOTLP(t.GetSpans())
		stream.Send(&api_v3.SpansResponseChunk{
			ResourceSpans: resourceSpans,
		})
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
