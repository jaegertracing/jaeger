// Copyright (c) 2017 Uber Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
	TraceID       TraceID       `json:"traceID"`
	SpanID        SpanID        `json:"spanID"`
	ParentSpanID  SpanID        `json:"parentSpanID"`
	OperationName string        `json:"operationName"`
	References    []SpanRef     `json:"references,omitempty"`
	Flags         Flags         `json:"flags,omitempty"`
	StartTime     time.Time     `json:"startTime"`
	Duration      time.Duration `json:"duration"`
	Tags          KeyValues     `json:"tags,omitempty"`
	Logs          []Log         `json:"logs,omitempty"`
	Process       *Process      `json:"process"`
	Warnings      []string      `json:"warnings,omitempty"`
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

// NormalizeTimestamps changes all timestamps in this span to UTC.
func (s *Span) NormalizeTimestamps() {
	s.StartTime = s.StartTime.UTC()
	for i := range s.Logs {
		s.Logs[i].Timestamp = s.Logs[i].Timestamp.UTC()
	}
}

func (s *Span) FlattenTags() KeyValues {
	retMe := s.Tags
	retMe = append(retMe, s.Process.Tags...)
	for _, l := range s.Logs {
		retMe = append(retMe, l.Fields...)
	}
	return retMe
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
