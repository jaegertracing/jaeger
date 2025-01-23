// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package model

import (
	jaegerIdlModel "github.com/jaegertracing/jaeger-idl/model/v1"
)

type SamplerType = jaegerIdlModel.SamplerType

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

func SpanKindTag(kind SpanKind) KeyValue {
	return jaegerIdlModel.SpanKindTag(kind)
}
