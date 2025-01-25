// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package model

import (
	modelv1 "github.com/jaegertracing/jaeger-idl/model/v1"
)

// These constants are kept mostly for backwards compatibility.
const (
	StringType  = modelv1.StringType
	BoolType    = modelv1.BoolType
	Int64Type   = modelv1.Int64Type
	Float64Type = modelv1.Float64Type
	BinaryType  = modelv1.BinaryType

	SpanKindKey     = modelv1.SpanKindKey
	SamplerTypeKey  = modelv1.SamplerTypeKey
	SamplerParamKey = modelv1.SamplerParamKey
)

type SpanKind = modelv1.SpanKind

const (
	SpanKindClient      SpanKind = modelv1.SpanKindClient
	SpanKindServer      SpanKind = modelv1.SpanKindServer
	SpanKindProducer    SpanKind = modelv1.SpanKindProducer
	SpanKindConsumer    SpanKind = modelv1.SpanKindConsumer
	SpanKindInternal    SpanKind = modelv1.SpanKindInternal
	SpanKindUnspecified SpanKind = modelv1.SpanKindUnspecified
)

var SpanKindFromString = modelv1.SpanKindFromString

// KeyValues is a type alias that exposes convenience functions like Sort, FindByKey.
type KeyValues = modelv1.KeyValues

// String creates a String-typed KeyValue
var String = modelv1.String

// Bool creates a Bool-typed KeyValue
var Bool = modelv1.Bool

// Int64 creates a Int64-typed KeyValue
var Int64 = modelv1.Int64

// Float64 creates a Float64-typed KeyValue
var Float64 = modelv1.Float64

// Binary creates a Binary-typed KeyValue
var Binary = modelv1.Binary
