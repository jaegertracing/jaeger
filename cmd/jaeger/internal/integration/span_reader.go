// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/ports"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

var (
	_ spanstore.Reader = (*spanReader)(nil)
	_ io.Closer        = (*spanReader)(nil)
)

// SpanReader retrieve span data from Jaeger-v2 query with api_v2.QueryServiceClient.
type spanReader struct {
	clientConn *grpc.ClientConn
	client     api_v2.QueryServiceClient
}

func createSpanReader(port int) (*spanReader, error) {
	opts := []grpc.DialOption{
		grpc.WithBlock(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cc, err := grpc.DialContext(ctx, ports.PortToHostPort(port), opts...)
	if err != nil {
		return nil, err
	}

	return &spanReader{
		clientConn: cc,
		client:     api_v2.NewQueryServiceClient(cc),
	}, nil
}

func (r *spanReader) Start() error {
	opts := []grpc.DialOption{
		grpc.WithBlock(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cc, err := grpc.DialContext(ctx, ports.PortToHostPort(ports.QueryGRPC), opts...)
	if err != nil {
		return err
	}

	r.clientConn = cc
	r.client = api_v2.NewQueryServiceClient(cc)
	return nil
}

func (r *spanReader) Close() error {
	return r.clientConn.Close()
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

	if query.NumTraces > math.MaxInt32 {
		return traces, fmt.Errorf("NumTraces must not greater than %d", math.MaxInt32)
	}
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
