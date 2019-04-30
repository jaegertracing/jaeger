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
	"io"
	"time"

	"github.com/pkg/errors"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/proto-gen/storage_v1"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

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
		return nil, errors.Wrap(err, "plugin error")
	}

	trace := model.Trace{}
	for received, err := stream.Recv(); err != io.EOF; received, err = stream.Recv() {
		if err != nil {
			return nil, errors.Wrap(err, "grpc stream error")
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
		return nil, errors.Wrap(err, "plugin error")
	}

	return resp.Services, nil
}

// GetOperations returns the operations of a given service
func (c *GRPCClient) GetOperations(ctx context.Context, service string) ([]string, error) {
	resp, err := c.readerClient.GetOperations(ctx, &storage_v1.GetOperationsRequest{
		Service: service,
	})
	if err != nil {
		return nil, errors.Wrap(err, "plugin error")
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
		return nil, errors.Wrap(err, "plugin error")
	}

	var traces []*model.Trace
	var trace *model.Trace
	var traceID model.TraceID
	for received, err := stream.Recv(); err != io.EOF; received, err = stream.Recv() {
		if err != nil {
			return nil, errors.Wrap(err, "stream error")
		}

		for i, span := range received.Spans {
			if span.TraceID != traceID {
				trace = &model.Trace{}
				traceID = span.TraceID
				traces = append(traces, trace)
			}
			trace.Spans = append(trace.Spans, &received.Spans[i])
		}
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
		return nil, errors.Wrap(err, "plugin error")
	}

	return resp.TraceIDs, nil
}

// WriteSpan saves the span
func (c *GRPCClient) WriteSpan(span *model.Span) error {
	_, err := c.writerClient.WriteSpan(context.Background(), &storage_v1.WriteSpanRequest{
		Span: span,
	})
	if err != nil {
		return errors.Wrap(err, "plugin error")
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
		return nil, errors.Wrap(err, "plugin error")
	}

	return resp.Dependencies, nil
}
