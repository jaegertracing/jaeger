// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package sampling

import (
	modelv1 "github.com/jaegertracing/jaeger-idl/thrift-gen/sampling"
)

type SamplingStrategyType = modelv1.SamplingStrategyType

const (
	SamplingStrategyType_PROBABILISTIC SamplingStrategyType = modelv1.SamplingStrategyType_PROBABILISTIC
	SamplingStrategyType_RATE_LIMITING SamplingStrategyType = modelv1.SamplingStrategyType_RATE_LIMITING
)

var SamplingStrategyTypeFromString = modelv1.SamplingStrategyTypeFromString

var SamplingStrategyTypePtr = modelv1.SamplingStrategyTypePtr

type ProbabilisticSamplingStrategy = modelv1.ProbabilisticSamplingStrategy

var NewProbabilisticSamplingStrategy = modelv1.NewProbabilisticSamplingStrategy

type RateLimitingSamplingStrategy = modelv1.RateLimitingSamplingStrategy

var NewRateLimitingSamplingStrategy = modelv1.NewRateLimitingSamplingStrategy

type OperationSamplingStrategy = modelv1.OperationSamplingStrategy

var NewOperationSamplingStrategy = modelv1.NewOperationSamplingStrategy

type PerOperationSamplingStrategies = modelv1.PerOperationSamplingStrategies

var NewPerOperationSamplingStrategies = modelv1.NewPerOperationSamplingStrategies

type SamplingStrategyResponse = modelv1.SamplingStrategyResponse

var NewSamplingStrategyResponse = modelv1.NewSamplingStrategyResponse

type SamplingManager = modelv1.SamplingManager

type SamplingManagerClient = modelv1.SamplingManagerClient

var NewSamplingManagerClientFactory = modelv1.NewSamplingManagerClientFactory

var NewSamplingManagerClientProtocol = modelv1.NewSamplingManagerClientProtocol

var NewSamplingManagerClient = modelv1.NewSamplingManagerClient

type SamplingManagerProcessor = modelv1.SamplingManagerProcessor

var NewSamplingManagerProcessor = modelv1.NewSamplingManagerProcessor

type SamplingManagerGetSamplingStrategyArgs = modelv1.SamplingManagerGetSamplingStrategyArgs

var NewSamplingManagerGetSamplingStrategyArgs = modelv1.NewSamplingManagerGetSamplingStrategyArgs

type SamplingManagerGetSamplingStrategyResult = modelv1.SamplingManagerGetSamplingStrategyResult

var NewSamplingManagerGetSamplingStrategyResult = modelv1.NewSamplingManagerGetSamplingStrategyResult
