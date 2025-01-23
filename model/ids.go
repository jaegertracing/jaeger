// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2018 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package model

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strconv"

	jaegerIdlModel "github.com/jaegertracing/jaeger-idl/model/v1"
)

const (
	// traceIDShortBytesLen indicates length of 64bit traceID when represented as list of bytes
	traceIDShortBytesLen = 8
	// traceIDLongBytesLen indicates length of 128bit traceID when represented as list of bytes
	traceIDLongBytesLen = 16
)

// TraceID is a random 128bit identifier for a trace
type TraceID = jaegerIdlModel.TraceID

// SpanID is a random 64bit identifier for a span
type SpanID = jaegerIdlModel.SpanID

// ------- TraceID -------

// NewTraceID creates a new TraceID from two 64bit unsigned ints.
func NewTraceID(high, low uint64) TraceID {
	return TraceID{High: high, Low: low}
}

// TraceIDFromString creates a TraceID from a hexadecimal string
func TraceIDFromString(s string) (TraceID, error) {
	var hi, lo uint64
	var err error
	switch {
	case len(s) > 32:
		return TraceID{}, fmt.Errorf("TraceID cannot be longer than 32 hex characters: %s", s)
	case len(s) > 16:
		hiLen := len(s) - 16
		if hi, err = strconv.ParseUint(s[0:hiLen], 16, 64); err != nil {
			return TraceID{}, err
		}
		if lo, err = strconv.ParseUint(s[hiLen:], 16, 64); err != nil {
			return TraceID{}, err
		}
	default:
		if lo, err = strconv.ParseUint(s, 16, 64); err != nil {
			return TraceID{}, err
		}
	}
	return TraceID{High: hi, Low: lo}, nil
}

// TraceIDFromBytes creates a TraceID from list of bytes
func TraceIDFromBytes(data []byte) (TraceID, error) {
	var t TraceID
	switch {
	case len(data) == traceIDLongBytesLen:
		t.High = binary.BigEndian.Uint64(data[:traceIDShortBytesLen])
		t.Low = binary.BigEndian.Uint64(data[traceIDShortBytesLen:])
	case len(data) == traceIDShortBytesLen:
		t.Low = binary.BigEndian.Uint64(data)
	default:
		return TraceID{}, errors.New("invalid length for TraceID")
	}
	return t, nil
}

// ------- SpanID -------

// NewSpanID creates a new SpanID from a 64bit unsigned int.
func NewSpanID(v uint64) SpanID {
	return SpanID(v)
}

// SpanIDFromString creates a SpanID from a hexadecimal string
func SpanIDFromString(s string) (SpanID, error) {
	if len(s) > 16 {
		return SpanID(0), fmt.Errorf("SpanID cannot be longer than 16 hex characters: %s", s)
	}
	id, err := strconv.ParseUint(s, 16, 64)
	if err != nil {
		return SpanID(0), err
	}
	return SpanID(id), nil
}

// SpanIDFromBytes creates a SpandID from list of bytes
func SpanIDFromBytes(data []byte) (SpanID, error) {
	if len(data) != traceIDShortBytesLen {
		return SpanID(0), errors.New("invalid length for SpanID")
	}
	return NewSpanID(binary.BigEndian.Uint64(data)), nil
}
