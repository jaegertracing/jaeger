package api_v2

import (
	_ "github.com/gogo/googleapis/google/api"
	_ "github.com/gogo/protobuf/gogoproto"
	jaegerIdlModel "github.com/jaegertracing/jaeger-idl/proto-gen/api_v2"
	grpc "google.golang.org/grpc"
)

// See description of the SamplingStrategyResponse.strategyType field.
type SamplingStrategyType = jaegerIdlModel.SamplingStrategyType

const (
	SamplingStrategyType_PROBABILISTIC SamplingStrategyType = jaegerIdlModel.SamplingStrategyType_PROBABILISTIC
	SamplingStrategyType_RATE_LIMITING SamplingStrategyType = jaegerIdlModel.SamplingStrategyType_RATE_LIMITING
)

var SamplingStrategyType_name = jaegerIdlModel.SamplingStrategyType_name

var SamplingStrategyType_value = jaegerIdlModel.SamplingStrategyType_value

// ProbabilisticSamplingStrategy samples traces with a fixed probability.
type ProbabilisticSamplingStrategy = jaegerIdlModel.ProbabilisticSamplingStrategy

// RateLimitingSamplingStrategy samples a fixed number of traces per time interval.
// The typical implementations use the leaky bucket algorithm.
type RateLimitingSamplingStrategy = jaegerIdlModel.RateLimitingSamplingStrategy

// OperationSamplingStrategy is a sampling strategy for a given operation
// (aka endpoint, span name). Only probabilistic sampling is currently supported.
type OperationSamplingStrategy = jaegerIdlModel.OperationSamplingStrategy

// PerOperationSamplingStrategies is a combination of strategies for different endpoints
// as well as some service-wide defaults. It is particularly useful for services whose
// endpoints receive vastly different traffic, so that any single rate of sampling would
// result in either too much data for some endpoints or almost no data for other endpoints.
type PerOperationSamplingStrategies = jaegerIdlModel.PerOperationSamplingStrategies

// SamplingStrategyResponse contains an overall sampling strategy for a given service.
// This type should = jaegerIdlModel.should as a union where only one of the strategy field is present.
type SamplingStrategyResponse = jaegerIdlModel.SamplingStrategyResponse

// SamplingStrategyParameters defines request parameters for remote sampler.
type SamplingStrategyParameters = jaegerIdlModel.SamplingStrategyParameters

// SamplingManagerClient is the client API for SamplingManager service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://godoc.org/google.golang.org/grpc#ClientConn.NewStream.
type SamplingManagerClient = jaegerIdlModel.SamplingManagerClient

func NewSamplingManagerClient(cc *grpc.ClientConn) SamplingManagerClient {
	return jaegerIdlModel.NewSamplingManagerClient(cc)
}

// SamplingManagerServer is the server API for SamplingManager service.
type SamplingManagerServer = jaegerIdlModel.SamplingManagerServer

// UnimplementedSamplingManagerServer can be embedded to have forward compatible implementations.
type UnimplementedSamplingManagerServer = jaegerIdlModel.UnimplementedSamplingManagerServer

func RegisterSamplingManagerServer(s *grpc.Server, srv SamplingManagerServer) {
	jaegerIdlModel.RegisterSamplingManagerServer(s, srv)
}

var (
	ErrInvalidLengthSampling        = jaegerIdlModel.ErrInvalidLengthSampling
	ErrIntOverflowSampling          = jaegerIdlModel.ErrIntOverflowSampling
	ErrUnexpectedEndOfGroupSampling = jaegerIdlModel.ErrUnexpectedEndOfGroupSampling
)
