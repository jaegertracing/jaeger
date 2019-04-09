package shared

import (
	"context"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/proto-gen/storage_v1"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

const spanBatchSize = 1000

// GRPCServer implements shared.StoragePlugin and reads/writes spans and dependencies
type GRPCServer struct {
	Impl StoragePlugin
}

// GetDependencies returns all interservice dependencies
func (s *GRPCServer) GetDependencies(ctx context.Context, r *storage_v1.GetDependenciesRequest) (*storage_v1.GetDependenciesResponse, error) {
	deps, err := s.Impl.GetDependencies(r.EndTime, r.EndTime.Sub(r.StartTime))
	if err != nil {
		return nil, err
	}
	return &storage_v1.GetDependenciesResponse{
		Dependencies: deps,
	}, nil
}

// WriteSpan saves the span
func (s *GRPCServer) WriteSpan(ctx context.Context, r *storage_v1.WriteSpanRequest) (*storage_v1.WriteSpanResponse, error) {
	err := s.Impl.WriteSpan(r.Span)
	if err != nil {
		return nil, err
	}
	return &storage_v1.WriteSpanResponse{}, nil
}

// GetTrace takes a traceID and streams a Trace associated with that traceID
func (s *GRPCServer) GetTrace(r *storage_v1.GetTraceRequest, stream storage_v1.SpanReaderPlugin_GetTraceServer) error {
	trace, err := s.Impl.GetTrace(stream.Context(), r.TraceID)
	if err != nil {
		return err
	}

	var allSpans [][]model.Span
	currentSpans := make([]model.Span, 0, spanBatchSize)
	i := 0
	for _, span := range trace.Spans {
		if i == spanBatchSize {
			i = 0
			allSpans = append(allSpans, currentSpans)
			currentSpans = make([]model.Span, 0, spanBatchSize)
		}
		currentSpans = append(currentSpans, *span)
		i++
	}
	if len(currentSpans) > 0 {
		allSpans = append(allSpans, currentSpans)
	}

	for _, spans := range allSpans {
		err = stream.Send(&storage_v1.SpansResponseChunk{Spans: spans})
		if err != nil {
			return err
		}
	}

	return nil
}

// GetServices returns a list of all known services
func (s *GRPCServer) GetServices(ctx context.Context, r *storage_v1.GetServicesRequest) (*storage_v1.GetServicesResponse, error) {
	services, err := s.Impl.GetServices(ctx)
	if err != nil {
		return nil, err
	}
	return &storage_v1.GetServicesResponse{
		Services: services,
	}, nil
}

// GetOperations returns the operations of a given service
func (s *GRPCServer) GetOperations(ctx context.Context, r *storage_v1.GetOperationsRequest) (*storage_v1.GetOperationsResponse, error) {
	operations, err := s.Impl.GetOperations(ctx, r.Service)
	if err != nil {
		return nil, err
	}
	return &storage_v1.GetOperationsResponse{
		Operations: operations,
	}, nil
}

// FindTraces streams traces that match the traceQuery
func (s *GRPCServer) FindTraces(r *storage_v1.FindTracesRequest, stream storage_v1.SpanReaderPlugin_FindTracesServer) error {
	traces, err := s.Impl.FindTraces(stream.Context(), &spanstore.TraceQueryParameters{
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

	var allSpans [][]model.Span
	currentSpans := make([]model.Span, 0, spanBatchSize)
	i := 0
	for _, trace := range traces {
		for _, span := range trace.Spans {
			if i == spanBatchSize {
				i = 0
				allSpans = append(allSpans, currentSpans)
				currentSpans = make([]model.Span, 0, spanBatchSize)
			}
			currentSpans = append(currentSpans, *span)
			i++
		}
	}
	if len(currentSpans) > 0 {
		allSpans = append(allSpans, currentSpans)
	}

	for _, spans := range allSpans {
		err = stream.Send(&storage_v1.SpansResponseChunk{Spans: spans})
		if err != nil {
			return err
		}
	}

	return nil
}

// FindTraceIDs retrieves traceIDs that match the traceQuery
func (s *GRPCServer) FindTraceIDs(ctx context.Context, r *storage_v1.FindTraceIDsRequest) (*storage_v1.FindTraceIDsResponse, error) {
	traceIDs, err := s.Impl.FindTraceIDs(ctx, &spanstore.TraceQueryParameters{
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
