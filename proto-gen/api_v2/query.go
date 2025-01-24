package api_v2

import (
	_ "github.com/gogo/googleapis/google/api"
	_ "github.com/gogo/protobuf/gogoproto"
	_ "github.com/gogo/protobuf/types"
	grpc "google.golang.org/grpc"

	jaegerIdlModel "github.com/jaegertracing/jaeger-idl/proto-gen/api_v2"
)

type GetTraceRequest = jaegerIdlModel.GetTraceRequest

type SpansResponseChunk = jaegerIdlModel.SpansResponseChunk

type ArchiveTraceRequest = jaegerIdlModel.ArchiveTraceRequest

type ArchiveTraceResponse = jaegerIdlModel.ArchiveTraceResponse

// Query parameters to find traces. Except for num_traces, all fields should be treated
// as forming a conjunction, e.g., "service_name='X' AND operation_name='Y' AND ...".
// All fields are matched against individual spans, not at the trace level.
// The returned results contain traces where at least one span matches the conditions.
// When num_traces results in fewer traces returned, there is no required ordering.
//
// Note: num_traces should restrict the number of traces returned, but not all backends
// interpret it this way. For instance, in Cassandra this limits the number of _spans_
// that match the conditions, and the resulting number of traces can be less.
//
// Note: some storage implementations do not guarantee the correct implementation of all parameters.
type TraceQueryParameters = jaegerIdlModel.TraceQueryParameters

type FindTracesRequest = jaegerIdlModel.FindTracesRequest

type GetServicesRequest = jaegerIdlModel.GetServicesRequest

type GetServicesResponse = jaegerIdlModel.GetServicesResponse

type GetOperationsRequest = jaegerIdlModel.GetOperationsRequest

type Operation = jaegerIdlModel.Operation

type GetOperationsResponse = jaegerIdlModel.GetOperationsResponse

type GetDependenciesRequest = jaegerIdlModel.GetDependenciesRequest

type GetDependenciesResponse = jaegerIdlModel.GetDependenciesResponse

// QueryServiceClient is the client API for QueryService service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://godoc.org/google.golang.org/grpc#ClientConn.NewStream.
type QueryServiceClient = jaegerIdlModel.QueryServiceClient

func NewQueryServiceClient(cc *grpc.ClientConn) QueryServiceClient {
	return jaegerIdlModel.NewQueryServiceClient(cc)
}

type QueryService_GetTraceClient = jaegerIdlModel.QueryService_GetTraceClient

type QueryService_FindTracesClient = jaegerIdlModel.QueryService_FindTracesClient

// QueryServiceServer is the server API for QueryService service.
type QueryServiceServer = jaegerIdlModel.QueryServiceServer

// UnimplementedQueryServiceServer can be embedded to have forward compatible implementations.
type UnimplementedQueryServiceServer = jaegerIdlModel.UnimplementedQueryServiceServer

func RegisterQueryServiceServer(s *grpc.Server, srv QueryServiceServer) {
	jaegerIdlModel.RegisterQueryServiceServer(s, srv)
}

type QueryService_GetTraceServer = jaegerIdlModel.QueryService_GetTraceServer

type QueryService_FindTracesServer = jaegerIdlModel.QueryService_FindTracesServer

var (
	ErrInvalidLengthQuery        = jaegerIdlModel.ErrInvalidLengthQuery
	ErrIntOverflowQuery          = jaegerIdlModel.ErrIntOverflowQuery
	ErrUnexpectedEndOfGroupQuery = jaegerIdlModel.ErrUnexpectedEndOfGroupQuery
)
