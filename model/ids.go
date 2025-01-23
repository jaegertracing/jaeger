// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2018 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package model

import (
	jaegerIdlModel "github.com/jaegertracing/jaeger-idl/model/v1"
)

const (
	// traceIDShortBytesLen indicates length of 64bit traceID when represented as list of bytes
	traceIDShortBytesLen = 8
)

// TraceID is a random 128bit identifier for a trace
type TraceID = jaegerIdlModel.TraceID

// SpanID is a random 64bit identifier for a span
type SpanID = jaegerIdlModel.SpanID

// ------- TraceID -------

// NewTraceID creates a new TraceID from two 64bit unsigned ints.
func NewTraceID(high, low uint64) TraceID {
	return jaegerIdlModel.NewTraceID(high, low)
}

// TraceIDFromString creates a TraceID from a hexadecimal string
func TraceIDFromString(s string) (TraceID, error) {
	return jaegerIdlModel.TraceIDFromString(s)
}

// TraceIDFromBytes creates a TraceID from list of bytes
func TraceIDFromBytes(data []byte) (TraceID, error) {
	return jaegerIdlModel.TraceIDFromBytes(data)
}

// ------- SpanID -------

// NewSpanID creates a new SpanID from a 64bit unsigned int.
func NewSpanID(v uint64) SpanID {
	return jaegerIdlModel.NewSpanID(v)
}

// SpanIDFromString creates a SpanID from a hexadecimal string
func SpanIDFromString(s string) (SpanID, error) {
	return jaegerIdlModel.SpanIDFromString(s)
}

// SpanIDFromBytes creates a SpandID from list of bytes
func SpanIDFromBytes(data []byte) (SpanID, error) {
	return jaegerIdlModel.SpanIDFromBytes(data)
}
