// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package model

import (
	modelv1 "github.com/jaegertracing/jaeger-idl/model/v1"
)

type SamplerType = modelv1.SamplerType

const (
	SamplerTypeUnrecognized SamplerType = iota
	SamplerTypeProbabilistic
	SamplerTypeLowerBound
	SamplerTypeRateLimiting
	SamplerTypeConst
)

var toSamplerType = map[string]SamplerType{
	"unrecognized":  SamplerTypeUnrecognized,
	"probabilistic": SamplerTypeProbabilistic,
	"lowerbound":    SamplerTypeLowerBound,
	"ratelimiting":  SamplerTypeRateLimiting,
	"const":         SamplerTypeConst,
}

var SpanKindTag = modelv1.SpanKindTag
