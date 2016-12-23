// Copyright (c) 2016 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package model

import (
	"fmt"
	"strconv"

	"github.com/opentracing/opentracing-go/ext"
)

// TraceID is a random 128bit identifier for a trace
type TraceID struct {
	Low  uint64 `json:"lo"`
	High uint64 `json:"hi"`
}

// SpanID is a random 64bit identifier for a span
type SpanID uint64

// Span represents a unit of work in an application, such as an RPC, a database call, etc.
type Span struct {
	TraceID       TraceID   `json:"traceId"`
	SpanID        SpanID    `json:"spanId"`
	ParentSpanID  SpanID    `json:"parentSpanId"`
	OperationName string    `json:"operationName"`
	References    []SpanRef `json:"references,omitempty"`
	Flags         uint32    `json:"flags"`
	StartTime     uint64    `json:"startTime"`
	Duration      uint64    `json:"duration"`
	Tags          KeyValues `json:"tags,omitempty"`
	Logs          []Log     `json:"logs,omitempty"`
	Process       *Process  `json:"process"`
}

// HasSpanKind returns true if the span has a `span.kind` tag set to `kind`.
func (s *Span) HasSpanKind(kind ext.SpanKindEnum) bool {
	if tag, ok := s.Tags.FindByKey(string(ext.SpanKind)); ok {
		return tag.AsString() == string(kind)
	}
	return false
}

// IsRPCClient returns true if the span represents a client side of an RPC,
// as indicated by the `span.kind` tag set to `client`.
func (s *Span) IsRPCClient() bool {
	return s.HasSpanKind(ext.SpanKindRPCClientEnum)
}

// IsRPCServer returns true if the span represents a server side of an RPC,
// as indicated by the `span.kind` tag set to `server`.
func (s *Span) IsRPCServer() bool {
	return s.HasSpanKind(ext.SpanKindRPCServerEnum)
}

func (t TraceID) String() string {
	if t.High == 0 {
		return fmt.Sprintf("%x", t.Low)
	}
	return fmt.Sprintf("%x%016x", t.High, t.Low)
}

// TraceIDFromString creates a TraceID from a hexadecimal string
func TraceIDFromString(s string) (TraceID, error) {
	var hi, lo uint64
	var err error
	if len(s) > 16 {
		hiLen := len(s) - 16
		if hi, err = strconv.ParseUint(s[0:hiLen], 16, 64); err != nil {
			return TraceID{}, err
		}
		if lo, err = strconv.ParseUint(s[hiLen:], 16, 64); err != nil {
			return TraceID{}, err
		}
	} else {
		if lo, err = strconv.ParseUint(s, 16, 64); err != nil {
			return TraceID{}, err
		}
	}
	return TraceID{High: hi, Low: lo}, nil
}

// MarshalText allows TraceID to serialize itself in JSON as a string.
func (t TraceID) MarshalText() ([]byte, error) {
	return []byte(t.String()), nil
}

// UnmarshalText allows TraceID to deserialize itself from a JSON string.
func (t *TraceID) UnmarshalText(text []byte) error {
	q, err := TraceIDFromString(string(text))
	if err != nil {
		return err
	}
	*t = q
	return nil
}
