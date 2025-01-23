// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package model

import (
	"fmt"

	jaegerIdlModel "github.com/jaegertracing/jaeger-idl/model/v1"
)

// These constants are kept mostly for backwards compatibility.
const (
	// StringType indicates the value is a unicode string
	StringType = ValueType_STRING
	// BoolType indicates the value is a Boolean encoded as int64 number 0 or 1
	BoolType = ValueType_BOOL
	// Int64Type indicates the value is an int64 number
	Int64Type = ValueType_INT64
	// Float64Type indicates the value is a float64 number stored as int64
	Float64Type = ValueType_FLOAT64
	// BinaryType indicates the value is binary blob stored as a byte array
	BinaryType = ValueType_BINARY

	SpanKindKey     = "span.kind"
	SamplerTypeKey  = "sampler.type"
	SamplerParamKey = "sampler.param"
)

type SpanKind = jaegerIdlModel.SpanKind

const (
	SpanKindClient      SpanKind = "client"
	SpanKindServer      SpanKind = "server"
	SpanKindProducer    SpanKind = "producer"
	SpanKindConsumer    SpanKind = "consumer"
	SpanKindInternal    SpanKind = "internal"
	SpanKindUnspecified SpanKind = ""
)

func SpanKindFromString(kind string) (SpanKind, error) {
	switch SpanKind(kind) {
	case SpanKindClient, SpanKindServer, SpanKindProducer, SpanKindConsumer, SpanKindInternal, SpanKindUnspecified:
		return SpanKind(kind), nil
	default:
		return SpanKindUnspecified, fmt.Errorf("unknown span kind %q", kind)
	}
}

// KeyValues is a type alias that exposes convenience functions like Sort, FindByKey.
type KeyValues = jaegerIdlModel.KeyValues

// String creates a String-typed KeyValue
func String(key string, value string) KeyValue {
	return KeyValue{Key: key, VType: StringType, VStr: value}
}

// Bool creates a Bool-typed KeyValue
func Bool(key string, value bool) KeyValue {
	return KeyValue{Key: key, VType: BoolType, VBool: value}
}

// Int64 creates a Int64-typed KeyValue
func Int64(key string, value int64) KeyValue {
	return KeyValue{Key: key, VType: Int64Type, VInt64: value}
}

// Float64 creates a Float64-typed KeyValue
func Float64(key string, value float64) KeyValue {
	return KeyValue{Key: key, VType: Float64Type, VFloat64: value}
}

// Binary creates a Binary-typed KeyValue
func Binary(key string, value []byte) KeyValue {
	return KeyValue{Key: key, VType: BinaryType, VBinary: value}
}
