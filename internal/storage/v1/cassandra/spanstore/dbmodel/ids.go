// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"

	"github.com/jaegertracing/jaeger-idl/model/v1"
)

// TraceID is a serializable form of model.TraceID
type TraceID [16]byte

// ToDomain converts trace ID from db-serializable form to domain TradeID
func (t TraceID) ToDomain() model.TraceID {
	traceIDHigh := binary.BigEndian.Uint64(t[:8])
	traceIDLow := binary.BigEndian.Uint64(t[8:])
	return model.NewTraceID(traceIDHigh, traceIDLow)
}

// String returns hex string representation of the trace ID.
func (t TraceID) String() string {
	traceIDHigh := binary.BigEndian.Uint64(t[:8])
	traceIDLow := binary.BigEndian.Uint64(t[8:])
	if traceIDHigh == 0 {
		return fmt.Sprintf("%016x", traceIDLow)
	}
	return fmt.Sprintf("%016x%016x", traceIDHigh, traceIDLow)
}

// MarshalJSON converts trace id into a base64 string enclosed in quotes.
func (t TraceID) MarshalJSON() ([]byte, error) {
	var out [26]byte
	out[0] = '"'
	base64.StdEncoding.Encode(out[1:25], t[:])
	out[25] = '"'
	return out[:], nil
}

func (t *TraceID) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return err
	}
	if len(b) != 16 {
		return fmt.Errorf("invalid TraceID length: %d", len(b))
	}
	copy(t[:], b)
	return nil
}
