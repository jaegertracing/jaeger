package api_v2

import (
	modelv1 "github.com/jaegertracing/jaeger-idl/proto-gen/api_v2"
)

type GetTraceRequest = modelv1.GetTraceRequest

type SpansResponseChunk = modelv1.SpansResponseChunk

type ArchiveTraceRequest = modelv1.ArchiveTraceRequest

type ArchiveTraceResponse = modelv1.ArchiveTraceResponse

type TraceQueryParameters = modelv1.TraceQueryParameters

type FindTracesRequest = modelv1.FindTracesRequest

type GetServicesRequest = modelv1.GetServicesRequest

type GetServicesResponse = modelv1.GetServicesResponse

type GetOperationsRequest = modelv1.GetOperationsRequest

type Operation = modelv1.Operation

type GetOperationsResponse = modelv1.GetOperationsResponse

type GetDependenciesRequest = modelv1.GetDependenciesRequest

type GetDependenciesResponse = modelv1.GetDependenciesResponse

// QueryServiceClient is the client API for QueryService service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://godoc.org/google.golang.org/grpc#ClientConn.NewStream.
type QueryServiceClient = modelv1.QueryServiceClient

var NewQueryServiceClient = modelv1.NewQueryServiceClient

type QueryService_GetTraceClient = modelv1.QueryService_GetTraceClient

type QueryService_FindTracesClient = modelv1.QueryService_FindTracesClient

// QueryServiceServer is the server API for QueryService service.
type QueryServiceServer = modelv1.QueryServiceServer

// UnimplementedQueryServiceServer can be embedded to have forward compatible implementations.
type UnimplementedQueryServiceServer = modelv1.UnimplementedQueryServiceServer

var RegisterQueryServiceServer = modelv1.RegisterQueryServiceServer

type QueryService_GetTraceServer = modelv1.QueryService_GetTraceServer

type QueryService_FindTracesServer = modelv1.QueryService_FindTracesServer

