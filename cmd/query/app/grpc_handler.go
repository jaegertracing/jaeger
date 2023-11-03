// Copyright (c) 2019 The Jaeger Authors.
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

package app

import (
	"context"
	"errors"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc"
	"github.com/jaegertracing/jaeger/model"
	_ "github.com/jaegertracing/jaeger/pkg/gogocodec" // force gogo codec registration
	"github.com/jaegertracing/jaeger/pkg/jtracer"
	"github.com/jaegertracing/jaeger/plugin/metrics/disabled"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2/metrics"
	"github.com/jaegertracing/jaeger/storage/metricsstore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

const (
	maxSpanCountInChunk = 10

	msgTraceNotFound = "trace not found"
)

var (
	errGRPCMetricsQueryDisabled = status.Error(codes.Unimplemented, "metrics querying is currently disabled")
	errNilRequest               = status.Error(codes.InvalidArgument, "a nil argument is not allowed")
	errUninitializedTraceID     = status.Error(codes.InvalidArgument, "uninitialized TraceID is not allowed")
	errMissingServiceNames      = status.Error(codes.InvalidArgument, "please provide at least one service name")
	errMissingQuantile          = status.Error(codes.InvalidArgument, "please provide a quantile between (0, 1]")
)

// GRPCHandler implements the gRPC endpoint of the query service.
type GRPCHandler struct {
	queryService        *querysvc.QueryService
	metricsQueryService querysvc.MetricsQueryService
	logger              *zap.Logger
	tracer              *jtracer.JTracer
	nowFn               func() time.Time
}

// GRPCHandlerOptions contains optional members of GRPCHandler.
type GRPCHandlerOptions struct {
	Logger *zap.Logger
	Tracer *jtracer.JTracer
	NowFn  func() time.Time
}

// NewGRPCHandler returns a GRPCHandler.
func NewGRPCHandler(queryService *querysvc.QueryService,
	metricsQueryService querysvc.MetricsQueryService,
	options GRPCHandlerOptions,
) *GRPCHandler {
	if options.Logger == nil {
		options.Logger = zap.NewNop()
	}

	if options.Tracer == nil {
		options.Tracer = jtracer.NoOp()
	}

	if options.NowFn == nil {
		options.NowFn = time.Now
	}

	return &GRPCHandler{
		queryService:        queryService,
		metricsQueryService: metricsQueryService,
		logger:              options.Logger,
		tracer:              options.Tracer,
		nowFn:               options.NowFn,
	}
}

var _ api_v2.QueryServiceServer = (*GRPCHandler)(nil)

// GetTrace is the gRPC handler to fetch traces based on trace-id.
func (g *GRPCHandler) GetTrace(r *api_v2.GetTraceRequest, stream api_v2.QueryService_GetTraceServer) error {
	if r == nil {
		return errNilRequest
	}
	if r.TraceID == (model.TraceID{}) {
		return errUninitializedTraceID
	}
	trace, err := g.queryService.GetTrace(stream.Context(), r.TraceID)
	if err == spanstore.ErrTraceNotFound {
		g.logger.Error(msgTraceNotFound, zap.Error(err))
		return status.Errorf(codes.NotFound, "%s: %v", msgTraceNotFound, err)
	}
	if err != nil {
		g.logger.Error("failed to fetch spans from the backend", zap.Error(err))
		return status.Errorf(codes.Internal, "failed to fetch spans from the backend: %v", err)
	}
	return g.sendSpanChunks(trace.Spans, stream.Send)
}

// ArchiveTrace is the gRPC handler to archive traces.
func (g *GRPCHandler) ArchiveTrace(ctx context.Context, r *api_v2.ArchiveTraceRequest) (*api_v2.ArchiveTraceResponse, error) {
	if r == nil {
		return nil, errNilRequest
	}
	if r.TraceID == (model.TraceID{}) {
		return nil, errUninitializedTraceID
	}
	err := g.queryService.ArchiveTrace(ctx, r.TraceID)
	if err == spanstore.ErrTraceNotFound {
		g.logger.Error("trace not found", zap.Error(err))
		return nil, status.Errorf(codes.NotFound, "%s: %v", msgTraceNotFound, err)
	}
	if err != nil {
		g.logger.Error("failed to archive trace", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to archive trace: %v", err)
	}

	return &api_v2.ArchiveTraceResponse{}, nil
}

// FindTraces is the gRPC handler to fetch traces based on TraceQueryParameters.
func (g *GRPCHandler) FindTraces(r *api_v2.FindTracesRequest, stream api_v2.QueryService_FindTracesServer) error {
	if r == nil {
		return errNilRequest
	}
	query := r.GetQuery()
	if query == nil {
		return status.Errorf(codes.InvalidArgument, "missing query")
	}
	queryParams := spanstore.TraceQueryParameters{
		ServiceName:   query.ServiceName,
		OperationName: query.OperationName,
		Tags:          query.Tags,
		StartTimeMin:  query.StartTimeMin,
		StartTimeMax:  query.StartTimeMax,
		DurationMin:   query.DurationMin,
		DurationMax:   query.DurationMax,
		NumTraces:     int(query.SearchDepth),
	}
	traces, err := g.queryService.FindTraces(stream.Context(), &queryParams)
	if err != nil {
		g.logger.Error("failed when searching for traces", zap.Error(err))
		return status.Errorf(codes.Internal, "failed when searching for traces: %v", err)
	}
	for _, trace := range traces {
		if err := g.sendSpanChunks(trace.Spans, stream.Send); err != nil {
			return err
		}
	}
	return nil
}

func (g *GRPCHandler) sendSpanChunks(spans []*model.Span, sendFn func(*api_v2.SpansResponseChunk) error) error {
	chunk := make([]model.Span, 0, len(spans))
	for i := 0; i < len(spans); i += maxSpanCountInChunk {
		chunk = chunk[:0]
		for j := i; j < len(spans) && j < i+maxSpanCountInChunk; j++ {
			chunk = append(chunk, *spans[j])
		}
		if err := sendFn(&api_v2.SpansResponseChunk{Spans: chunk}); err != nil {
			g.logger.Error("failed to send response to client", zap.Error(err))
			return err
		}
	}
	return nil
}

// GetServices is the gRPC handler to fetch services.
func (g *GRPCHandler) GetServices(ctx context.Context, r *api_v2.GetServicesRequest) (*api_v2.GetServicesResponse, error) {
	services, err := g.queryService.GetServices(ctx)
	if err != nil {
		g.logger.Error("failed to fetch services", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to fetch services: %v", err)
	}

	return &api_v2.GetServicesResponse{Services: services}, nil
}

// GetOperations is the gRPC handler to fetch operations.
func (g *GRPCHandler) GetOperations(
	ctx context.Context,
	r *api_v2.GetOperationsRequest,
) (*api_v2.GetOperationsResponse, error) {
	if r == nil {
		return nil, errNilRequest
	}
	operations, err := g.queryService.GetOperations(ctx, spanstore.OperationQueryParameters{
		ServiceName: r.Service,
		SpanKind:    r.SpanKind,
	})
	if err != nil {
		g.logger.Error("failed to fetch operations", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to fetch operations: %v", err)
	}

	result := make([]*api_v2.Operation, len(operations))
	for i, operation := range operations {
		result[i] = &api_v2.Operation{
			Name:     operation.Name,
			SpanKind: operation.SpanKind,
		}
	}
	return &api_v2.GetOperationsResponse{
		Operations: result,
		// TODO: remove OperationNames after all clients are updated
		OperationNames: getUniqueOperationNames(operations),
	}, nil
}

// GetDependencies is the gRPC handler to fetch dependencies.
func (g *GRPCHandler) GetDependencies(ctx context.Context, r *api_v2.GetDependenciesRequest) (*api_v2.GetDependenciesResponse, error) {
	if r == nil {
		return nil, errNilRequest
	}

	startTime := r.StartTime
	endTime := r.EndTime
	if startTime == (time.Time{}) || endTime == (time.Time{}) {
		return nil, status.Errorf(codes.InvalidArgument, "StartTime and EndTime must be initialized.")
	}

	dependencies, err := g.queryService.GetDependencies(ctx, startTime, endTime.Sub(startTime))
	if err != nil {
		g.logger.Error("failed to fetch dependencies", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to fetch dependencies: %v", err)
	}

	return &api_v2.GetDependenciesResponse{Dependencies: dependencies}, nil
}

// GetLatencies is the gRPC handler to fetch latency metrics.
func (g *GRPCHandler) GetLatencies(ctx context.Context, r *metrics.GetLatenciesRequest) (*metrics.GetMetricsResponse, error) {
	bqp, err := g.newBaseQueryParameters(r)
	if err := g.handleErr("failed to build parameters", err); err != nil {
		return nil, err
	}
	// Check for cases where clients do not provide the Quantile, which defaults to the float64's zero value.
	if r.Quantile == 0 {
		return nil, errMissingQuantile
	}
	queryParams := metricsstore.LatenciesQueryParameters{
		BaseQueryParameters: bqp,
		Quantile:            r.Quantile,
	}
	m, err := g.metricsQueryService.GetLatencies(ctx, &queryParams)
	if err := g.handleErr("failed to fetch latencies", err); err != nil {
		return nil, err
	}
	return &metrics.GetMetricsResponse{Metrics: *m}, nil
}

// GetCallRates is the gRPC handler to fetch call rate metrics.
func (g *GRPCHandler) GetCallRates(ctx context.Context, r *metrics.GetCallRatesRequest) (*metrics.GetMetricsResponse, error) {
	bqp, err := g.newBaseQueryParameters(r)
	if err := g.handleErr("failed to build parameters", err); err != nil {
		return nil, err
	}
	queryParams := metricsstore.CallRateQueryParameters{
		BaseQueryParameters: bqp,
	}
	m, err := g.metricsQueryService.GetCallRates(ctx, &queryParams)
	if err := g.handleErr("failed to fetch call rates", err); err != nil {
		return nil, err
	}
	return &metrics.GetMetricsResponse{Metrics: *m}, nil
}

// GetErrorRates is the gRPC handler to fetch error rate metrics.
func (g *GRPCHandler) GetErrorRates(ctx context.Context, r *metrics.GetErrorRatesRequest) (*metrics.GetMetricsResponse, error) {
	bqp, err := g.newBaseQueryParameters(r)
	if err := g.handleErr("failed to build parameters", err); err != nil {
		return nil, err
	}
	queryParams := metricsstore.ErrorRateQueryParameters{
		BaseQueryParameters: bqp,
	}
	m, err := g.metricsQueryService.GetErrorRates(ctx, &queryParams)
	if err := g.handleErr("failed to fetch error rates", err); err != nil {
		return nil, err
	}
	return &metrics.GetMetricsResponse{Metrics: *m}, nil
}

// GetMinStepDuration is the gRPC handler to fetch the minimum step duration supported by the underlying metrics store.
func (g *GRPCHandler) GetMinStepDuration(ctx context.Context, _ *metrics.GetMinStepDurationRequest) (*metrics.GetMinStepDurationResponse, error) {
	minStep, err := g.metricsQueryService.GetMinStepDuration(ctx, &metricsstore.MinStepDurationQueryParameters{})
	if err := g.handleErr("failed to fetch min step duration", err); err != nil {
		return nil, err
	}
	return &metrics.GetMinStepDurationResponse{MinStep: minStep}, nil
}

func (g *GRPCHandler) handleErr(msg string, err error) error {
	if err == nil {
		return nil
	}
	g.logger.Error(msg, zap.Error(err))

	// Avoid wrapping "expected" errors with an "Internal Server" error.
	if errors.Is(err, disabled.ErrDisabled) {
		return errGRPCMetricsQueryDisabled
	}
	if _, ok := status.FromError(err); ok {
		return err
	}

	// Received an "unexpected" error.
	return status.Errorf(codes.Internal, "%s: %v", msg, err)
}

func (g *GRPCHandler) newBaseQueryParameters(r interface{}) (bqp metricsstore.BaseQueryParameters, err error) {
	if r == nil {
		return bqp, errNilRequest
	}
	var baseRequest *metrics.MetricsQueryBaseRequest
	switch v := r.(type) {
	case *metrics.GetLatenciesRequest:
		baseRequest = v.BaseRequest
	case *metrics.GetCallRatesRequest:
		baseRequest = v.BaseRequest
	case *metrics.GetErrorRatesRequest:
		baseRequest = v.BaseRequest
	}
	if baseRequest == nil || len(baseRequest.ServiceNames) == 0 {
		return bqp, errMissingServiceNames
	}

	// Copy non-nullable params.
	bqp.GroupByOperation = baseRequest.GroupByOperation
	bqp.ServiceNames = baseRequest.ServiceNames

	// Initialize nullable params with defaults.
	defaultEndTime := g.nowFn()
	bqp.EndTime = &defaultEndTime
	bqp.Lookback = &defaultMetricsQueryLookbackDuration
	bqp.RatePer = &defaultMetricsQueryRateDuration
	bqp.SpanKinds = defaultMetricsSpanKinds
	bqp.Step = &defaultMetricsQueryStepDuration

	// ... and override defaults with any provided request params.
	if baseRequest.EndTime != nil {
		bqp.EndTime = baseRequest.EndTime
	}
	if baseRequest.Lookback != nil {
		bqp.Lookback = baseRequest.Lookback
	}
	if baseRequest.Step != nil {
		bqp.Step = baseRequest.Step
	}
	if baseRequest.RatePer != nil {
		bqp.RatePer = baseRequest.RatePer
	}
	if len(baseRequest.SpanKinds) > 0 {
		spanKinds := make([]string, len(baseRequest.SpanKinds))
		for i, v := range baseRequest.SpanKinds {
			spanKinds[i] = v.String()
		}
		bqp.SpanKinds = spanKinds
	}
	return bqp, nil
}
