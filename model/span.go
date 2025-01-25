// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package model

import (
	modelv1 "github.com/jaegertracing/jaeger-idl/model/v1"
)

type SamplerType = modelv1.SamplerType

const (
	SamplerTypeUnrecognized  SamplerType = modelv1.SamplerTypeUnrecognized
	SamplerTypeProbabilistic             = modelv1.SamplerTypeProbabilistic
	SamplerTypeLowerBound                = modelv1.SamplerTypeLowerBound
	SamplerTypeRateLimiting              = modelv1.SamplerTypeRateLimiting
	SamplerTypeConst                     = modelv1.SamplerTypeConst
)

var SpanKindTag = modelv1.SpanKindTag
