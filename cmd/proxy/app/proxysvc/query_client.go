// Copyright (c) 2021 The Jaeger Authors.
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

package proxysvc

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	"google.golang.org/grpc"
)

type QueryClient struct {
	client api_v2.QueryServiceClient
}

func NewQueryClient(conn *grpc.ClientConn) *QueryClient {
	return &QueryClient{
		client: api_v2.NewQueryServiceClient(conn),
	}
}

func (qc *QueryClient) GetSpans(ctx context.Context, traceID model.TraceID) ([]model.Span, error) {
	var spans []model.Span
	req := api_v2.GetTraceRequest{TraceID: traceID}
	stream, err := qc.client.GetTrace(ctx, &req)
	if err != nil {
		return nil, err
	}
	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		for _, span := range chunk.GetSpans() {
			fmt.Println(span.Process.ServiceName)
			spans = append(spans, span)
		}
	}
	fmt.Println("returning")
	return spans, nil
}

func (qc *QueryClient) GetServices(ctx context.Context) ([]string, error) {
	res, err := qc.client.GetServices(ctx, &api_v2.GetServicesRequest{})
	if err != nil {
		return nil, err
	}
	return res.GetServices(), nil
}

func (qc *QueryClient) GetOperations(
	ctx context.Context,
	query spanstore.OperationQueryParameters,
) ([]spanstore.Operation, error) {
	req := api_v2.GetOperationsRequest{
		Service:  query.ServiceName,
		SpanKind: query.SpanKind,
	}
	res, err := qc.client.GetOperations(ctx, &req)
	if err != nil {
		return nil, err
	}
	var spanOps []spanstore.Operation
	for _, ops := range res.GetOperations() {
		spanOp := spanstore.Operation{
			Name:     ops.GetName(),
			SpanKind: ops.GetSpanKind(),
		}
		spanOps = append(spanOps, spanOp)
	}
	return spanOps, nil
}

func (qc *QueryClient) FindTraces(ctx context.Context, query *spanstore.TraceQueryParameters) ([]*model.Trace, error) {
	req := api_v2.FindTracesRequest{
		Query: &api_v2.TraceQueryParameters{
			ServiceName:   query.ServiceName,
			OperationName: query.OperationName,
			Tags:          query.Tags,
			StartTimeMin:  query.StartTimeMin,
			StartTimeMax:  query.StartTimeMax,
			DurationMin:   query.DurationMin,
			DurationMax:   query.DurationMax,
		},
	}

	stream, err := qc.client.FindTraces(ctx, &req)
	if err != nil {
		return nil, err
	}
	traceMap := make(map[model.TraceID]*model.Trace)
	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		spans := chunk.GetSpans()
		for i := range spans {
			traceID := spans[i].TraceID
			if t, ok := traceMap[traceID]; ok {
				t.Spans = append(t.Spans, &spans[i])
			} else {
				trace := &model.Trace{
					Spans: []*model.Span{&spans[i]},
				}
				traceMap[traceID] = trace
			}
		}
	}

	var traces []*model.Trace
	for _, t := range traceMap {
		traces = append(traces, t)
	}

	return traces, nil
}

func (qc *QueryClient) GetDependencies(ctx context.Context, endTs time.Time, lookback time.Duration) ([]model.DependencyLink, error) {
	req := api_v2.GetDependenciesRequest{
		StartTime: endTs.Add(-lookback),
		EndTime:   endTs,
	}
	res, err := qc.client.GetDependencies(ctx, &req)
	if err != nil {
		return nil, err
	}

	return res.GetDependencies(), nil
}
