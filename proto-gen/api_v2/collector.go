package api_v2

import (
	_ "github.com/gogo/googleapis/google/api"
	_ "github.com/gogo/protobuf/gogoproto"
	jaegerIdlModel "github.com/jaegertracing/jaeger-idl/proto-gen/api_v2"
	grpc "google.golang.org/grpc"
)

type PostSpansRequest = jaegerIdlModel.PostSpansRequest

type PostSpansResponse = jaegerIdlModel.PostSpansResponse

// CollectorServiceClient is the client API for CollectorService service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://godoc.org/google.golang.org/grpc#ClientConn.NewStream.
type CollectorServiceClient = jaegerIdlModel.CollectorServiceClient

func NewCollectorServiceClient(cc *grpc.ClientConn) CollectorServiceClient {
	return jaegerIdlModel.NewCollectorServiceClient(cc)
}

// CollectorServiceServer is the server API for CollectorService service.
type CollectorServiceServer = jaegerIdlModel.CollectorServiceServer

// UnimplementedCollectorServiceServer can be embedded to have forward compatible implementations.
type UnimplementedCollectorServiceServer = jaegerIdlModel.UnimplementedCollectorServiceServer

func RegisterCollectorServiceServer(s *grpc.Server, srv CollectorServiceServer) {
	jaegerIdlModel.RegisterCollectorServiceServer(s, srv)
}

var (
	ErrInvalidLengthCollector        = jaegerIdlModel.ErrInvalidLengthCollector
	ErrIntOverflowCollector          = jaegerIdlModel.ErrIntOverflowCollector
	ErrUnexpectedEndOfGroupCollector = jaegerIdlModel.ErrUnexpectedEndOfGroupCollector
)
