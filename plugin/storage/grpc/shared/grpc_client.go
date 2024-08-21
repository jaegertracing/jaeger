// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package shared

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/bearertoken"
	_ "github.com/jaegertracing/jaeger/pkg/gogocodec" // force gogo codec registration
	"github.com/jaegertracing/jaeger/proto-gen/storage_v1"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

// BearerTokenKey is the key name for the bearer token context value.
const BearerTokenKey = "bearer.token"

var (
	_ StoragePlugin        = (*GRPCClient)(nil)
	_ ArchiveStoragePlugin = (*GRPCClient)(nil)
	_ PluginCapabilities   = (*GRPCClient)(nil)

	// upgradeContext composites several steps of upgrading context
	upgradeContext = composeContextUpgradeFuncs(upgradeContextWithBearerToken)
)

// GRPCClient implements shared.StoragePlugin and reads/writes spans and dependencies
type GRPCClient struct {
	readerClient        storage_v1.SpanReaderPluginClient
	writerClient        storage_v1.SpanWriterPluginClient
	archiveReaderClient storage_v1.ArchiveSpanReaderPluginClient
	archiveWriterClient storage_v1.ArchiveSpanWriterPluginClient
	capabilitiesClient  storage_v1.PluginCapabilitiesClient
	depsReaderClient    storage_v1.DependenciesReaderPluginClient
	streamWriterClient  storage_v1.StreamingSpanWriterPluginClient
}

func NewGRPCClient(c *grpc.ClientConn) *GRPCClient {
	return &GRPCClient{
		readerClient:        storage_v1.NewSpanReaderPluginClient(c),
		writerClient:        storage_v1.NewSpanWriterPluginClient(c),
		archiveReaderClient: storage_v1.NewArchiveSpanReaderPluginClient(c),
		archiveWriterClient: storage_v1.NewArchiveSpanWriterPluginClient(c),
		capabilitiesClient:  storage_v1.NewPluginCapabilitiesClient(c),
		depsReaderClient:    storage_v1.NewDependenciesReaderPluginClient(c),
		streamWriterClient:  storage_v1.NewStreamingSpanWriterPluginClient(c),
	}
}

// ContextUpgradeFunc is a functional type that can be composed to upgrade context
type ContextUpgradeFunc func(ctx context.Context) context.Context

// composeContextUpgradeFuncs composes ContextUpgradeFunc and returns a composed function
// to run the given func in strict order.
func composeContextUpgradeFuncs(funcs ...ContextUpgradeFunc) ContextUpgradeFunc {
	return func(ctx context.Context) context.Context {
		for _, fun := range funcs {
			ctx = fun(ctx)
		}
		return ctx
	}
}

// upgradeContextWithBearerToken turns the context into a gRPC outgoing context with bearer token
// in the request metadata, if the original context has bearer token attached.
// Otherwise returns original context.
func upgradeContextWithBearerToken(ctx context.Context) context.Context {
	bearerToken, hasToken := bearertoken.GetBearerToken(ctx)
	if hasToken {
		md, ok := metadata.FromOutgoingContext(ctx)
		if !ok {
			md = metadata.New(nil)
		}
		md.Set(BearerTokenKey, bearerToken)
		return metadata.NewOutgoingContext(ctx, md)
	}
	return ctx
}

// DependencyReader implements shared.StoragePlugin.
func (c *GRPCClient) DependencyReader() dependencystore.Reader {
	return c
}

// SpanReader implements shared.StoragePlugin.
func (c *GRPCClient) SpanReader() spanstore.Reader {
	return c
}

// SpanWriter implements shared.StoragePlugin.
func (c *GRPCClient) SpanWriter() spanstore.Writer {
	return c
}

func (c *GRPCClient) StreamingSpanWriter() spanstore.Writer {
	return newStreamingSpanWriter(c.streamWriterClient)
}

func (c *GRPCClient) ArchiveSpanReader() spanstore.Reader {
	return &archiveReader{client: c.archiveReaderClient}
}

func (c *GRPCClient) ArchiveSpanWriter() spanstore.Writer {
	return &archiveWriter{client: c.archiveWriterClient}
}

// GetTrace takes a traceID and returns a Trace associated with that traceID
func (c *GRPCClient) GetTrace(ctx context.Context, traceID model.TraceID) (*model.Trace, error) {
	stream, err := c.readerClient.GetTrace(upgradeContext(ctx), &storage_v1.GetTraceRequest{
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
func (c *GRPCClient) GetServices(ctx context.Context) ([]string, error) {
	resp, err := c.readerClient.GetServices(upgradeContext(ctx), &storage_v1.GetServicesRequest{})
	if err != nil {
		return nil, fmt.Errorf("plugin error: %w", err)
	}

	return resp.Services, nil
}

// GetOperations returns the operations of a given service
func (c *GRPCClient) GetOperations(
	ctx context.Context,
	query spanstore.OperationQueryParameters,
) ([]spanstore.Operation, error) {
	resp, err := c.readerClient.GetOperations(upgradeContext(ctx), &storage_v1.GetOperationsRequest{
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
func (c *GRPCClient) FindTraces(ctx context.Context, query *spanstore.TraceQueryParameters) ([]*model.Trace, error) {
	stream, err := c.readerClient.FindTraces(upgradeContext(ctx), &storage_v1.FindTracesRequest{
		Query: &storage_v1.TraceQueryParameters{
			ServiceName:   query.ServiceName,
			OperationName: query.OperationName,
			Tags:          query.Tags,
			StartTimeMin:  query.StartTimeMin,
			StartTimeMax:  query.StartTimeMax,
			DurationMin:   query.DurationMin,
			DurationMax:   query.DurationMax,
			//nolint: gosec // G115
			NumTraces: int32(query.NumTraces),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("plugin error: %w", err)
	}

	var traces []*model.Trace
	var trace *model.Trace
	var traceID model.TraceID
	for received, err := stream.Recv(); !errors.Is(err, io.EOF); received, err = stream.Recv() {
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
func (c *GRPCClient) FindTraceIDs(ctx context.Context, query *spanstore.TraceQueryParameters) ([]model.TraceID, error) {
	resp, err := c.readerClient.FindTraceIDs(upgradeContext(ctx), &storage_v1.FindTraceIDsRequest{
		Query: &storage_v1.TraceQueryParameters{
			ServiceName:   query.ServiceName,
			OperationName: query.OperationName,
			Tags:          query.Tags,
			StartTimeMin:  query.StartTimeMin,
			StartTimeMax:  query.StartTimeMax,
			DurationMin:   query.DurationMin,
			DurationMax:   query.DurationMax,
			//nolint: gosec // G115
			NumTraces: int32(query.NumTraces),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("plugin error: %w", err)
	}

	return resp.TraceIDs, nil
}

// WriteSpan saves the span
func (c *GRPCClient) WriteSpan(ctx context.Context, span *model.Span) error {
	_, err := c.writerClient.WriteSpan(ctx, &storage_v1.WriteSpanRequest{
		Span: span,
	})
	if err != nil {
		return fmt.Errorf("plugin error: %w", err)
	}

	return nil
}

func (c *GRPCClient) Close() error {
	_, err := c.writerClient.Close(context.Background(), &storage_v1.CloseWriterRequest{})
	if err != nil && status.Code(err) != codes.Unimplemented {
		return fmt.Errorf("plugin error: %w", err)
	}

	return nil
}

// GetDependencies returns all interservice dependencies
func (c *GRPCClient) GetDependencies(ctx context.Context, endTs time.Time, lookback time.Duration) ([]model.DependencyLink, error) {
	resp, err := c.depsReaderClient.GetDependencies(ctx, &storage_v1.GetDependenciesRequest{
		EndTime:   endTs,
		StartTime: endTs.Add(-lookback),
	})
	if err != nil {
		return nil, fmt.Errorf("plugin error: %w", err)
	}

	return resp.Dependencies, nil
}

func (c *GRPCClient) Capabilities() (*Capabilities, error) {
	capabilities, err := c.capabilitiesClient.Capabilities(context.Background(), &storage_v1.CapabilitiesRequest{})
	if status.Code(err) == codes.Unimplemented {
		return &Capabilities{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("plugin error: %w", err)
	}

	return &Capabilities{
		ArchiveSpanReader:   capabilities.ArchiveSpanReader,
		ArchiveSpanWriter:   capabilities.ArchiveSpanWriter,
		StreamingSpanWriter: capabilities.StreamingSpanWriter,
	}, nil
}

func readTrace(stream storage_v1.SpanReaderPlugin_GetTraceClient) (*model.Trace, error) {
	trace := model.Trace{}
	for received, err := stream.Recv(); !errors.Is(err, io.EOF); received, err = stream.Recv() {
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
