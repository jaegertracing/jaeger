// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2018 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package model

import (
	"encoding/binary"

	"go.opentelemetry.io/collector/pdata/pcommon"
)

// ToOTELTraceID converts the TraceID to OTEL's representation of a trace identitfier.
// This was taken from
// https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/internal/coreinternal/idutils/big_endian_converter.go.
func ToOTELTraceID(t TraceID) pcommon.TraceID {
	traceID := [16]byte{}
	binary.BigEndian.PutUint64(traceID[:8], t.High)
	binary.BigEndian.PutUint64(traceID[8:], t.Low)
	return traceID
}

func TraceIDFromOTEL(traceID pcommon.TraceID) TraceID {
	return TraceID{
		High: binary.BigEndian.Uint64(traceID[:traceIDShortBytesLen]),
		Low:  binary.BigEndian.Uint64(traceID[traceIDShortBytesLen:]),
	}
}

// ToOTELSpanID converts the SpanID to OTEL's representation of a span identitfier.
// This was taken from
// https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/internal/coreinternal/idutils/big_endian_converter.go.
func ToOTELSpanID(s SpanID) pcommon.SpanID {
	spanID := [8]byte{}
	binary.BigEndian.PutUint64(spanID[:], uint64(s))
	return pcommon.SpanID(spanID)
}

// ToOTELSpanID converts OTEL's SpanID to the model representation of a span identitfier.
// This was taken from
// https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/internal/coreinternal/idutils/big_endian_converter.go.
func SpanIDFromOTEL(spanID pcommon.SpanID) SpanID {
	return SpanID(binary.BigEndian.Uint64(spanID[:]))
}
