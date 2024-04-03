// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"time"

	jaeger2otlp "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/jaeger"
	"github.com/open-telemetry/opentelemetry-collector-contrib/testbed/testbed"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/plugin/storage/integration"
	"github.com/jaegertracing/jaeger/ports"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

var (
	_ spanstore.Writer = (*spanWriter)(nil)
	_ spanstore.Reader = (*spanReader)(nil)
)

// E2EStorageIntegration holds components for e2e mode of Jaeger-v2
// storage integration test. The intended usage is as follows:
//   - A specific storage implementation declares its own test functions
//     (e.g. starts remote-storage).
//   - In those functions, instantiates with e2eInitialize()
//     and clean up with e2eCleanUp().
//   - Then calls RunTestSpanstore.
type E2EStorageIntegration struct {
	integration.StorageIntegration
	ConfigFile string

	runner        testbed.OtelcolRunner
	configCleanup func()
}

// e2eInitialize starts the Jaeger-v2 collector with the provided config file,
// it also initialize the SpanWriter and SpanReader below.
func (s *E2EStorageIntegration) e2eInitialize() error {
	factories, err := internal.Components()
	if err != nil {
		return err
	}

	config, err := os.ReadFile(s.ConfigFile)
	if err != nil {
		return err
	}
	config = []byte(strings.Replace(string(config), "./cmd/jaeger/", "../../../", 1))

	s.runner = testbed.NewInProcessCollector(factories)
	s.configCleanup, err = s.runner.PrepareConfig(string(config))
	if err != nil {
		return err
	}
	if err = s.runner.Start(testbed.StartParams{}); err != nil {
		return err
	}

	if s.SpanWriter, err = createSpanWriter(); err != nil {
		return err
	}
	if s.SpanReader, err = createSpanReader(); err != nil {
		return err
	}
	return nil
}

func (s *E2EStorageIntegration) e2eCleanUp() error {
	s.configCleanup()
	_, err := s.runner.Stop()
	return err
}

// SpanWriter utilizes the OTLP exporter to send span data to the Jaeger-v2 receiver
type spanWriter struct {
	testbed.TraceDataSender
}

func createSpanWriter() (*spanWriter, error) {
	sender := testbed.NewOTLPTraceDataSender(testbed.DefaultHost, testbed.DefaultOTLPPort)
	err := sender.Start()
	if err != nil {
		return nil, err
	}

	return &spanWriter{
		TraceDataSender: sender,
	}, nil
}

func (w *spanWriter) WriteSpan(ctx context.Context, span *model.Span) error {
	td, err := jaeger2otlp.ProtoToTraces([]*model.Batch{
		{
			Spans:   []*model.Span{span},
			Process: span.Process,
		},
	})
	if err != nil {
		return err
	}

	return w.ConsumeTraces(ctx, td)
}

// SpanReader retrieve span data from Jaeger-v2 query with api_v2.QueryServiceClient.
type spanReader struct {
	client api_v2.QueryServiceClient
}

func createSpanReader() (*spanReader, error) {
	opts := []grpc.DialOption{
		grpc.WithBlock(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cc, err := grpc.DialContext(ctx, ports.PortToHostPort(ports.QueryGRPC), opts...)
	if err != nil {
		return nil, err
	}

	return &spanReader{
		client: api_v2.NewQueryServiceClient(cc),
	}, nil
}

func unwrapNotFoundErr(err error) error {
	if s, _ := status.FromError(err); s != nil {
		if strings.Contains(s.Message(), spanstore.ErrTraceNotFound.Error()) {
			return spanstore.ErrTraceNotFound
		}
	}
	return err
}

func (r *spanReader) GetTrace(ctx context.Context, traceID model.TraceID) (*model.Trace, error) {
	stream, err := r.client.GetTrace(ctx, &api_v2.GetTraceRequest{
		TraceID: traceID,
	})
	if err != nil {
		return nil, unwrapNotFoundErr(err)
	}

	var spans []*model.Span
	for received, err := stream.Recv(); !errors.Is(err, io.EOF); received, err = stream.Recv() {
		if err != nil {
			return nil, unwrapNotFoundErr(err)
		}
		for i := range received.Spans {
			spans = append(spans, &received.Spans[i])
		}
	}

	return &model.Trace{
		Spans: spans,
	}, nil
}

func (r *spanReader) GetServices(ctx context.Context) ([]string, error) {
	res, err := r.client.GetServices(ctx, &api_v2.GetServicesRequest{})
	if err != nil {
		return []string{}, err
	}
	return res.Services, nil
}

func (r *spanReader) GetOperations(ctx context.Context, query spanstore.OperationQueryParameters) ([]spanstore.Operation, error) {
	var operations []spanstore.Operation
	res, err := r.client.GetOperations(ctx, &api_v2.GetOperationsRequest{
		Service:  query.ServiceName,
		SpanKind: query.SpanKind,
	})
	if err != nil {
		return operations, err
	}

	for _, operation := range res.Operations {
		operations = append(operations, spanstore.Operation{
			Name:     operation.Name,
			SpanKind: operation.SpanKind,
		})
	}
	return operations, nil
}

func (r *spanReader) FindTraces(ctx context.Context, query *spanstore.TraceQueryParameters) ([]*model.Trace, error) {
	var traces []*model.Trace
	stream, err := r.client.FindTraces(ctx, &api_v2.FindTracesRequest{
		Query: &api_v2.TraceQueryParameters{
			ServiceName:   query.ServiceName,
			OperationName: query.OperationName,
			Tags:          query.Tags,
			StartTimeMin:  query.StartTimeMin,
			StartTimeMax:  query.StartTimeMax,
			DurationMin:   query.DurationMin,
			DurationMax:   query.DurationMax,
			SearchDepth:   int32(query.NumTraces),
		},
	})
	if err != nil {
		return traces, err
	}

	spanMaps := map[string][]*model.Span{}
	for received, err := stream.Recv(); !errors.Is(err, io.EOF); received, err = stream.Recv() {
		if err != nil {
			return nil, unwrapNotFoundErr(err)
		}
		for i, span := range received.Spans {
			traceID := span.TraceID.String()
			if _, ok := spanMaps[traceID]; !ok {
				spanMaps[traceID] = make([]*model.Span, 0)
			}
			spanMaps[traceID] = append(spanMaps[traceID], &received.Spans[i])
		}
	}

	for _, spans := range spanMaps {
		traces = append(traces, &model.Trace{
			Spans: spans,
		})
	}
	return traces, nil
}

func (r *spanReader) FindTraceIDs(ctx context.Context, query *spanstore.TraceQueryParameters) ([]model.TraceID, error) {
	panic("not implemented")
}
