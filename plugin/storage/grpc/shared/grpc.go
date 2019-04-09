// Copyright (c) 2018 The Jaeger Authors.
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
	"time"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/proto-gen/storage_v1"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

const spanBatchSize = 1000

// GRPCClient implements shared.StoragePlugin and reads/writes spans and dependencies
type GRPCClient struct {
	readerClient     storage_v1.SpanReaderPluginClient
	writerClient     storage_v1.SpanWriterPluginClient
	depsReaderClient storage_v1.DependenciesReaderPluginClient
}

// GetTrace takes a traceID and returns a Trace associated with that traceID
func (c *GRPCClient) GetTrace(ctx context.Context, traceID model.TraceID) (*model.Trace, error) {
	stream, err := c.readerClient.GetTrace(ctx, &storage_v1.GetTraceRequest{
		TraceID: traceID,
	})
	if err != nil {
		return nil, fmt.Errorf("plugin error: %s", err)
	}

	trace := model.Trace{}
	for {
		received, err := stream.Recv()
		if err == io.EOF {
			break
		}

		if err != nil {
			return nil, fmt.Errorf("stream error: %s", err)
		}

		for _, span := range received.Spans {
			trace.Spans = append(trace.Spans, &span)
		}
	}

	return &trace, nil
}

// GetServices returns a list of all known services
func (c *GRPCClient) GetServices(ctx context.Context) ([]string, error) {
	resp, err := c.readerClient.GetServices(ctx, &storage_v1.GetServicesRequest{})
	if err != nil {
		return nil, fmt.Errorf("plugin error: %s", err)
	}

	return resp.Services, nil
}

// GetOperations returns the operations of a given service
func (c *GRPCClient) GetOperations(ctx context.Context, service string) ([]string, error) {
	resp, err := c.readerClient.GetOperations(ctx, &storage_v1.GetOperationsRequest{
		Service: service,
	})
	if err != nil {
		return nil, fmt.Errorf("grpc error: %s", err)
	}

	return resp.Operations, nil
}

// FindTraces retrieves traces that match the traceQuery
func (c *GRPCClient) FindTraces(ctx context.Context, query *spanstore.TraceQueryParameters) ([]*model.Trace, error) {
	stream, err := c.readerClient.FindTraces(context.Background(), &storage_v1.FindTracesRequest{
		Query: &storage_v1.TraceQueryParameters{
			ServiceName:   query.ServiceName,
			OperationName: query.OperationName,
			Tags:          query.Tags,
			StartTimeMin:  query.StartTimeMin,
			StartTimeMax:  query.StartTimeMax,
			DurationMin:   query.DurationMin,
			DurationMax:   query.DurationMax,
			NumTraces:     int32(query.NumTraces),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("plugin error: %s", err)
	}

	var traces []*model.Trace
	var trace *model.Trace
	var traceID model.TraceID
	for {
		received, err := stream.Recv()
		if err == io.EOF {
			break
		}

		if err != nil {
			return nil, fmt.Errorf("stream error: %s", err)
		}

		for i, span := range received.Spans {
			if span.TraceID != traceID {
				if trace != nil {
					traces = append(traces, trace)
				}
				trace = &model.Trace{}
				traceID = span.TraceID
			}
			trace.Spans = append(trace.Spans, &received.Spans[i])
		}
	}
	if trace != nil {
		traces = append(traces, trace)
	}
	return traces, nil
}

// FindTraceIDs retrieves traceIDs that match the traceQuery
func (c *GRPCClient) FindTraceIDs(ctx context.Context, query *spanstore.TraceQueryParameters) ([]model.TraceID, error) {
	resp, err := c.readerClient.FindTraceIDs(context.Background(), &storage_v1.FindTraceIDsRequest{
		Query: &storage_v1.TraceQueryParameters{
			ServiceName:   query.ServiceName,
			OperationName: query.OperationName,
			Tags:          query.Tags,
			StartTimeMin:  query.StartTimeMin,
			StartTimeMax:  query.StartTimeMax,
			DurationMin:   query.DurationMin,
			DurationMax:   query.DurationMax,
			NumTraces:     int32(query.NumTraces),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("plugin error: %s", err)
	}

	return resp.TraceIDs, nil
}

// WriteSpan saves the span
func (c *GRPCClient) WriteSpan(span *model.Span) error {
	_, err := c.writerClient.WriteSpan(context.Background(), &storage_v1.WriteSpanRequest{
		Span: span,
	})
	if err != nil {
		return fmt.Errorf("plugin error: %s", err)
	}

	return nil
}

// GetDependencies returns all interservice dependencies
func (c *GRPCClient) GetDependencies(endTs time.Time, lookback time.Duration) ([]model.DependencyLink, error) {
	resp, err := c.depsReaderClient.GetDependencies(context.Background(), &storage_v1.GetDependenciesRequest{
		EndTime:   endTs,
		StartTime: endTs.Add(-lookback),
	})
	if err != nil {
		return nil, fmt.Errorf("grpc error: %s", err)
	}

	return resp.Dependencies, nil
}

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
