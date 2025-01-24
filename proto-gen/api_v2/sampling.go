package api_v2

import (
	_ "github.com/gogo/googleapis/google/api"
	_ "github.com/gogo/protobuf/gogoproto"
	modelv1 "github.com/jaegertracing/jaeger-idl/proto-gen/api_v2"
)

// See description of the SamplingStrategyResponse.strategyType field.
type SamplingStrategyType = modelv1.SamplingStrategyType

const (
	SamplingStrategyType_PROBABILISTIC SamplingStrategyType = modelv1.SamplingStrategyType_PROBABILISTIC
	SamplingStrategyType_RATE_LIMITING SamplingStrategyType = modelv1.SamplingStrategyType_RATE_LIMITING
)

var SamplingStrategyType_name = modelv1.SamplingStrategyType_name

var SamplingStrategyType_value = modelv1.SamplingStrategyType_value

// ProbabilisticSamplingStrategy samples traces with a fixed probability.
type ProbabilisticSamplingStrategy = modelv1.ProbabilisticSamplingStrategy

// RateLimitingSamplingStrategy samples a fixed number of traces per time interval.
// The typical implementations use the leaky bucket algorithm.
type RateLimitingSamplingStrategy = modelv1.RateLimitingSamplingStrategy

// OperationSamplingStrategy is a sampling strategy for a given operation
// (aka endpoint, span name). Only probabilistic sampling is currently supported.
type OperationSamplingStrategy = modelv1.OperationSamplingStrategy

// PerOperationSamplingStrategies is a combination of strategies for different endpoints
// as well as some service-wide defaults. It is particularly useful for services whose
// endpoints receive vastly different traffic, so that any single rate of sampling would
// result in either too much data for some endpoints or almost no data for other endpoints.
type PerOperationSamplingStrategies = modelv1.PerOperationSamplingStrategies

// SamplingStrategyResponse contains an overall sampling strategy for a given service.
// This type should = modelv1.should as a union where only one of the strategy field is present.
type SamplingStrategyResponse = modelv1.SamplingStrategyResponse

// SamplingStrategyParameters defines request parameters for remote sampler.
type SamplingStrategyParameters = modelv1.SamplingStrategyParameters

// SamplingManagerClient is the client API for SamplingManager service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://godoc.org/google.golang.org/grpc#ClientConn.NewStream.
type SamplingManagerClient = modelv1.SamplingManagerClient

var NewSamplingManagerClient = modelv1.NewSamplingManagerClient

// SamplingManagerServer is the server API for SamplingManager service.
type SamplingManagerServer = modelv1.SamplingManagerServer

// UnimplementedSamplingManagerServer can be embedded to have forward compatible implementations.
type UnimplementedSamplingManagerServer = modelv1.UnimplementedSamplingManagerServer

var RegisterSamplingManagerServer = modelv1.RegisterSamplingManagerServer

var (
	ErrInvalidLengthSampling        = modelv1.ErrInvalidLengthSampling
	ErrIntOverflowSampling          = modelv1.ErrIntOverflowSampling
	ErrUnexpectedEndOfGroupSampling = modelv1.ErrUnexpectedEndOfGroupSampling
)
