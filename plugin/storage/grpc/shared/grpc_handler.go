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
	"io"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/proto-gen/storage_v1"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

const spanBatchSize = 1000

// GRPCHandler implements all methods of Remote Storage gRPC API.
type GRPCHandler struct {
	// impl        StoragePlugin
	// ArchiveImpl ArchiveStoragePlugin
	// StreamImpl  StreamingSpanWriterPlugin
	impl *GRPCHandlerStorageImpl
}

// GRPCHandlerStorageImpl contains accessors for various storage implementations needed by the handler.
type GRPCHandlerStorageImpl struct {
	SpanReader       func() spanstore.Reader
	SpanWriter       func() spanstore.Writer
	DependencyReader func() dependencystore.Reader

	ArchiveSpanReader func() spanstore.Reader
	ArchiveSpanWriter func() spanstore.Writer

	StreamingSpanWriter func() spanstore.Writer
}

// NewGRPCHandler creates a handler given individual storage implementations.
func NewGRPCHandler(impl *GRPCHandlerStorageImpl) *GRPCHandler {
	return &GRPCHandler{impl: impl}
}

// NewGRPCHandler creates a handler given implementations grouped by plugin services.
func NewGRPCHandlerWithPlugins(
	mainImpl StoragePlugin,
	archiveImpl ArchiveStoragePlugin,
	streamImpl StreamingSpanWriterPlugin,
) *GRPCHandler {
	impl := &GRPCHandlerStorageImpl{
		SpanReader:       mainImpl.SpanReader,
		SpanWriter:       mainImpl.SpanWriter,
		DependencyReader: mainImpl.DependencyReader,

		ArchiveSpanReader:   func() spanstore.Reader { return nil },
		ArchiveSpanWriter:   func() spanstore.Writer { return nil },
		StreamingSpanWriter: func() spanstore.Writer { return nil },
	}
	if archiveImpl != nil {
		impl.ArchiveSpanReader = archiveImpl.ArchiveSpanReader
		impl.ArchiveSpanWriter = archiveImpl.ArchiveSpanWriter
	}
	if streamImpl != nil {
		impl.StreamingSpanWriter = streamImpl.StreamingSpanWriter
	}
	return NewGRPCHandler(impl)
}

// Register registers the server as gRPC methods handler.
func (s *GRPCHandler) Register(ss *grpc.Server) error {
	storage_v1.RegisterSpanReaderPluginServer(ss, s)
	storage_v1.RegisterSpanWriterPluginServer(ss, s)
	storage_v1.RegisterArchiveSpanReaderPluginServer(ss, s)
	storage_v1.RegisterArchiveSpanWriterPluginServer(ss, s)
	storage_v1.RegisterPluginCapabilitiesServer(ss, s)
	storage_v1.RegisterDependenciesReaderPluginServer(ss, s)
	storage_v1.RegisterStreamingSpanWriterPluginServer(ss, s)
	return nil
}

// GetDependencies returns all interservice dependencies
func (s *GRPCHandler) GetDependencies(ctx context.Context, r *storage_v1.GetDependenciesRequest) (*storage_v1.GetDependenciesResponse, error) {
	deps, err := s.impl.DependencyReader().GetDependencies(ctx, r.EndTime, r.EndTime.Sub(r.StartTime))
	if err != nil {
		return nil, err
	}
	return &storage_v1.GetDependenciesResponse{
		Dependencies: deps,
	}, nil
}

// WriteSpanStream receive the span from stream and save it
func (s *GRPCHandler) WriteSpanStream(stream storage_v1.StreamingSpanWriterPlugin_WriteSpanStreamServer) error {
	writer := s.impl.StreamingSpanWriter()
	if writer == nil {
		return status.Error(codes.Unimplemented, "not implemented")
	}
	for {
		in, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		err = writer.WriteSpan(stream.Context(), in.Span)
		if err != nil {
			return err
		}
	}
	return stream.SendAndClose(&storage_v1.WriteSpanResponse{})
}

// WriteSpan saves the span
func (s *GRPCHandler) WriteSpan(ctx context.Context, r *storage_v1.WriteSpanRequest) (*storage_v1.WriteSpanResponse, error) {
	err := s.impl.SpanWriter().WriteSpan(ctx, r.Span)
	if err != nil {
		return nil, err
	}
	return &storage_v1.WriteSpanResponse{}, nil
}

func (s *GRPCHandler) Close(ctx context.Context, r *storage_v1.CloseWriterRequest) (*storage_v1.CloseWriterResponse, error) {
	if closer, ok := s.impl.SpanWriter().(io.Closer); ok {
		if err := closer.Close(); err != nil {
			return nil, err
		}

		return &storage_v1.CloseWriterResponse{}, nil
	} else {
		return nil, status.Error(codes.Unimplemented, "span writer does not support graceful shutdown")
	}
}

// GetTrace takes a traceID and streams a Trace associated with that traceID
func (s *GRPCHandler) GetTrace(r *storage_v1.GetTraceRequest, stream storage_v1.SpanReaderPlugin_GetTraceServer) error {
	trace, err := s.impl.SpanReader().GetTrace(stream.Context(), r.TraceID)
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
func (s *GRPCHandler) GetServices(ctx context.Context, r *storage_v1.GetServicesRequest) (*storage_v1.GetServicesResponse, error) {
	services, err := s.impl.SpanReader().GetServices(ctx)
	if err != nil {
		return nil, err
	}
	return &storage_v1.GetServicesResponse{
		Services: services,
	}, nil
}

// GetOperations returns the operations of a given service
func (s *GRPCHandler) GetOperations(
	ctx context.Context,
	r *storage_v1.GetOperationsRequest,
) (*storage_v1.GetOperationsResponse, error) {
	operations, err := s.impl.SpanReader().GetOperations(ctx, spanstore.OperationQueryParameters{
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
func (s *GRPCHandler) FindTraces(r *storage_v1.FindTracesRequest, stream storage_v1.SpanReaderPlugin_FindTracesServer) error {
	traces, err := s.impl.SpanReader().FindTraces(stream.Context(), &spanstore.TraceQueryParameters{
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
func (s *GRPCHandler) FindTraceIDs(ctx context.Context, r *storage_v1.FindTraceIDsRequest) (*storage_v1.FindTraceIDsResponse, error) {
	traceIDs, err := s.impl.SpanReader().FindTraceIDs(ctx, &spanstore.TraceQueryParameters{
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

func (s *GRPCHandler) sendSpans(spans []*model.Span, sendFn func(*storage_v1.SpansResponseChunk) error) error {
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

func (s *GRPCHandler) Capabilities(ctx context.Context, request *storage_v1.CapabilitiesRequest) (*storage_v1.CapabilitiesResponse, error) {
	return &storage_v1.CapabilitiesResponse{
		ArchiveSpanReader:   s.impl.ArchiveSpanReader() != nil,
		ArchiveSpanWriter:   s.impl.ArchiveSpanWriter() != nil,
		StreamingSpanWriter: s.impl.StreamingSpanWriter() != nil,
	}, nil
}

func (s *GRPCHandler) GetArchiveTrace(r *storage_v1.GetTraceRequest, stream storage_v1.ArchiveSpanReaderPlugin_GetArchiveTraceServer) error {
	reader := s.impl.ArchiveSpanReader()
	if reader == nil {
		return status.Error(codes.Unimplemented, "not implemented")
	}
	trace, err := reader.GetTrace(stream.Context(), r.TraceID)
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

func (s *GRPCHandler) WriteArchiveSpan(ctx context.Context, r *storage_v1.WriteSpanRequest) (*storage_v1.WriteSpanResponse, error) {
	writer := s.impl.ArchiveSpanWriter()
	if writer == nil {
		return nil, status.Error(codes.Unimplemented, "not implemented")
	}
	err := writer.WriteSpan(ctx, r.Span)
	if err != nil {
		return nil, err
	}
	return &storage_v1.WriteSpanResponse{}, nil
}
