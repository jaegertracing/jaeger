// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2018 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package v1adapter

import (
	"encoding/binary"

	"go.opentelemetry.io/collector/pdata/pcommon"

	"github.com/jaegertracing/jaeger-idl/model/v1"
)

// FromV1TraceID converts the TraceID to OTEL's representation of a trace identitfier.
// This was taken from
// https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/internal/coreinternal/idutils/big_endian_converter.go.
func FromV1TraceID(t model.TraceID) pcommon.TraceID {
	traceID := [16]byte{}
	binary.BigEndian.PutUint64(traceID[:8], t.High)
	binary.BigEndian.PutUint64(traceID[8:], t.Low)
	return traceID
}

func ToV1TraceID(traceID pcommon.TraceID) model.TraceID {
	// traceIDShortBytesLen indicates length of 64bit traceID when represented as list of bytes
	const traceIDShortBytesLen = 8

	return model.TraceID{
		High: binary.BigEndian.Uint64(traceID[:traceIDShortBytesLen]),
		Low:  binary.BigEndian.Uint64(traceID[traceIDShortBytesLen:]),
	}
}

// FromV1SpanID converts the SpanID to OTEL's representation of a span identitfier.
// This was taken from
// https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/internal/coreinternal/idutils/big_endian_converter.go.
func FromV1SpanID(s model.SpanID) pcommon.SpanID {
	spanID := [8]byte{}
	binary.BigEndian.PutUint64(spanID[:], uint64(s))
	return pcommon.SpanID(spanID)
}

// ToV1SpanID converts OTEL's SpanID to the model representation of a span identitfier.
// This was taken from
// https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/internal/coreinternal/idutils/big_endian_converter.go.
func ToV1SpanID(spanID pcommon.SpanID) model.SpanID {
	return model.SpanID(binary.BigEndian.Uint64(spanID[:]))
}
