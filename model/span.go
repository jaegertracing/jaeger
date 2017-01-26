// Copyright (c) 2017 Uber Technologies, Inc.
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
	"io"
	"strconv"
	"time"

	"encoding/gob"

	"github.com/opentracing/opentracing-go/ext"
)

const (
	// sampledFlag is the bit set in Flags in order to define a span as a sampled span
	sampledFlag = Flags(1)
	// debugFlag is the bit set in Flags in order to define a span as a debug span
	debugFlag = Flags(2)
)

// TraceID is a random 128bit identifier for a trace
type TraceID struct {
	Low  uint64 `json:"lo"`
	High uint64 `json:"hi"`
}

// Flags is a bit map of flags for a span
type Flags uint32

// SpanID is a random 64bit identifier for a span
type SpanID uint64

// Span represents a unit of work in an application, such as an RPC, a database call, etc.
type Span struct {
	TraceID       TraceID   `json:"traceID"`
	SpanID        SpanID    `json:"spanID"`
	ParentSpanID  SpanID    `json:"parentSpanID"`
	OperationName string    `json:"operationName"`
	References    []SpanRef `json:"references,omitempty"`
	Flags         Flags     `json:"flags,omitempty"`
	StartTime     uint64    `json:"startTime"` // microseconds since Unix epoch
	Duration      uint64    `json:"duration"`  // microseconds since Unix epoch
	Tags          KeyValues `json:"tags,omitempty"`
	Logs          []Log     `json:"logs,omitempty"`
	Process       *Process  `json:"process"`
	Warnings      []string  `json:"warnings,omitempty"`
}

// Hash implements Hash from Hashable.
func (s *Span) Hash(w io.Writer) (err error) {
	// gob is not the most efficient way, but it ensures we don't miss any fields.
	// See BenchmarkSpanHash in span_test.go
	enc := gob.NewEncoder(w)
	return enc.Encode(s)
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

// GetStartTime returns the span's StartTime as time.Time value.
func (s *Span) GetStartTime() time.Time {
	seconds := s.StartTime / 1000000
	nanos := 1000 * (s.StartTime % 1000000)
	return time.Unix(int64(seconds), int64(nanos))
}

// GetDuration returns the span's duration as time.Duration value.
func (s *Span) GetDuration() time.Duration {
	return time.Duration(s.Duration * 1000)
}

// TimeAsEpochMicroseconds converts time.Time to microseconds since epoch,
// which is the format the StartTime field is stored in the Span.
func TimeAsEpochMicroseconds(t time.Time) uint64 {
	return uint64(t.UnixNano() / 1000)
}

// DurationAsMicroseconds converts time.Duration to microseconds,
// which is the format the Duration field is stored in the Span.
func DurationAsMicroseconds(d time.Duration) uint64 {
	return uint64(d.Nanoseconds() / 1000)
}

// ------- Flags -------

// SetSampled sets the Flags as sampled
func (f *Flags) SetSampled() {
	f.setFlags(sampledFlag)
}

// SetDebug set the Flags as sampled
func (f *Flags) SetDebug() {
	f.setFlags(debugFlag)
}

func (f *Flags) setFlags(bit Flags) {
	*f = *f | bit
}

// IsSampled returns true if the Flags denote sampling
func (f Flags) IsSampled() bool {
	return f.checkFlags(sampledFlag)
}

// IsDebug returns true if the Flags denote debugging
// Debugging can be useful in testing tracing availability or correctness
func (f Flags) IsDebug() bool {
	return f.checkFlags(debugFlag)
}

func (f Flags) checkFlags(bit Flags) bool {
	return f&bit == bit
}

// ------- TraceID -------

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
	if len(s) > 32 {
		return TraceID{}, fmt.Errorf("TraceID cannot be longer than 32 hex characters: %s", s)
	} else if len(s) > 16 {
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

// ------- SpanID -------

func (s SpanID) String() string {
	return fmt.Sprintf("%x", uint64(s))
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

// MarshalText allows SpanID to serialize itself in JSON as a string.
func (s SpanID) MarshalText() ([]byte, error) {
	return []byte(s.String()), nil
}

// UnmarshalText allows SpanID to deserialize itself from a JSON string.
func (s *SpanID) UnmarshalText(text []byte) error {
	q, err := SpanIDFromString(string(text))
	if err != nil {
		return err
	}
	*s = q
	return nil
}
