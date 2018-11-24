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
	"time"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/plugin/storage/external/proto"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

type GRPCClient struct {
	client proto.StoragePluginClient
}

func (c *GRPCClient) GetTrace(ctx context.Context, traceID model.TraceID) (*model.Trace, error) {
	resp, err := c.client.GetTrace(ctx, &proto.GetTraceRequest{
		TraceId: TraceIDToProto(&traceID),
	})
	if err != nil {
		return nil, fmt.Errorf("grpc error: %s", err)
	}

	switch t := resp.Response.(type) {
	case *proto.GetTraceResponse_Success:
		return TraceFromProto(t.Success.Trace), nil
	case *proto.GetTraceResponse_Error:
		return nil, fmt.Errorf("plugin error: %s", t.Error.Message)
	default:
		panic("unreachable")
	}
}

func (c *GRPCClient) GetServices(ctx context.Context) ([]string, error) {
	resp, err := c.client.GetServices(ctx, &proto.GetServicesRequest{})
	if err != nil {
		return nil, fmt.Errorf("grpc error: %s", err)
	}

	switch t := resp.Response.(type) {
	case *proto.GetServicesResponse_Success:
		return t.Success.Services, nil
	case *proto.GetServicesResponse_Error:
		return nil, fmt.Errorf("plugin error: %s", t.Error.Message)
	default:
		panic("unreachable")
	}
}

func (c *GRPCClient) GetOperations(ctx context.Context, service string) ([]string, error) {
	resp, err := c.client.GetOperations(ctx, &proto.GetOperationsRequest{
		Service: service,
	})
	if err != nil {
		return nil, fmt.Errorf("grpc error: %s", err)
	}

	switch t := resp.Response.(type) {
	case *proto.GetOperationsResponse_Success:
		return t.Success.Operations, nil
	case *proto.GetOperationsResponse_Error:
		return nil, fmt.Errorf("plugin error: %s", t.Error.Message)
	default:
		panic("unreachable")
	}
}

func (c *GRPCClient) FindTraces(ctx context.Context, query *spanstore.TraceQueryParameters) ([]*model.Trace, error) {
	resp, err := c.client.FindTraces(context.Background(), &proto.FindTracesRequest{
		ServiceName:   query.ServiceName,
		OperationName: query.OperationName,
		Tags:          query.Tags,
		StartTimeMin:  TimeToProto(query.StartTimeMin),
		StartTimeMax:  TimeToProto(query.StartTimeMax),
		DurationMin:   DurationToProto(query.DurationMin),
		DurationMax:   DurationToProto(query.DurationMax),
		NumTraces:     int32(query.NumTraces),
	})
	if err != nil {
		return nil, fmt.Errorf("grpc error: %s", err)
	}

	switch t := resp.Response.(type) {
	case *proto.FindTracesResponse_Success:
		return TraceSliceFromProto(t.Success.Traces), nil
	case *proto.FindTracesResponse_Error:
		return nil, fmt.Errorf("plugin error: %s", t.Error.Message)
	default:
		panic("unreachable")
	}
}

func (c *GRPCClient) WriteSpan(span *model.Span) error {
	resp, err := c.client.WriteSpan(context.Background(), &proto.WriteSpanRequest{
		Span: SpanToProto(span),
	})
	if err != nil {
		return fmt.Errorf("grpc error: %s", err)
	}

	switch t := resp.Response.(type) {
	case *proto.WriteSpanResponse_Success:
		return nil
	case *proto.WriteSpanResponse_Error:
		return fmt.Errorf("plugin error: %s", t.Error.Message)
	default:
		panic("unreachable")
	}
}

func (c *GRPCClient) GetDependencies(endTs time.Time, lookback time.Duration) ([]model.DependencyLink, error) {
	resp, err := c.client.GetDependencies(context.Background(), &proto.GetDependenciesRequest{
		EndTimestamp: TimeToProto(endTs),
		Lookback:     DurationToProto(lookback),
	})
	if err != nil {
		return nil, fmt.Errorf("grpc error: %s", err)
	}

	switch t := resp.Response.(type) {
	case *proto.GetDependenciesResponse_Success:
		return DependencyLinkSliceFromProto(t.Success.Dependencies), nil
	case *proto.GetDependenciesResponse_Error:
		return nil, fmt.Errorf("plugin error: %s", t.Error.Message)
	default:
		panic("unreachable")
	}
}

type GRPCServer struct {
	Impl StoragePlugin
}

func (s *GRPCServer) GetDependencies(ctx context.Context, r *proto.GetDependenciesRequest) (*proto.GetDependenciesResponse, error) {
	deps, err := s.Impl.GetDependencies(TimeFromProto(r.EndTimestamp), DurationFromProto(r.Lookback))
	if err != nil {
		return &proto.GetDependenciesResponse{
			Response: &proto.GetDependenciesResponse_Error{
				Error: &proto.StoragePluginError{
					Message: err.Error(),
				},
			},
		}, nil
	}
	return &proto.GetDependenciesResponse{
		Response: &proto.GetDependenciesResponse_Success{
			Success: &proto.GetDependenciesSuccess{
				Dependencies: DependencyLinkSliceToProto(deps),
			},
		},
	}, nil
}

func (s *GRPCServer) WriteSpan(ctx context.Context, r *proto.WriteSpanRequest) (*proto.WriteSpanResponse, error) {
	err := s.Impl.WriteSpan(SpanFromProto(r.Span))
	if err != nil {
		return &proto.WriteSpanResponse{
			Response: &proto.WriteSpanResponse_Error{
				Error: &proto.StoragePluginError{
					Message: err.Error(),
				},
			},
		}, nil
	}
	return &proto.WriteSpanResponse{
		Response: &proto.WriteSpanResponse_Success{
			Success: &proto.EmptyResponse{},
		},
	}, nil
}

func (s *GRPCServer) GetTrace(ctx context.Context, r *proto.GetTraceRequest) (*proto.GetTraceResponse, error) {
	trace, err := s.Impl.GetTrace(ctx, TraceIDFromProto(r.TraceId))
	if err != nil {
		return &proto.GetTraceResponse{
			Response: &proto.GetTraceResponse_Error{
				Error: &proto.StoragePluginError{
					Message: err.Error(),
				},
			},
		}, nil
	}
	return &proto.GetTraceResponse{
		Response: &proto.GetTraceResponse_Success{
			Success: &proto.GetTraceSuccess{
				Trace: TraceToProto(trace),
			},
		},
	}, nil
}

func (s *GRPCServer) GetServices(ctx context.Context, r *proto.GetServicesRequest) (*proto.GetServicesResponse, error) {
	services, err := s.Impl.GetServices(ctx)
	if err != nil {
		return &proto.GetServicesResponse{
			Response: &proto.GetServicesResponse_Error{
				Error: &proto.StoragePluginError{
					Message: err.Error(),
				},
			},
		}, nil
	}
	return &proto.GetServicesResponse{
		Response: &proto.GetServicesResponse_Success{
			Success: &proto.GetServicesSuccess{
				Services: services,
			},
		},
	}, nil
}

func (s *GRPCServer) GetOperations(ctx context.Context, r *proto.GetOperationsRequest) (*proto.GetOperationsResponse, error) {
	operations, err := s.Impl.GetOperations(ctx, r.Service)
	if err != nil {
		return &proto.GetOperationsResponse{
			Response: &proto.GetOperationsResponse_Error{
				Error: &proto.StoragePluginError{
					Message: err.Error(),
				},
			},
		}, nil
	}
	return &proto.GetOperationsResponse{
		Response: &proto.GetOperationsResponse_Success{
			Success: &proto.GetOperationsSuccess{
				Operations: operations,
			},
		},
	}, nil
}

func (s *GRPCServer) FindTraces(ctx context.Context, r *proto.FindTracesRequest) (*proto.FindTracesResponse, error) {
	traces, err := s.Impl.FindTraces(ctx, &spanstore.TraceQueryParameters{
		ServiceName:   r.ServiceName,
		OperationName: r.OperationName,
		Tags:          r.Tags,
		StartTimeMin:  TimeFromProto(r.StartTimeMin),
		StartTimeMax:  TimeFromProto(r.StartTimeMax),
		DurationMin:   DurationFromProto(r.DurationMin),
		DurationMax:   DurationFromProto(r.DurationMax),
		NumTraces:     int(r.NumTraces),
	})
	if err != nil {
		return &proto.FindTracesResponse{
			Response: &proto.FindTracesResponse_Error{
				Error: &proto.StoragePluginError{
					Message: err.Error(),
				},
			},
		}, nil
	}
	return &proto.FindTracesResponse{
		Response: &proto.FindTracesResponse_Success{
			Success: &proto.FindTracesSuccess{
				Traces: TraceSliceToProto(traces),
			},
		},
	}, nil
}
