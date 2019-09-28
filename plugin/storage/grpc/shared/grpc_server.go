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

package shared

import (
	"context"

	"github.com/pkg/errors"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/proto-gen/storage_v1"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

const spanBatchSize = 1000

// grpcServer implements shared.StoragePlugin and reads/writes spans and dependencies
type grpcServer struct {
	Impl StoragePlugin
}

// getMaybeBearerTokenContext returns a context with bearer token, if token string is not empty
// the assumption is that if the token has arrived on the wire, the grpcClient
// verified that bearer token forward was enabled.
// If token is empty, returns original context.
func (s *grpcServer) getMaybeBearerTokenContext(ctx context.Context, token string) context.Context {
	if token != "" {
		return spanstore.ContextWithBearerToken(ctx, token)
	}
	return ctx
}

// GetDependencies returns all interservice dependencies
func (s *grpcServer) GetDependencies(ctx context.Context, r *storage_v1.GetDependenciesRequest) (*storage_v1.GetDependenciesResponse, error) {
	deps, err := s.Impl.DependencyReader().GetDependencies(r.EndTime, r.EndTime.Sub(r.StartTime))
	if err != nil {
		return nil, err
	}
	return &storage_v1.GetDependenciesResponse{
		Dependencies: deps,
	}, nil
}

// WriteSpan saves the span
func (s *grpcServer) WriteSpan(ctx context.Context, r *storage_v1.WriteSpanRequest) (*storage_v1.WriteSpanResponse, error) {
	err := s.Impl.SpanWriter().WriteSpan(r.Span)
	if err != nil {
		return nil, err
	}
	return &storage_v1.WriteSpanResponse{}, nil
}

// GetTrace takes a traceID and streams a Trace associated with that traceID
func (s *grpcServer) GetTrace(r *storage_v1.GetTraceRequest, stream storage_v1.SpanReaderPlugin_GetTraceServer) error {
	opCtx := s.getMaybeBearerTokenContext(stream.Context(), r.BearerToken)
	trace, err := s.Impl.SpanReader().GetTrace(opCtx, r.TraceID)
	if err != nil {
		return err
	}

	err = s.sendSpans(trace.Spans, stream.Send)
	if err != nil {
		return err
	}

	return nil
}

// GetServices returns a list of all known services
func (s *grpcServer) GetServices(ctx context.Context, r *storage_v1.GetServicesRequest) (*storage_v1.GetServicesResponse, error) {
	opCtx := s.getMaybeBearerTokenContext(ctx, r.BearerToken)
	services, err := s.Impl.SpanReader().GetServices(opCtx)
	if err != nil {
		return nil, err
	}
	return &storage_v1.GetServicesResponse{
		Services: services,
	}, nil
}

// GetOperations returns the operations of a given service
func (s *grpcServer) GetOperations(ctx context.Context, r *storage_v1.GetOperationsRequest) (*storage_v1.GetOperationsResponse, error) {
	opCtx := s.getMaybeBearerTokenContext(ctx, r.BearerToken)
	operations, err := s.Impl.SpanReader().GetOperations(opCtx, r.Service)
	if err != nil {
		return nil, err
	}
	return &storage_v1.GetOperationsResponse{
		Operations: operations,
	}, nil
}

// FindTraces streams traces that match the traceQuery
func (s *grpcServer) FindTraces(r *storage_v1.FindTracesRequest, stream storage_v1.SpanReaderPlugin_FindTracesServer) error {
	opCtx := s.getMaybeBearerTokenContext(stream.Context(), r.BearerToken)
	traces, err := s.Impl.SpanReader().FindTraces(opCtx, &spanstore.TraceQueryParameters{
		ServiceName:   r.Query.ServiceName,
		OperationName: r.Query.OperationName,
		Tags:          r.Query.Tags,
		StartTimeMin:  r.Query.StartTimeMin,
		StartTimeMax:  r.Query.StartTimeMax,
		DurationMin:   r.Query.DurationMin,
		DurationMax:   r.Query.DurationMax,
		NumTraces:     int(r.Query.NumTraces),
	})
	if err != nil {
		return err
	}

	for _, trace := range traces {
		err = s.sendSpans(trace.Spans, stream.Send)
		if err != nil {
			return err
		}
	}

	return nil
}

// FindTraceIDs retrieves traceIDs that match the traceQuery
func (s *grpcServer) FindTraceIDs(ctx context.Context, r *storage_v1.FindTraceIDsRequest) (*storage_v1.FindTraceIDsResponse, error) {
	opCtx := s.getMaybeBearerTokenContext(ctx, r.BearerToken)
	traceIDs, err := s.Impl.SpanReader().FindTraceIDs(opCtx, &spanstore.TraceQueryParameters{
		ServiceName:   r.Query.ServiceName,
		OperationName: r.Query.OperationName,
		Tags:          r.Query.Tags,
		StartTimeMin:  r.Query.StartTimeMin,
		StartTimeMax:  r.Query.StartTimeMax,
		DurationMin:   r.Query.DurationMin,
		DurationMax:   r.Query.DurationMax,
		NumTraces:     int(r.Query.NumTraces),
	})
	if err != nil {
		return nil, err
	}
	return &storage_v1.FindTraceIDsResponse{
		TraceIDs: traceIDs,
	}, nil
}

func (s *grpcServer) sendSpans(spans []*model.Span, sendFn func(*storage_v1.SpansResponseChunk) error) error {
	chunk := make([]model.Span, 0, len(spans))
	for i := 0; i < len(spans); i += spanBatchSize {
		chunk = chunk[:0]
		for j := i; j < len(spans) && j < i+spanBatchSize; j++ {
			chunk = append(chunk, *spans[j])
		}
		if err := sendFn(&storage_v1.SpansResponseChunk{Spans: chunk}); err != nil {
			return errors.Wrap(err, "grpc plugin failed to send response")
		}
	}

	return nil
}
