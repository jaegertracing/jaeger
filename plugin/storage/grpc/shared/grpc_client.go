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
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/proto-gen/storage_v1"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

var (
	_ StoragePlugin        = (*grpcClient)(nil)
	_ ArchiveStoragePlugin = (*grpcClient)(nil)
	_ PluginCapabilities   = (*grpcClient)(nil)
)

// grpcClient implements shared.StoragePlugin and reads/writes spans and dependencies
type grpcClient struct {
	readerClient        storage_v1.SpanReaderPluginClient
	writerClient        storage_v1.SpanWriterPluginClient
	archiveReaderClient storage_v1.ArchiveSpanReaderPluginClient
	archiveWriterClient storage_v1.ArchiveSpanWriterPluginClient
	capabilitiesClient  storage_v1.PluginCapabilitiesClient
	depsReaderClient    storage_v1.DependenciesReaderPluginClient
}

// upgradeContextWithBearerToken turns the context into a gRPC outgoing context with bearer token
// in the request metadata, if the original context has bearer token attached.
// Otherwise returns original context.
func upgradeContextWithBearerToken(ctx context.Context) context.Context {
	bearerToken, hasToken := spanstore.GetBearerToken(ctx)
	if hasToken {
		requestMetadata := metadata.New(map[string]string{
			spanstore.BearerTokenKey: bearerToken,
		})
		return metadata.NewOutgoingContext(ctx, requestMetadata)
	}
	return ctx
}

// DependencyReader implements shared.StoragePlugin.
func (c *grpcClient) DependencyReader() dependencystore.Reader {
	return c
}

// SpanReader implements shared.StoragePlugin.
func (c *grpcClient) SpanReader() spanstore.Reader {
	return c
}

// SpanWriter implements shared.StoragePlugin.
func (c *grpcClient) SpanWriter() spanstore.Writer {
	return c
}

func (c *grpcClient) ArchiveSpanReader() spanstore.Reader {
	return &archiveReader{client: c.archiveReaderClient}
}

func (c *grpcClient) ArchiveSpanWriter() spanstore.Writer {
	return &archiveWriter{client: c.archiveWriterClient}
}

// GetTrace takes a traceID and returns a Trace associated with that traceID
func (c *grpcClient) GetTrace(ctx context.Context, traceID model.TraceID) (*model.Trace, error) {
	stream, err := c.readerClient.GetTrace(upgradeContextWithBearerToken(ctx), &storage_v1.GetTraceRequest{
		TraceID: traceID,
	})
	if status.Code(err) == codes.NotFound {
		return nil, spanstore.ErrTraceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("plugin error: %w", err)
	}

	return readTrace(stream)
}

// GetServices returns a list of all known services
func (c *grpcClient) GetServices(ctx context.Context) ([]string, error) {
	resp, err := c.readerClient.GetServices(upgradeContextWithBearerToken(ctx), &storage_v1.GetServicesRequest{})
	if err != nil {
		return nil, fmt.Errorf("plugin error: %w", err)
	}

	return resp.Services, nil
}

// GetOperations returns the operations of a given service
func (c *grpcClient) GetOperations(
	ctx context.Context,
	query spanstore.OperationQueryParameters,
) ([]spanstore.Operation, error) {
	resp, err := c.readerClient.GetOperations(upgradeContextWithBearerToken(ctx), &storage_v1.GetOperationsRequest{
		Service:  query.ServiceName,
		SpanKind: query.SpanKind,
	})
	if err != nil {
		return nil, fmt.Errorf("plugin error: %w", err)
	}

	var operations []spanstore.Operation
	if resp.Operations != nil {
		for _, operation := range resp.Operations {
			operations = append(operations, spanstore.Operation{
				Name:     operation.Name,
				SpanKind: operation.SpanKind,
			})
		}
	} else {
		for _, name := range resp.OperationNames {
			operations = append(operations, spanstore.Operation{
				Name: name,
			})
		}
	}
	return operations, nil
}

// FindTraces retrieves traces that match the traceQuery
func (c *grpcClient) FindTraces(ctx context.Context, query *spanstore.TraceQueryParameters) ([]*model.Trace, error) {
	stream, err := c.readerClient.FindTraces(upgradeContextWithBearerToken(ctx), &storage_v1.FindTracesRequest{
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
		return nil, fmt.Errorf("plugin error: %w", err)
	}

	var traces []*model.Trace
	var trace *model.Trace
	var traceID model.TraceID
	for received, err := stream.Recv(); err != io.EOF; received, err = stream.Recv() {
		if err != nil {
			return nil, fmt.Errorf("stream error: %w", err)
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
func (c *grpcClient) FindTraceIDs(ctx context.Context, query *spanstore.TraceQueryParameters) ([]model.TraceID, error) {
	resp, err := c.readerClient.FindTraceIDs(upgradeContextWithBearerToken(ctx), &storage_v1.FindTraceIDsRequest{
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
		return nil, fmt.Errorf("plugin error: %w", err)
	}

	return resp.TraceIDs, nil
}

// WriteSpan saves the span
func (c *grpcClient) WriteSpan(ctx context.Context, span *model.Span) error {
	_, err := c.writerClient.WriteSpan(ctx, &storage_v1.WriteSpanRequest{
		Span: span,
	})
	if err != nil {
		return fmt.Errorf("plugin error: %w", err)
	}

	return nil
}

// GetDependencies returns all interservice dependencies
func (c *grpcClient) GetDependencies(ctx context.Context, endTs time.Time, lookback time.Duration) ([]model.DependencyLink, error) {
	resp, err := c.depsReaderClient.GetDependencies(ctx, &storage_v1.GetDependenciesRequest{
		EndTime:   endTs,
		StartTime: endTs.Add(-lookback),
	})
	if err != nil {
		return nil, fmt.Errorf("plugin error: %w", err)
	}

	return resp.Dependencies, nil
}

func (c *grpcClient) Capabilities() (*Capabilities, error) {
	capabilities, err := c.capabilitiesClient.Capabilities(context.Background(), &storage_v1.CapabilitiesRequest{})
	if status.Code(err) == codes.Unimplemented {
		return &Capabilities{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("plugin error: %w", err)
	}

	return &Capabilities{
		ArchiveSpanReader: capabilities.ArchiveSpanReader,
		ArchiveSpanWriter: capabilities.ArchiveSpanWriter,
	}, nil
}

func readTrace(stream storage_v1.SpanReaderPlugin_GetTraceClient) (*model.Trace, error) {
	trace := model.Trace{}
	for received, err := stream.Recv(); err != io.EOF; received, err = stream.Recv() {
		if err != nil {
			if s, _ := status.FromError(err); s != nil {
				if s.Message() == spanstore.ErrTraceNotFound.Error() {
					return nil, spanstore.ErrTraceNotFound
				}
			}
			return nil, fmt.Errorf("grpc stream error: %w", err)
		}

		for i := range received.Spans {
			trace.Spans = append(trace.Spans, &received.Spans[i])
		}
	}

	return &trace, nil
}
