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
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/proto-gen/storage_v1"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

const spanBatchSize = 1000

// grpcServer implements shared.StoragePlugin and reads/writes spans and dependencies
type grpcServer struct {
	Impl        StoragePlugin
	ArchiveImpl ArchiveStoragePlugin
}

// GetDependencies returns all interservice dependencies
func (s *grpcServer) GetDependencies(ctx context.Context, r *storage_v1.GetDependenciesRequest) (*storage_v1.GetDependenciesResponse, error) {
	deps, err := s.Impl.DependencyReader().GetDependencies(ctx, r.EndTime, r.EndTime.Sub(r.StartTime))
	if err != nil {
		return nil, err
	}
	return &storage_v1.GetDependenciesResponse{
		Dependencies: deps,
	}, nil
}

// WriteSpan saves the span
func (s *grpcServer) WriteSpan(ctx context.Context, r *storage_v1.WriteSpanRequest) (*storage_v1.WriteSpanResponse, error) {
	err := s.Impl.SpanWriter().WriteSpan(ctx, r.Span)
	if err != nil {
		return nil, err
	}
	return &storage_v1.WriteSpanResponse{}, nil
}

// GetTrace takes a traceID and streams a Trace associated with that traceID
func (s *grpcServer) GetTrace(r *storage_v1.GetTraceRequest, stream storage_v1.SpanReaderPlugin_GetTraceServer) error {
	trace, err := s.Impl.SpanReader().GetTrace(stream.Context(), r.TraceID)
	if err == spanstore.ErrTraceNotFound {
		return status.Errorf(codes.NotFound, spanstore.ErrTraceNotFound.Error())
	}
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
	services, err := s.Impl.SpanReader().GetServices(ctx)
	if err != nil {
		return nil, err
	}
	return &storage_v1.GetServicesResponse{
		Services: services,
	}, nil
}

// GetOperations returns the operations of a given service
func (s *grpcServer) GetOperations(
	ctx context.Context,
	r *storage_v1.GetOperationsRequest,
) (*storage_v1.GetOperationsResponse, error) {
	operations, err := s.Impl.SpanReader().GetOperations(ctx, spanstore.OperationQueryParameters{
		ServiceName: r.Service,
		SpanKind:    r.SpanKind,
	})
	if err != nil {
		return nil, err
	}
	grpcOperation := make([]*storage_v1.Operation, len(operations))
	for i, operation := range operations {
		grpcOperation[i] = &storage_v1.Operation{
			Name:     operation.Name,
			SpanKind: operation.SpanKind,
		}
	}
	return &storage_v1.GetOperationsResponse{
		Operations: grpcOperation,
	}, nil
}

// FindTraces streams traces that match the traceQuery
func (s *grpcServer) FindTraces(r *storage_v1.FindTracesRequest, stream storage_v1.SpanReaderPlugin_FindTracesServer) error {
	traces, err := s.Impl.SpanReader().FindTraces(stream.Context(), &spanstore.TraceQueryParameters{
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
	traceIDs, err := s.Impl.SpanReader().FindTraceIDs(ctx, &spanstore.TraceQueryParameters{
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
			return fmt.Errorf("grpc plugin failed to send response: %w", err)
		}
	}

	return nil
}

func (s *grpcServer) Capabilities(ctx context.Context, request *storage_v1.CapabilitiesRequest) (*storage_v1.CapabilitiesResponse, error) {
	return &storage_v1.CapabilitiesResponse{
		ArchiveSpanReader: s.ArchiveImpl != nil,
		ArchiveSpanWriter: s.ArchiveImpl != nil,
	}, nil
}

func (s *grpcServer) GetArchiveTrace(r *storage_v1.GetTraceRequest, stream storage_v1.ArchiveSpanReaderPlugin_GetArchiveTraceServer) error {
	if s.ArchiveImpl == nil {
		return status.Error(codes.Unimplemented, "not implemented")
	}
	trace, err := s.ArchiveImpl.ArchiveSpanReader().GetTrace(stream.Context(), r.TraceID)
	if err == spanstore.ErrTraceNotFound {
		return status.Errorf(codes.NotFound, spanstore.ErrTraceNotFound.Error())
	}
	if err != nil {
		return err
	}

	err = s.sendSpans(trace.Spans, stream.Send)
	if err != nil {
		return err
	}

	return nil
}

func (s *grpcServer) WriteArchiveSpan(ctx context.Context, r *storage_v1.WriteSpanRequest) (*storage_v1.WriteSpanResponse, error) {
	if s.ArchiveImpl == nil {
		return nil, status.Error(codes.Unimplemented, "not implemented")
	}
	err := s.ArchiveImpl.ArchiveSpanWriter().WriteSpan(ctx, r.Span)
	if err != nil {
		return nil, err
	}
	return &storage_v1.WriteSpanResponse{}, nil
}
